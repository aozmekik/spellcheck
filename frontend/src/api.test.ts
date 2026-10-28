import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, checkWords } from './api'

const respond = (status: number, body: unknown) =>
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body), { status })))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('checkWords', () => {
  it('returns the results in request order, with the checker version', async () => {
    const results = [
      { word: 'teh', correct: false, suggestions: ['the'] },
      { word: 'cat', correct: true, suggestions: [] },
    ]
    respond(200, { version: 'v1', results })

    expect(await checkWords(['teh', 'cat'])).toEqual({ version: 'v1', results })
  })

  it.each([
    ['too few results', { version: 'v1', results: [] }],
    ['no results', { version: 'v1' }],
    ['no version', { results: [{ word: 'teh', correct: false, suggestions: [] }] }],
    ['a result of the wrong shape', { version: 'v1', results: [{ word: 'teh', correct: 'no', suggestions: [] }] }],
    ['a result without its word', { version: 'v1', results: [{ correct: false, suggestions: [] }] }],
    ['suggestions that are not strings', { version: 'v1', results: [{ word: 'teh', correct: false, suggestions: [1] }] }],
    ['not an object', 'ok'],
  ])('rejects a response with %s', async (_name, body) => {
    respond(200, body)
    await expect(checkWords(['teh'])).rejects.toThrow('malformed')
  })

  it('turns problem details into an ApiError', async () => {
    respond(422, { status: 422, detail: 'some words can’t be checked' })

    const err: unknown = await checkWords(['']).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 422, message: 'some words can’t be checked', rejected: [] })
  })

  it('reads which words a 422 refused from the JSON pointers', async () => {
    respond(422, {
      status: 422,
      detail: 'some words can’t be checked',
      errors: [{ pointer: '/words/1', detail: 'x' }, { pointer: '/words/3', detail: 'x' }, { pointer: '/words', detail: 'x' }],
    })

    const err = (await checkWords(['a', '', 'b', ' ']).catch((e: unknown) => e)) as ApiError
    expect(err.rejected).toEqual([1, 3])
  })

  it('falls back to the status text when the error body is not JSON', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>', { status: 502, statusText: 'Bad Gateway' })))

    await expect(checkWords(['teh'])).rejects.toMatchObject({ status: 502, message: 'Bad Gateway' })
  })
})
