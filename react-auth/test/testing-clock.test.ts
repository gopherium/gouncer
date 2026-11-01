// SPDX-License-Identifier: Apache-2.0

import { expect, test, vi } from 'vitest'

vi.useFakeTimers()
vi.resetModules()
const { installTestEnvironment, server } = await import('../src/testing')
vi.spyOn(server, 'listen').mockImplementation(() => {})
vi.spyOn(server, 'close').mockImplementation(() => {})
installTestEnvironment()
vi.useRealTimers()

test('ends a test when the harness loaded and installed under fake timers', () => {
	expect(vi.isFakeTimers()).toBe(false)
})
