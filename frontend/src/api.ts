export interface WordResult {
  word: string
  correct: boolean
  suggestions: string[]
}

export interface CheckResponse {
  /** The checker's version: its word list and build. Answers only change when it does. */
  version: string
  results: WordResult[]
}

/** An error response from the spell-check API. */
export class ApiError extends Error {
  readonly status: number
  /** For a 422, the positions of the words the API refused. */
  readonly rejected: readonly number[]

  constructor(status: number, detail: string, rejected: readonly number[] = []) {
    super(detail)
    this.name = 'ApiError'
    this.status = status
    this.rejected = rejected
  }
}

const timeoutMs = 5000

/** Checks a batch of words. The results line up with the words, in order. */
export async function checkWords(words: string[]): Promise<CheckResponse> {
  const res = await fetch('/v1/check', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ words }),
    signal: AbortSignal.timeout(timeoutMs),
  })
  if (!res.ok) {
    const problem: unknown = await res.json().catch(() => null)
    throw new ApiError(res.status, detailOf(problem) ?? res.statusText, rejectedPositions(problem))
  }
  const body: unknown = await res.json()
  if (!isCheckResponse(body) || body.results.length !== words.length) {
    throw new Error('the spell checker sent a malformed response')
  }
  return body
}

function isCheckResponse(body: unknown): body is CheckResponse {
  return isRecord(body) && typeof body.version === 'string' && Array.isArray(body.results) && body.results.every(isWordResult)
}

function isWordResult(result: unknown): result is WordResult {
  return (
    isRecord(result) &&
    typeof result.word === 'string' &&
    typeof result.correct === 'boolean' &&
    Array.isArray(result.suggestions) &&
    result.suggestions.every((s) => typeof s === 'string')
  )
}

function detailOf(problem: unknown): string | undefined {
  return isRecord(problem) && typeof problem.detail === 'string' ? problem.detail : undefined
}

/** Reads the word positions out of problem details pointers like "/words/3". */
function rejectedPositions(problem: unknown): number[] {
  if (!isRecord(problem) || !Array.isArray(problem.errors)) return []
  return problem.errors.flatMap((e: unknown) => {
    const match = isRecord(e) && typeof e.pointer === 'string' ? /^\/words\/(\d+)$/.exec(e.pointer) : null
    return match ? [Number(match[1])] : []
  })
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}
