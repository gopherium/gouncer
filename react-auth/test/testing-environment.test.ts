// SPDX-License-Identifier: Apache-2.0

import { expect, onTestFinished, test, vi } from 'vitest'

import { HttpResponse, http, server } from '../src/testing'
import { keptFile } from './setup'

/** Returns a form holding a five megabyte file. */
function largeForm() {
	const body = new FormData()
	body.append('file', new File([new Uint8Array(5_000_000)], 'harbor.jpg'))
	return body
}

test('fails a request no handler answers and names it on the console', async () => {
	const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
	onTestFinished(() => logged.mockRestore())

	await expect(fetch('/api/unanswered')).rejects.toThrow()

	expect(logged).toHaveBeenCalledWith(expect.stringContaining('GET /api/unanswered'))
})

test('answers from a handler a test adds', async () => {
	server.use(http.get('/api/once', () => new HttpResponse(null, { status: 204 })))

	await expect(fetch('/api/once')).resolves.toHaveProperty('status', 204)
})

test('drops the handlers of the test before', async () => {
	const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
	onTestFinished(() => logged.mockRestore())

	await expect(fetch('/api/once')).rejects.toThrow()
})

test('hands an uploaded file to its handler with its name and bytes', async () => {
	let received = new File([], 'nothing')
	server.use(
		http.post('/api/uploads', async ({ request }) => {
			received = (await request.formData()).get('file') as File
			return new HttpResponse(null, { status: 204 })
		}),
	)
	const body = new FormData()
	body.append('file', new File(['the bytes'], 'harbor.jpg'))

	await fetch('/api/uploads', { method: 'POST', body })

	expect(received.name).toBe('harbor.jpg')
	await expect(received.text()).resolves.toBe('the bytes')
})

test('lets a File class kept before the install recognise a test file and nothing else', () => {
	expect(new File(['the bytes'], 'harbor.jpg')).toBeInstanceOf(keptFile)
	expect({}).not.toBeInstanceOf(keptFile)
})

test('answers a request left in flight from the handlers of the test that sent it', () => {
	server.use(http.post('/api/left', () => new HttpResponse(null, { status: 204 })))

	const left = fetch('/api/left', { method: 'POST' })

	onTestFinished(() => expect(left).resolves.toHaveProperty('status', 204))
})

test.each(Array.from({ length: 20 }, (_, round) => round))(
	'answers a large upload left in flight after a finished one, round %i',
	async () => {
		server.use(
			http.post('/api/uploads', async ({ request }) => {
				await request.formData()
				return new HttpResponse(null, { status: 204 })
			}),
		)
		await (await fetch('/api/uploads', { method: 'POST', body: largeForm() })).text()

		const left = fetch('/api/uploads', { method: 'POST', body: largeForm() })

		onTestFinished(() => expect(left).resolves.toHaveProperty('status', 204))
	},
)

test('ends a test that leaves fake timers on', () => {
	vi.useFakeTimers()
	onTestFinished(() => {
		vi.useRealTimers()
	})

	expect(vi.isFakeTimers()).toBe(true)
})
