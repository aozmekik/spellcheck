import type { Extension, Text } from '@codemirror/state'
import { EditorView } from '@codemirror/view'

const storageKey = 'spellcheck.draft'

/** The text from the last session, or null when there is none. */
export function loadDraft(): string | null {
  try {
    return localStorage.getItem(storageKey)
  } catch {
    return null
  }
}

export function saveDraft(text: string): void {
  try {
    localStorage.setItem(storageKey, text)
  } catch {
    // Storage is unavailable (private browsing, quota): the draft lives for this session only.
  }
}

/**
 * Saves the text shortly after it changes, and at once when the page is
 * hidden or closed, so a reload brings back the last keystroke too. Across
 * tabs the last writer wins.
 */
export function keepDraft(): Extension {
  let timer: ReturnType<typeof setTimeout> | undefined
  let unsaved: Text | undefined
  const flush = () => {
    if (unsaved === undefined) return
    clearTimeout(timer)
    saveDraft(unsaved.toString())
    unsaved = undefined
  }
  addEventListener('pagehide', flush)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') flush()
  })
  return EditorView.updateListener.of((update) => {
    if (!update.docChanged) return
    unsaved = update.state.doc
    clearTimeout(timer)
    timer = setTimeout(flush, 500)
  })
}
