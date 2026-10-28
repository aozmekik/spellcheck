import { afterEach, describe, expect, it, vi } from 'vitest'
import { loadDraft, saveDraft } from './draft'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('draft', () => {
  it('brings back what was saved, including an emptied document', () => {
    const data = new Map<string, string>()
    vi.stubGlobal('localStorage', { getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => void data.set(k, v) })

    expect(loadDraft()).toBeNull()
    saveDraft('Teh text')
    expect(loadDraft()).toBe('Teh text')
    saveDraft('')
    expect(loadDraft()).toBe('')
  })

  it('does nothing when storage is unavailable', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('denied')
      },
    })

    expect(() => saveDraft('x')).not.toThrow()
    expect(loadDraft()).toBeNull()
  })
})
