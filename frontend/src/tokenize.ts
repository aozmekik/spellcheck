/** A word and where it is, as UTF-16 offsets, which are CodeMirror positions. */
export interface Token {
  word: string
  from: number
  to: number
}

// Letters and combining marks with apostrophes inside. U+02BC (ʼ) is a letter
// to Unicode but an apostrophe to writers, so it only counts as the latter.
// Written without the v flag, which Safari 16 can't parse.
const wordPattern = /(?:(?!ʼ)[\p{L}\p{M}])+(?:['’ʼ](?:(?!ʼ)[\p{L}\p{M}])+)*/gu
const chunkPattern = /\S+/g
// Whole chunks to skip: URLs, emails and handles, anything with a digit, and
// a dot followed by a lowercase letter, as in nhost.io, main.go or e.g. A dot
// followed by a capital is more likely a missing space between sentences.
const skipPattern = /:\/\/|^www\.|@|\d|\p{L}\.\p{Ll}/u
const maxLength = 64
// Prefixes that only occur before a hyphen (pre-trained) and are not words
// on their own in the dictionary. Prefixes that are words, like re or co,
// need no rule.
const boundPrefixes = new Set([
  'pre', 'un', 'de', 'tri', 'neo', 'intra', 'omni', 'peri', 'socio', 'proto', 'nano', 'neuro', 'astro', 'aero', 'thermo', 'agro', 'electro', 'demi', 'hemi', 'mal', 'tele', 'mis', 'dys', 'ethno',
])
const hyphens = new Set(['-', '\u2010', '\u2011'])

/**
 * Splits text into the words worth checking. Like a word processor, it skips
 * acronyms (NASA), mixed-case names (iPhone), single letters other than "i",
 * and anything that looks like a URL, an email address or a number. Scripts
 * without capitals (Chinese, Hebrew) are skipped too: the dictionary is English.
 */
export function tokenize(text: string): Token[] {
  const tokens: Token[] = []
  for (const chunk of text.matchAll(chunkPattern)) {
    if (skipPattern.test(chunk[0])) continue
    for (const match of chunk[0].matchAll(wordPattern)) {
      // Measure the word as it will be sent; the positions stay the original's.
      const word = match[0].normalize('NFC')
      if (skip(word) || boundPrefix(chunk[0], match)) continue
      const from = chunk.index + match.index
      tokens.push({ word, from, to: from + match[0].length })
    }
  }
  return tokens
}

/** Whether the match is a bound prefix like the "pre" in "pre-trained". */
function boundPrefix(chunk: string, match: RegExpExecArray): boolean {
  const next = chunk[match.index + match[0].length]
  return next !== undefined && hyphens.has(next) && boundPrefixes.has(match[0].toLowerCase())
}

function skip(word: string): boolean {
  const length = [...word].length
  if (length > maxLength) return true
  if (length === 1) return word !== 'i'
  return word === word.toUpperCase() || /\p{Lu}/u.test(word.slice(1))
}
