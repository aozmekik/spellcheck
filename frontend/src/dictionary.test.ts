import { afterEach, describe, expect, it, vi } from 'vitest'
import { PersonalDictionary } from './dictionary'

const key = 'spellcheck.personal-dictionary'

function fakeStorage(stored: Record<string, string> = {}) {
  const data = new Map(Object.entries(stored))
  const storage = {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
  }
  vi.stubGlobal('localStorage', storage)
  return data
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PersonalDictionary', () => {
  it('knows an added word in any case', () => {
    fakeStorage()
    const personal = new PersonalDictionary()

    expect(personal.has('Nhost')).toBe(false)
    personal.add('Nhost')
    expect(personal.has('nhost')).toBe(true)
    expect(personal.has('NHOST')).toBe(true)
  })

  it('keeps the words for the next session', () => {
    const data = fakeStorage()
    new PersonalDictionary().add('Nhost')

    expect(JSON.parse(data.get(key) ?? '')).toEqual(['nhost'])
    expect(new PersonalDictionary().has('nhost')).toBe(true)
  })

  it('merges with words another tab stored meanwhile', () => {
    const data = fakeStorage()
    const personal = new PersonalDictionary()
    data.set(key, JSON.stringify(['hasura']))

    personal.add('Nhost')

    expect(JSON.parse(data.get(key) ?? '')).toEqual(['hasura', 'nhost'])
  })

  it('still works for the session when storage is unavailable', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('denied')
      },
    })
    const personal = new PersonalDictionary()

    personal.add('Nhost')

    expect(personal.has('nhost')).toBe(true)
  })

  it('ignores anything in storage that is not a list of words', () => {
    fakeStorage({ [key]: '{"not": "a list"}' })
    expect(new PersonalDictionary().has('not')).toBe(false)

    fakeStorage({ [key]: '["ok", 3, null]' })
    expect(new PersonalDictionary().has('ok')).toBe(true)
  })
})
