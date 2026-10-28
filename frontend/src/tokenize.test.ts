import { describe, expect, it } from 'vitest'
import { tokenize } from './tokenize'

const words = (text: string) => tokenize(text).map((t) => t.word)

describe('tokenize', () => {
  it('reports UTF-16 offsets, which CodeMirror uses as positions', () => {
    expect(tokenize('😀 teh cat')).toEqual([
      { word: 'teh', from: 3, to: 6 },
      { word: 'cat', from: 7, to: 10 },
    ])
  })

  it('keeps apostrophes inside words, in any style', () => {
    expect(words("don't don’t donʼt rock'n'roll")).toEqual(["don't", 'don’t', 'donʼt', "rock'n'roll"])
  })

  it('drops apostrophes and quotes around words', () => {
    expect(words(`'tis the dogs' toys, “quoted” dogsʼ`)).toEqual(['tis', 'the', 'dogs', 'toys', 'quoted', 'dogs'])
  })

  it('splits hyphenated words and punctuation', () => {
    expect(words('a well-known, often-quoted line.')).toEqual(['well', 'known', 'often', 'quoted', 'line'])
  })

  it('skips bound prefixes before a hyphen, but not the same letters elsewhere', () => {
    expect(words('pre-trained Pre-war un-American socio-economic pre trained')).toEqual([
      'trained',
      'war',
      'American',
      'economic',
      'pre',
      'trained',
    ])
    expect(words('pre\u2010trained pre\u2011trained')).toEqual(['trained', 'trained'])
  })

  it('sends composed characters but keeps the original offsets', () => {
    const decomposed = 'café ok'
    expect(tokenize(decomposed)).toEqual([
      { word: 'café', from: 0, to: 5 },
      { word: 'ok', from: 6, to: 8 },
    ])
  })

  it('measures length after normalizing, as the server does', () => {
    // 64 code points as typed, 126 once composed: over the API's limit.
    const long = 'a' + '̈́'.repeat(63)
    expect([...long.normalize('NFC')].length).toBeGreaterThan(64)
    expect(words(`${long} ok`)).toEqual(['ok'])
  })

  it('skips URLs, email addresses, handles, numbers, file names and abbreviations', () => {
    expect(
      words('see https://nhost.io www.example.com nhost.io main.go me@example.com @nhost 3rd COVID-19 e.g. U.S. here'),
    ).toEqual(['see', 'here'])
  })

  it('splits at a dot followed by a capital, a likely missing space', () => {
    expect(words('I recieve.Teh cat')).toEqual(['recieve', 'Teh', 'cat'])
  })

  it('skips acronyms, mixed-case names and single letters other than i', () => {
    expect(words('NASA iPhone GitHub McDonald a x I i Hello')).toEqual(['i', 'Hello'])
  })

  it('skips words longer than 64 characters', () => {
    expect(words(`${'a'.repeat(65)} fine`)).toEqual(['fine'])
  })
})
