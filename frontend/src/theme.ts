import { EditorView } from '@codemirror/view'

// The same zigzag @codemirror/lint draws, in the palette's red, which reads
// on both light and dark backgrounds.
const squiggle = `url("data:image/svg+xml,${encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="6" height="3"><path d="m0 2.5 l2 -1.5 l1 0 l2 1.5 l1 0" fill="none" stroke="#e5484d" stroke-width=".8"/></svg>',
)}")`

// Padding that centers a column of at most 40rem: the content element still
// spans the full width, so a click in the margin lands in the text.
const column = 'max(1.25rem, calc(50% - 20rem))'

/**
 * Makes CodeMirror a writing surface rather than a code editor. Colors come
 * from the variables in style.css, so dark mode is one media query there.
 */
export const theme = EditorView.theme({
  '&': { height: '100%', color: 'var(--text)', backgroundColor: 'var(--page)' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { fontFamily: 'inherit', fontSize: '1.125rem', lineHeight: '1.75' },
  '.cm-content': { padding: `3.5rem ${column} 30vh` },
  '.cm-line': { padding: 0 },
  '.cm-cursor, .cm-dropCursor': { borderLeft: '2px solid var(--accent)' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection':
    { backgroundColor: 'var(--selection)' },
  '.cm-placeholder': { color: 'var(--faint)' },

  '.cm-lintRange-error': { backgroundImage: squiggle },
  '.cm-lintRange-active': { backgroundColor: 'var(--selection)' },

  // Hover tooltip: a small menu of suggestions.
  // CodeMirror caps the height when the menu doesn't fit; scroll, don't spill.
  '.cm-tooltip.cm-tooltip-hover': {
    minWidth: '12rem',
    overflowY: 'auto',
    padding: '0.25rem',
    border: '1px solid var(--border)',
    borderRadius: '10px',
    backgroundColor: 'var(--surface)',
    boxShadow: 'var(--shadow)',
  },
  '.cm-tooltip-lint .cm-diagnostic': { display: 'flex', flexDirection: 'column', margin: 0, padding: 0, border: 'none' },
  '.cm-tooltip-lint .cm-diagnosticText': { padding: '0.375rem 0.625rem 0.25rem', fontSize: '0.75rem', color: 'var(--muted)' },
  '.cm-tooltip-lint .cm-spellWord': { display: 'none' },
  '.cm-tooltip-lint .cm-diagnosticAction': {
    margin: 0,
    padding: '0.3125rem 0.625rem',
    borderRadius: '6px',
    backgroundColor: 'transparent',
    color: 'var(--text)',
    fontSize: '0.9375rem',
    fontWeight: 500,
    textAlign: 'left',
  },
  '.cm-tooltip-lint .cm-diagnosticAction:hover': { backgroundColor: 'var(--hover)' },
  '.cm-tooltip-lint .cm-addWord': { position: 'relative', marginTop: '0.5625rem', color: 'var(--muted)', fontWeight: 400 },
  '.cm-tooltip-lint .cm-addWord::before': {
    content: '""',
    position: 'absolute',
    top: '-0.3125rem',
    left: '0.625rem',
    right: '0.625rem',
    borderTop: '1px solid var(--border)',
  },
  '.cm-tooltip-lint .cm-diagnosticText + .cm-addWord': { marginTop: 0 },
  '.cm-tooltip-lint .cm-diagnosticText + .cm-addWord::before': { display: 'none' },

  // Ctrl/Cmd-Shift-M panel: one row per misspelling, aligned with the text.
  '.cm-panels': { backgroundColor: 'var(--surface)', color: 'var(--text)' },
  '.cm-panels.cm-panels-bottom': { borderTop: '1px solid var(--border)' },
  '.cm-panel.cm-panel-lint ul': { maxHeight: '13.5rem', padding: '0.375rem 0', outline: 'none' },
  '.cm-panel.cm-panel-lint ul [aria-selected]': { backgroundColor: 'var(--hover)' },
  '.cm-panel.cm-panel-lint ul:focus-visible': { outline: '2px solid var(--accent)', outlineOffset: '-2px' },
  '.cm-panel.cm-panel-lint ul:focus [aria-selected]': { backgroundColor: 'var(--selection)', color: 'var(--text)' },
  '.cm-panel-lint .cm-diagnostic': {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: '0.375rem',
    margin: 0,
    padding: `0.375rem ${column}`,
    border: 'none',
    fontSize: '0.875rem',
  },
  '.cm-panel-lint .cm-diagnosticText': { display: 'flex', gap: '0.5rem', minWidth: '6rem', marginRight: '0.375rem' },
  '.cm-panel-lint .cm-spellWord': { fontWeight: 600 },
  '.cm-panel-lint .cm-spellPrompt': { display: 'none' },
  '.cm-panel-lint .cm-spellNote': { color: 'var(--muted)' },
  '.cm-panel-lint .cm-diagnosticAction': {
    margin: 0,
    padding: '0.0625rem 0.625rem',
    border: '1px solid var(--border)',
    borderRadius: '999px',
    backgroundColor: 'var(--surface)',
    color: 'var(--text)',
  },
  '.cm-panel-lint .cm-addWord': { color: 'var(--muted)' },
  '.cm-panel.cm-panel-lint [name=close]': {
    top: '0.375rem',
    right: '0.75rem',
    color: 'var(--muted)',
    fontSize: '1.25rem',
    cursor: 'pointer',
  },
})
