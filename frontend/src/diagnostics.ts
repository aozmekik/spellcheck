import { forEachDiagnostic, setDiagnosticsEffect, type Action, type Diagnostic } from '@codemirror/lint'
import type { EditorState } from '@codemirror/state'
import type { WordResult } from './api'
import type { Token } from './tokenize'

/** The cached answer for a word, or undefined while there is none. */
export type Lookup = (word: string) => WordResult | undefined

/**
 * Turns every token whose answer says "misspelled" into a diagnostic, with
 * the suggestions and "Add to dictionary" as quick fixes. Tokens without an
 * answer yet are left alone rather than underlined.
 */
export function diagnosticsFor(
  tokens: readonly Token[],
  lookup: Lookup,
  addToDictionary: (word: string) => Action,
): Diagnostic[] {
  return tokens.flatMap((t) => {
    const result = lookup(t.word)
    return result && !result.correct ? [diagnostic(t, result, addToDictionary(t.word))] : []
  })
}

/** What a suggestion inserts: the writer's own kind of apostrophe is kept. */
export function replacement(word: string, suggestion: string): string {
  const apostrophe = /[’ʼ]/.exec(word)?.[0]
  return apostrophe ? suggestion.replaceAll("'", apostrophe) : suggestion
}

function diagnostic(token: Token, result: WordResult, addToDictionary: Action): Diagnostic {
  const [noteClass, note] =
    result.suggestions.length > 0 ? ['cm-spellPrompt', 'Did you mean'] : ['cm-spellNote', 'No suggestions']
  return {
    from: token.from,
    to: token.to,
    severity: 'error',
    message: `${token.word} is not in the dictionary`,
    // The tooltip sits on the word, so it hides the word and keeps the
    // prompt; the issues panel lists every word, so it does the opposite.
    // See theme.ts.
    renderMessage: () => {
      const message = document.createDocumentFragment()
      message.append(span('cm-spellWord', token.word), span(noteClass, note))
      return message
    },
    actions: [
      ...result.suggestions.map((suggestion): Action => {
        const insert = replacement(token.word, suggestion)
        return { name: insert, apply: (view, from, to) => view.dispatch(fix(view.state, from, to, insert)) }
      }),
      addToDictionary,
    ],
  }
}

/**
 * The transaction that applies a fix: it replaces the word and drops its
 * underline at once. Left to the next check, the old mark would sit on the
 * corrected word for a moment.
 */
function fix(state: EditorState, from: number, to: number, insert: string) {
  const changes = state.changes({ from, to, insert })
  const remaining: Diagnostic[] = []
  forEachDiagnostic(state, (d, dFrom, dTo) => {
    if (dFrom === from && dTo === to) return
    remaining.push({ ...d, from: changes.mapPos(dFrom), to: changes.mapPos(dTo, 1) })
  })
  return { changes, effects: setDiagnosticsEffect.of(remaining) }
}

function span(className: string, text: string): HTMLSpanElement {
  const element = document.createElement('span')
  element.className = className
  element.textContent = text
  return element
}
