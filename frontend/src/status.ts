export interface SpellStatus {
  /**
   * 'checking' until the API first answers, 'offline' while it can't be
   * reached, 'rejected' when it refused some of the words on screen.
   */
  connection: 'checking' | 'online' | 'offline' | 'rejected'
  /** How many words are underlined right now. */
  issues: number
}

/** The status line's state, for styling, and its text. */
export function describeStatus({ connection, issues }: SpellStatus): [state: string, text: string] {
  if (connection === 'checking') return ['checking', 'Checking…']
  if (connection === 'offline') return ['offline', 'Spell checker offline, retrying']
  const count = issues === 0 ? 'No spelling issues' : `${issues} spelling issue${issues === 1 ? '' : 's'}`
  if (connection === 'rejected') return ['rejected', `${count}, some words not checked`]
  return [issues === 0 ? 'clean' : 'issues', count]
}
