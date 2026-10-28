const storageKey = 'spellcheck.personal-dictionary'

/**
 * Words the user added, saved in localStorage when the browser allows it.
 * Saving merges with what is already stored, so two open tabs keep each
 * other's words.
 */
export class PersonalDictionary {
  private readonly words = new Set<string>(load())

  has(word: string): boolean {
    return this.words.has(word.toLowerCase())
  }

  add(word: string): void {
    for (const stored of load()) this.words.add(stored)
    this.words.add(word.toLowerCase())
    try {
      localStorage.setItem(storageKey, JSON.stringify([...this.words]))
    } catch {
      // Storage is unavailable (private browsing, quota): keep it for this session.
    }
  }
}

function load(): string[] {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(storageKey) ?? '[]')
    return Array.isArray(stored) ? stored.filter((w): w is string => typeof w === 'string') : []
  } catch {
    return []
  }
}
