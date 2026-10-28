import { ApiError, type CheckResponse, type WordResult } from './api'

export type CheckWords = (words: string[]) => Promise<CheckResponse>

/**
 * 'ok' when every word has an answer, 'offline' when some wait for the API to
 * be reachable again, 'rejected' when the API refused some of them.
 */
export type Outcome = 'ok' | 'offline' | 'rejected'

/** The most words the API accepts in one request. */
const batchSize = 1000
/** How many requests may be in flight at once; the rest queue. */
const maxInFlight = 2
/** How long to leave the API alone after it fails. */
const pauseMs = 5000
/**
 * How many rounds resolve makes before giving up for now. A round only
 * re-asks about words that a version change or a partial rejection sent
 * back to the start, so more than one is rare.
 */
const maxRounds = 5
/**
 * Responses that sending the same words again can't change. Anything else (no
 * connection, a timeout, a 5xx, or a 404 or 429 from a proxy) is retried.
 */
const permanent = new Set([400, 413, 422])

/**
 * WordCache remembers the API's answer for each word, so every distinct word
 * is sent once per session. Answers only change when the checker's version
 * does, which every response reports: when it changes, the cache starts over.
 */
export class WordCache {
  // null marks a word the API refused to check; it is never sent again.
  private readonly results = new Map<string, WordResult | null>()
  private readonly inflight = new Map<string, Promise<void>>()
  private readonly queue: Array<() => void> = []
  private active = 0
  private version: string | undefined
  // Bumped when the version changes. A response to a request made under an
  // older generation is stale and ignored; its words are asked about again.
  private generation = 0
  private readonly check: CheckWords
  private readonly now: () => number
  private offlineUntil = 0

  // The clock is monotonic so that a wall-clock step during an outage can't
  // stretch or shorten the pause.
  constructor(check: CheckWords, now: () => number = () => performance.now()) {
    this.check = check
    this.now = now
  }

  get(word: string): WordResult | undefined {
    return this.results.get(word) ?? undefined
  }

  /** Milliseconds until the API may be called again after a failure. */
  retryIn(): number {
    return Math.max(0, this.offlineUntil - this.now())
  }

  /**
   * Makes sure each word has an answer, fetching only words never seen
   * before; overlapping calls share requests. It never rejects: while the API
   * is failing it answers from the cache and reports 'offline'.
   */
  async resolve(words: readonly string[]): Promise<Outcome> {
    let exhausted = true
    for (let round = 0; round < maxRounds; round++) {
      // Sorted, so a request carries the distinct words and nothing of the
      // order they were written in.
      const unseen = words.filter((w) => !this.results.has(w) && !this.inflight.has(w)).sort()
      if (unseen.length > 0 && this.now() >= this.offlineUntil) {
        for (let i = 0; i < unseen.length; i += batchSize) {
          this.request(unseen.slice(i, i + batchSize))
        }
      }
      const pending = new Set(words.flatMap((w) => this.inflight.get(w) ?? []))
      if (pending.size === 0) {
        exhausted = false
        break
      }
      await Promise.all(pending)
    }
    if (words.some((w) => !this.results.has(w))) {
      // Unanswered after every round, with nothing still in flight and no
      // failure to explain it: the version keeps changing, as in a rollout.
      // Leave the API alone a while.
      if (exhausted && this.retryIn() === 0 && !words.some((w) => this.inflight.has(w))) {
        this.offlineUntil = this.now() + pauseMs
      }
      return 'offline'
    }
    return words.some((w) => this.results.get(w) === null) ? 'rejected' : 'ok'
  }

  private request(batch: string[]): void {
    const generation = this.generation
    const done = new Promise<void>((finish) => {
      this.queue.push(() => {
        // A pause that began while this batch waited applies to it too. The
        // call is deferred so a check that throws synchronously still fails
        // the batch rather than the queue.
        const attempt: Promise<CheckResponse | undefined> =
          this.now() < this.offlineUntil ? Promise.resolve(undefined) : Promise.resolve().then(() => this.check(batch))
        attempt
          .then(
            (response) => {
              if (response) this.store(batch, response, generation)
            },
            (err: unknown) => this.fail(batch, err, generation),
          )
          .finally(() => {
            for (const w of batch) if (this.inflight.get(w) === done) this.inflight.delete(w)
            this.active--
            finish()
            this.pump()
          })
      })
    })
    for (const w of batch) this.inflight.set(w, done)
    this.pump()
  }

  /** Starts queued requests while fewer than maxInFlight are running. */
  private pump(): void {
    while (this.active < maxInFlight) {
      const run = this.queue.shift()
      if (!run) return
      this.active++
      run()
    }
  }

  private store(batch: string[], { version, results }: CheckResponse, generation: number): void {
    if (this.version !== undefined && this.version !== version) {
      // Another version than the one the cache is keyed on. From a request
      // made before the last change it is stale: ignore it, and the words
      // are asked again. Otherwise the checker has changed: start over.
      if (generation !== this.generation) return
      this.results.clear()
      this.generation++
    }
    this.version = version
    // Results line up with the batch, so a word the server echoes
    // differently still lands under the word that was sent.
    batch.forEach((w, i) => this.results.set(w, results[i] ?? null))
  }

  private fail(batch: string[], err: unknown, generation: number): void {
    // A failure from before the last version change says nothing about the
    // checker now answering; the words are asked again.
    if (generation !== this.generation) return
    if (!(err instanceof ApiError) || !permanent.has(err.status)) {
      this.offlineUntil = this.now() + pauseMs
      return
    }
    // Client and server disagree about what a valid request is, so sending
    // the same words again can't help. A 422 names the words it refused; the
    // rest of the batch was fine and is asked about again. One that names
    // none of them is a server bug, and the whole batch is given up rather
    // than sent again at once.
    console.error('The spell checker rejected a request:', err)
    const named = err.status === 422 ? err.rejected.map((i) => batch[i]).filter((w) => w !== undefined) : []
    for (const w of named.length > 0 ? named : batch) this.results.set(w, null)
    const rest = batch.filter((w) => !this.results.has(w))
    if (rest.length > 0) this.request(rest)
  }
}
