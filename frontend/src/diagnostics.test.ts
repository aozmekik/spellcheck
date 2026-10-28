import { forEachDiagnostic, setDiagnostics, type Action } from '@codemirror/lint'
import { EditorState, type TransactionSpec } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'
import { describe, expect, it, vi } from 'vitest'
import type { WordResult } from './api'
import { diagnosticsFor, replacement } from './diagnostics'
import { tokenize } from './tokenize'

const answers: Record<string, WordResult> = {
  teh: { word: 'teh', correct: false, suggestions: ['the', 'ten'] },
  cat: { word: 'cat', correct: true, suggestions: [] },
  xqzt: { word: 'xqzt', correct: false, suggestions: [] },
  'don’t': { word: 'don’t', correct: false, suggestions: ["don't"] },
  dont: { word: 'dont', correct: false, suggestions: ["don't"] },
}
const lookup = (word: string) => answers[word]
const addToDictionary = (word: string): Action => ({ name: `Add ${word}`, apply: () => {} })

/** An editor state with the diagnostics for doc in place, and a view that records what a fix dispatches. */
function editorWith(doc: string) {
  const diagnostics = diagnosticsFor(tokenize(doc), lookup, addToDictionary)
  const base = EditorState.create({ doc })
  const state = base.update(setDiagnostics(base, diagnostics)).state
  const dispatch = vi.fn()
  return { diagnostics, state, view: { state, dispatch } as unknown as EditorView, dispatch }
}

describe('diagnosticsFor', () => {
  it('underlines each misspelled occurrence where it is, and nothing else', () => {
    const diagnostics = diagnosticsFor(tokenize('teh cat teh unknown'), lookup, addToDictionary)

    expect(diagnostics.map(({ from, to, severity, message }) => ({ from, to, severity, message }))).toEqual([
      { from: 0, to: 3, severity: 'error', message: 'teh is not in the dictionary' },
      { from: 8, to: 11, severity: 'error', message: 'teh is not in the dictionary' },
    ])
  })

  it('offers the suggestions, then adding the word, as quick fixes', () => {
    const [diagnostic] = diagnosticsFor(tokenize('teh'), lookup, addToDictionary)

    expect(diagnostic?.actions?.map((a) => a.name)).toEqual(['the', 'ten', 'Add teh'])
  })

  it('applies a suggestion by replacing the word and dropping its underline at once', () => {
    const { diagnostics, state, view, dispatch } = editorWith('dont cat teh')

    diagnostics[0]?.actions?.[0]?.apply(view, 0, 4) // dont → don't, one character longer

    const next = state.update(dispatch.mock.calls[0]?.[0] as TransactionSpec).state
    expect(next.doc.toString()).toBe("don't cat teh")
    const underlined: Array<[number, number]> = []
    forEachDiagnostic(next, (_d, from, to) => underlined.push([from, to]))
    expect(underlined).toEqual([[10, 13]]) // the other underline moved with the text
  })

  it('only offers to add a word that has no suggestions', () => {
    const [diagnostic] = diagnosticsFor(tokenize('xqzt'), lookup, addToDictionary)

    expect(diagnostic?.actions?.map((a) => a.name)).toEqual(['Add xqzt'])
  })

  it('keeps the writer’s curly apostrophe in the fix', () => {
    const [diagnostic] = diagnosticsFor(tokenize('don’t'), lookup, addToDictionary)

    expect(diagnostic?.actions?.[0]?.name).toBe('don’t')
  })
})

describe('replacement', () => {
  it('uses the writer’s kind of apostrophe', () => {
    expect(replacement('don’t', "don't")).toBe('don’t')
    expect(replacement('donʼt', "don't")).toBe('donʼt')
    expect(replacement('dont', "don't")).toBe("don't")
  })
})
