import { describe, expect, it } from 'vitest'
import { describeStatus } from './status'

describe('describeStatus', () => {
  it('reports the connection before the count', () => {
    expect(describeStatus({ connection: 'checking', issues: 3 })).toEqual(['checking', 'Checking…'])
    expect(describeStatus({ connection: 'offline', issues: 3 })).toEqual(['offline', 'Spell checker offline, retrying'])
  })

  it('counts the issues once online', () => {
    expect(describeStatus({ connection: 'online', issues: 0 })).toEqual(['clean', 'No spelling issues'])
    expect(describeStatus({ connection: 'online', issues: 1 })).toEqual(['issues', '1 spelling issue'])
    expect(describeStatus({ connection: 'online', issues: 2 })).toEqual(['issues', '2 spelling issues'])
  })

  it('keeps the count when some words were refused', () => {
    expect(describeStatus({ connection: 'rejected', issues: 2 })).toEqual(['rejected', '2 spelling issues, some words not checked'])
    expect(describeStatus({ connection: 'rejected', issues: 0 })).toEqual(['rejected', 'No spelling issues, some words not checked'])
  })
})
