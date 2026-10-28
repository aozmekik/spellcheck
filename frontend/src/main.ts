import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { lintKeymap } from '@codemirror/lint'
import { EditorState } from '@codemirror/state'
import {
  EditorView,
  ViewPlugin,
  closeHoverTooltips,
  drawSelection,
  hasHoverTooltips,
  keymap,
  placeholder,
  tooltips,
} from '@codemirror/view'
import { checkWords } from './api'
import { keepDraft, loadDraft } from './draft'
import { spellcheck, spellStatus } from './spellcheck'
import { describeStatus, type SpellStatus } from './status'
import { theme } from './theme'
import './style.css'

const sample = `Welcome! This editor checks your speling as you type. Hover over or tap an underlined word to see sugestions, or press F8 to jump to the next one.

Teh first thing i noticed in london last monday was how quiet it was. I recieve postcards from friends there every month, and I definately want to go back.

Names the dictionary doesn't know, like Nhost, can be added from the same menu. Dont worry about links like https://nhost.io, numbers like 3rd, or acronyms like NASA: they are skipped on purpose.`

const view = new EditorView({
  parent: byId('editor'),
  doc: loadDraft() ?? sample,
  extensions: [
    history(),
    keepDraft(),
    drawSelection(),
    EditorView.lineWrapping,
    EditorView.contentAttributes.of({ 'aria-label': 'Text to spell-check' }),
    EditorState.phrases.of({ Diagnostics: 'Spelling issues', 'No diagnostics': 'No spelling issues' }),
    placeholder('Start writing…'),
    // The suggestion menu stays below the header but may use the rest of the
    // window, so the issues panel doesn't squeeze it.
    tooltips({
      tooltipSpace: (view) => ({ top: view.scrollDOM.getBoundingClientRect().top, left: 0, right: innerWidth, bottom: innerHeight }),
    }),
    keymap.of([{ key: 'Escape', run: closeSuggestions }, ...defaultKeymap, ...historyKeymap, ...lintKeymap]),
    spellcheck(checkWords),
    statusLine(byId('status')),
    theme,
  ],
})
// On a phone, focusing would raise the keyboard before anything is read.
if (!matchMedia('(pointer: coarse)').matches) view.focus()

if (/Mac|iPhone|iPad/.test(navigator.platform)) byId('issues-key').textContent = '⇧⌘M'

/** Escape closes an open suggestion menu, as it would any menu. */
function closeSuggestions(view: EditorView): boolean {
  if (!hasHoverTooltips(view.state)) return false
  view.dispatch({ effects: closeHoverTooltips })
  return true
}

/** Keeps the header's status in step with the editor. */
function statusLine(element: HTMLElement) {
  const render = (status: SpellStatus) => {
    const [state, text] = describeStatus(status)
    // Only touch the live region when the message changes; the count still
    // changes on most pauses, which screen readers announce.
    if (element.textContent === text) return
    element.textContent = text
    element.dataset.state = state
  }
  return ViewPlugin.define((view) => {
    render(spellStatus(view.state))
    return { update: (update) => render(spellStatus(update.state)) }
  })
}

function byId(id: string): HTMLElement {
  const element = document.getElementById(id)
  if (!element) throw new Error(`#${id} is missing from index.html`)
  return element
}
