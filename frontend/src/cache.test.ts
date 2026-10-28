import { describe, expect, it, vi } from 'vitest'
import { ApiError, type CheckResponse } from './api'
import { WordCache, type CheckWords } from './cache'

const answer = async (words: string[], version = 'v1'): Promise<CheckResponse> => ({
  version,
  results: words.map((word) => ({ word, correct: word !== 'teh', suggestions: word === 'teh' ? ['the'] : [] })),
})

/** A check whose answers and failures are released by hand, in any order. */
function manualCheck() {
  const pending: Array<{ words: string[]; release: (version?: string) => void; fail: (err: unknown) => void }> = []
  const check = vi.fn<CheckWords>(
    (words) =>
      new Promise<CheckResponse>((resolve, reject) => {
        pending.push({ words, release: (version) => void answer(words, version).then(resolve), fail: reject })
      }),
  )
  return { check, pending }
}

describe('WordCache', () => {
  it('asks about each word once', async () => {
    const check = vi.fn<CheckWords>(answer)
    const cache = new WordCache(check)

    expect(await cache.resolve(['teh', 'cat'])).toBe('ok')
    expect(await cache.resolve(['cat', 'teh', 'dog'])).toBe('ok')

    expect(check.mock.calls).toEqual([[['cat', 'teh']], [['dog']]])
    expect(cache.get('teh')).toEqual({ word: 'teh', correct: false, suggestions: ['the'] })
  })

  it('sends the words sorted, so a request says nothing about their order in the text', async () => {
    const check = vi.fn<CheckWords>(answer)
    await new WordCache(check).resolve(['the', 'cat', 'ate'])

    expect(check.mock.calls).toEqual([[['ate', 'cat', 'the']]])
  })

  it('shares one request between overlapping calls', async () => {
    const check = vi.fn<CheckWords>(answer)
    const cache = new WordCache(check)

    await Promise.all([cache.resolve(['teh', 'cat']), cache.resolve(['cat', 'teh'])])

    expect(check).toHaveBeenCalledTimes(1)
  })

  it('makes no request when every word is known', async () => {
    const check = vi.fn<CheckWords>(answer)
    const cache = new WordCache(check)
    await cache.resolve(['teh'])

    expect(await cache.resolve(['teh'])).toBe('ok')
    expect(await cache.resolve([])).toBe('ok')
    expect(check).toHaveBeenCalledTimes(1)
  })

  it('splits requests at 1000 words and keeps at most two in flight', async () => {
    const { check, pending } = manualCheck()
    const cache = new WordCache(check)
    const words = Array.from({ length: 2500 }, (_, i) => `word${i}`)

    const outcome = cache.resolve(words)
    await vi.waitFor(() => expect(check.mock.calls.map(([batch]) => batch.length)).toEqual([1000, 1000]))

    pending.shift()?.release()
    await vi.waitFor(() => expect(check).toHaveBeenCalledTimes(3))
    expect(check.mock.calls[2]?.[0].length).toBe(500)

    for (const p of pending.splice(0)) p.release()
    expect(await outcome).toBe('ok')
  })

  it('does not send a queued request while the API is being left alone', async () => {
    let now = 0
    const check = vi.fn<CheckWords>(answer).mockRejectedValueOnce(new TypeError('Failed to fetch'))
    const cache = new WordCache(check, () => now)
    const words = Array.from({ length: 2500 }, (_, i) => `word${i}`)

    expect(await cache.resolve(words)).toBe('offline')

    // The first request failed, so the third batch, queued behind it, was skipped.
    expect(check).toHaveBeenCalledTimes(2)
    now = 5000
    expect(await cache.resolve(words)).toBe('ok')
  })

  it('matches results to words by position, not by the echoed word', async () => {
    const check = vi.fn<CheckWords>(async (words) => ({
      version: 'v1',
      results: words.map((word) => ({ word: word.toUpperCase(), correct: false, suggestions: ['the'] })),
    }))
    const cache = new WordCache(check)

    expect(await cache.resolve(['teh'])).toBe('ok')
    expect(cache.get('teh')?.suggestions).toEqual(['the'])
  })

  it('starts over when the checker version changes', async () => {
    const check = vi.fn<CheckWords>(answer).mockImplementationOnce((words) => answer(words, 'v0'))
    const cache = new WordCache(check)
    await cache.resolve(['teh'])

    expect(await cache.resolve(['teh', 'cat'])).toBe('ok')

    // cat came back under v1, so teh's v0 answer was dropped and fetched again.
    expect(check.mock.calls).toEqual([[['teh']], [['cat']], [['teh']]])
    expect(cache.get('teh')).toEqual({ word: 'teh', correct: false, suggestions: ['the'] })
  })

  it('ignores an older answer that arrives after a version change', async () => {
    const { check, pending } = manualCheck()
    const cache = new WordCache(check)
    const first = cache.resolve(['old'])
    await vi.waitFor(() => expect(pending.length).toBe(1))
    pending.shift()?.release('v0')
    await first

    // Two requests in flight; the one for "stale" was made before the version changed.
    const stale = cache.resolve(['stale'])
    const fresh = cache.resolve(['fresh'])
    await vi.waitFor(() => expect(pending.length).toBe(2))
    const [staleRequest, freshRequest] = pending.splice(0)
    freshRequest?.release('v1') // the cache starts over here
    await fresh
    staleRequest?.release('v0') // answered by the old version: ignored and asked again
    await vi.waitFor(() => expect(pending.length).toBe(1))
    pending.shift()?.release('v1')

    expect(await stale).toBe('ok')
    expect(cache.get('stale')?.correct).toBe(true)
    expect(cache.get('fresh')?.correct).toBe(true)
    expect(cache.get('old')).toBeUndefined()
  })

  it('keeps an answer from an older request when its version is the current one', async () => {
    const { check, pending } = manualCheck()
    const cache = new WordCache(check)
    const first = cache.resolve(['old'])
    await vi.waitFor(() => expect(pending.length).toBe(1))
    pending.shift()?.release('v0')
    await first

    // Both requests were made under v0; the new build answers both.
    const y = cache.resolve(['y'])
    const z = cache.resolve(['z'])
    await vi.waitFor(() => expect(pending.length).toBe(2))
    for (const p of pending.splice(0)) p.release('v1')

    expect(await y).toBe('ok')
    expect(await z).toBe('ok')
    expect(check).toHaveBeenCalledTimes(3) // z was not asked again
  })

  it('ignores a failure from before a version change', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const { check, pending } = manualCheck()
    const cache = new WordCache(check)
    const first = cache.resolve(['old'])
    await vi.waitFor(() => expect(pending.length).toBe(1))
    pending.shift()?.release('v0')
    await first

    const stale = cache.resolve(['stale'])
    const fresh = cache.resolve(['fresh'])
    await vi.waitFor(() => expect(pending.length).toBe(2))
    const [staleRequest, freshRequest] = pending.splice(0)
    freshRequest?.release('v1')
    await fresh
    staleRequest?.fail(new ApiError(422, 'invalid', [0])) // the old build refused it: asked again, not given up
    await vi.waitFor(() => expect(pending.length).toBe(1))
    pending.shift()?.release('v1')

    expect(await stale).toBe('ok')
    expect(cache.get('stale')?.correct).toBe(true)
  })

  it('does not pause for a connection failure from before a version change', async () => {
    const { check, pending } = manualCheck()
    const cache = new WordCache(check)
    const first = cache.resolve(['old'])
    await vi.waitFor(() => expect(pending.length).toBe(1))
    pending.shift()?.release('v0')
    await first

    const stale = cache.resolve(['stale'])
    const fresh = cache.resolve(['fresh'])
    await vi.waitFor(() => expect(pending.length).toBe(2))
    const [staleRequest, freshRequest] = pending.splice(0)
    freshRequest?.release('v1')
    await fresh
    staleRequest?.fail(new TypeError('Failed to fetch'))
    await vi.waitFor(() => expect(pending.length).toBe(1))
    expect(cache.retryIn()).toBe(0)
    pending.shift()?.release('v1')

    expect(await stale).toBe('ok')
  })

  it('does not start a new pause when it wakes at the end of one', async () => {
    let t = 0
    const check = vi.fn<CheckWords>(answer).mockRejectedValueOnce(new TypeError('Failed to fetch'))
    const cache = new WordCache(check, () => t++) // the clock moves a millisecond per read
    expect(await cache.resolve(['teh'])).toBe('offline')

    t = cache.retryIn() - 1 // the retry timer fires right at the boundary
    expect(await cache.resolve(['teh'])).toBe('offline')
    expect(check).toHaveBeenCalledTimes(1)
    expect(cache.retryIn()).toBeLessThan(100) // not a fresh 5 s pause
  })

  it('does not reject when the check function throws synchronously', async () => {
    let now = 0
    const check = vi.fn<CheckWords>(answer).mockImplementationOnce(() => {
      throw new Error('sync boom')
    })
    const cache = new WordCache(check, () => now)

    expect(await cache.resolve(['teh'])).toBe('offline')
    now = 5000
    expect(await cache.resolve(['teh'])).toBe('ok')
  })

  it('leaves the API alone for 5 seconds after it fails', async () => {
    let now = 0
    const check = vi.fn<CheckWords>(answer).mockRejectedValueOnce(new TypeError('Failed to fetch'))
    const cache = new WordCache(check, () => now)

    expect(await cache.resolve(['teh'])).toBe('offline')
    now = 3000
    expect(cache.retryIn()).toBe(2000)
    expect(await cache.resolve(['teh'])).toBe('offline')
    expect(check).toHaveBeenCalledTimes(1)

    now = 5000
    expect(await cache.resolve(['teh'])).toBe('ok')
    expect(check).toHaveBeenCalledTimes(2)
  })

  it.each([404, 408, 429, 503])('treats a %i as an outage and tries again later', async (status) => {
    let now = 0
    const check = vi.fn<CheckWords>(answer).mockRejectedValueOnce(new ApiError(status, 'unavailable'))
    const cache = new WordCache(check, () => now)

    expect(await cache.resolve(['teh'])).toBe('offline')
    now = 5000
    expect(await cache.resolve(['teh'])).toBe('ok')
  })

  it('reports words the API rejects, and never sends them again', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const check = vi.fn<CheckWords>().mockRejectedValue(new ApiError(400, 'invalid'))
    const cache = new WordCache(check)

    expect(await cache.resolve(['x'])).toBe('rejected')
    expect(await cache.resolve(['x'])).toBe('rejected')
    expect(check).toHaveBeenCalledTimes(1)
    expect(cache.get('x')).toBeUndefined()
  })

  it('gives up on a whole batch when a 422 names none of its words', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const check = vi.fn<CheckWords>(answer).mockRejectedValueOnce(new ApiError(422, 'invalid', [99]))
    const cache = new WordCache(check)

    expect(await cache.resolve(['teh', 'cat'])).toBe('rejected')
    expect(check).toHaveBeenCalledTimes(1)
  })

  it('after a 422, gives up only on the words it named and asks again about the rest', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const check = vi.fn<CheckWords>(answer).mockRejectedValueOnce(new ApiError(422, 'invalid', [2]))
    const cache = new WordCache(check)

    expect(await cache.resolve(['teh', 'x', 'cat'])).toBe('rejected')

    expect(check.mock.calls).toEqual([[['cat', 'teh', 'x']], [['cat', 'teh']]])
    expect(cache.get('x')).toBeUndefined()
    expect(cache.get('teh')?.correct).toBe(false)
    expect(cache.get('cat')?.correct).toBe(true)
  })
})
