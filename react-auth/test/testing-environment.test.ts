// SPDX-License-Identifier: Apache-2.0

import { expect, onTestFinished, test, vi } from 'vitest'

test('fails a request no handler answers and names it on the console', async () => {
	const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
	onTestFinished(() => logged.mockRestore())

	await expect(fetch('/api/unanswered')).rejects.toThrow()

	expect(logged).toHaveBeenCalledWith(expect.stringContaining('GET /api/unanswered'))
})
