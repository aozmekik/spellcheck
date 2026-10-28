import { diagnosticCount, forEachDiagnostic, forceLinting, linter, type Action, type Diagnostic } from '@codemirror/lint'
import { StateEffect, StateField, type EditorState, type Extension } from '@codemirror/state'
import { EditorView, activateHover } from '@codemirror/view'
import { WordCache, type CheckWords } from './cache'
import { diagnosticsFor } from './diagnostics'
import { PersonalDictionary } from './dictionary'
import type { SpellStatus } from './status'
import { tokenize } from './tokenize'

/**
 * Reads the status from the editor state rather than from a lint run, so it
 * always matches the underlines on screen, even while the text is changing.
 */
export function spellStatus(state: EditorState): SpellStatus {
  return { connection: state.field(connection), issues: diagnosticCount(state) }
}

const setConnection = StateEffect.define<SpellStatus['connection']>()

const connection = StateField.define<SpellStatus['connection']>({
  create: () => 'checking',
  update: (value, tr) => tr.effects.reduce((v, e) => (e.is(setConnection) ? e.value : v), value),
})

// Re-runs the linter when something other than the text changes.
const refresh = StateEffect.define<null>()

/**
 * Underlines misspelled words, with suggestions as quick fixes. The scheduling
 * is left to @codemirror/lint: it waits for a pause in typing, keeps existing
 * underlines on their words while the text changes, and drops results that
 * were computed for text that has changed since.
 */
export function spellcheck(check: CheckWords): Extension {
  const cache = new WordCache(check)
  const personal = new PersonalDictionary()
  let retry: ReturnType<typeof setTimeout> | undefined

  const relint = (view: EditorView) => {
    view.dispatch({ effects: refresh.of(null) })
    forceLinting(view)
  }

  // While the API is down, ask again when the backoff ends, and re-lint only
  // once it answers: new diagnostics would close a tooltip the user is reading.
  const retryLater = (view: EditorView, words: string[]) => {
    if (retry !== undefined) return
    retry = setTimeout(async () => {
      retry = undefined
      if (!view.dom.isConnected) return
      if ((await cache.resolve(words)) === 'offline') retryLater(view, words)
      else relint(view)
    }, cache.retryIn())
  }

  const addToDictionary = (word: string): Action => ({
    name: 'Add to dictionary',
    markClass: 'cm-addWord',
    apply(view) {
      personal.add(word)
      relint(view)
    },
  })

  const source = async (view: EditorView): Promise<Diagnostic[]> => {
    const doc = view.state.doc
    const tokens = tokenize(doc.toString()).filter((t) => !personal.has(t.word))
    const words = [...new Set(tokens.map((t) => t.word))]
    const outcome = await cache.resolve(words)
    // The text changed while the API answered. @codemirror/lint drops this
    // run, and the run already scheduled for the new text reports instead.
    if (view.state.doc !== doc) return []

    const status = outcome === 'ok' ? 'online' : outcome
    if (view.state.field(connection) !== status) view.dispatch({ effects: setConnection.of(status) })
    if (outcome === 'offline') retryLater(view, words)

    return diagnosticsFor(tokens, (word) => cache.get(word), addToDictionary)
  }

  return [
    connection,
    linter(source, {
      delay: 350,
      needsRefresh: (update) => update.transactions.some((tr) => tr.effects.some((e) => e.is(refresh))),
    }),
    tapToOpen(),
  ]
}

/**
 * A touch screen has no hover, so a tap on an underlined word opens its
 * suggestions the way F8 does: they stay until the caret leaves the word or
 * the text changes. Pointer events rather than touch events, which would
 * make CodeMirror's own touch listeners block scrolling; a scroll ends in
 * pointercancel, so only a tap reaches pointerup.
 */
function tapToOpen(): Extension {
  return EditorView.domEventHandlers({
    pointerup(event, view) {
      if (event.pointerType !== 'touch') return
      const pos = view.posAtCoords({ x: event.clientX, y: event.clientY })
      const word = pos === null ? undefined : underlinedWordAt(view.state, pos)
      if (!word) return
      activateHover(view, word.from, 1, {
        // The caret may land just past the word, after a comma, so allow a
        // character of slack before a selection change closes the menu.
        until: (tr) =>
          tr.docChanged || tr.newSelection.main.head < word.from - 1 || tr.newSelection.main.head > word.to + 1,
      })
    },
  })
}

function underlinedWordAt(state: EditorState, pos: number): { from: number; to: number } | undefined {
  let found: { from: number; to: number } | undefined
  forEachDiagnostic(state, (_diagnostic, from, to) => {
    if (from <= pos && pos <= to) found = { from, to }
  })
  return found
}
