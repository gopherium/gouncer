// SPDX-License-Identifier: Apache-2.0

import { expect, test, vi } from 'vitest'

import { installTestEnvironment, server } from '../src/testing'

vi.spyOn(server, 'listen').mockImplementation(() => {})
vi.spyOn(server, 'close').mockImplementation(() => {})
vi.useFakeTimers()
installTestEnvironment()
vi.useRealTimers()

test('ends a test when the install ran under fake timers', () => {
	expect(vi.isFakeTimers()).toBe(false)
})
