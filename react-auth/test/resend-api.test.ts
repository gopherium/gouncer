// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from 'vitest'

import { UserNotEditableError, ValidationError, resendInvite } from '../src/admin'
import { RateLimitedError, UnauthorizedError, configureAuthTransport, resetAuthTransport } from '../src/index'
import { HttpResponse, http, server } from '../src/testing'

afterEach(() => {
	resetAuthTransport()
})

describe('resendInvite', () => {
	it('posts the address and answers the delivery report', async () => {
		let body: unknown
		server.use(
			http.post('/api/users/invite/resend', async ({ request }) => {
				body = await request.json()
				return HttpResponse.json({ delivered: true })
			}),
		)

		await expect(resendInvite('maria@example.com')).resolves.toEqual({ delivered: true })
		expect(body).toEqual({ email: 'maria@example.com' })
	})

	it('hands its signal to the request, which stops once the signal fires', async () => {
		let seen: AbortSignal | undefined
		server.use(
			http.post('/api/users/invite/resend', ({ request }) => {
				seen = request.signal
				return new Promise<never>(() => {})
			}),
		)
		const controller = new AbortController()
		const sending = resendInvite('maria@example.com', controller.signal)
		await vi.waitFor(() => expect(seen).toBeDefined())

		controller.abort(new DOMException('The write ran out of time.', 'TimeoutError'))

		await vi.waitFor(() => expect(seen?.aborted).toBe(true))
		await expect(sending).rejects.toMatchObject({ name: 'TimeoutError' })
	})

	it('answers the by-hand link when the server has no mailer', async () => {
		server.use(
			http.post('/api/users/invite/resend', () =>
				HttpResponse.json({ delivered: false, activation_link: 'https://example.com/activate?token=t-456' }),
			),
		)

		await expect(resendInvite('maria@example.com')).resolves.toEqual({
			delivered: false,
			activation_link: 'https://example.com/activate?token=t-456',
		})
	})

	it('rejects an undelivered report carrying no link', async () => {
		server.use(http.post('/api/users/invite/resend', () => HttpResponse.json({ delivered: false })))

		await expect(resendInvite('maria@example.com')).rejects.toThrow()
	})

	it('throws UnauthorizedError without a session', async () => {
		server.use(
			http.post('/api/users/invite/resend', () => HttpResponse.json({ error: 'no session' }, { status: 401 })),
		)

		await expect(resendInvite('maria@example.com')).rejects.toBeInstanceOf(UnauthorizedError)
	})

	it('throws RateLimitedError when throttled', async () => {
		server.use(
			http.post('/api/users/invite/resend', () => HttpResponse.json({ error: 'slow down' }, { status: 429 })),
		)

		await expect(resendInvite('maria@example.com')).rejects.toBeInstanceOf(RateLimitedError)
	})

	it('throws UserNotEditableError for an account the reader may not edit', async () => {
		server.use(
			http.post('/api/users/invite/resend', () =>
				HttpResponse.json(
					{ error: 'Sorry, you are not allowed to edit this user.', code: 'user_not_editable' },
					{ status: 403 },
				),
			),
		)

		await expect(resendInvite('maria@example.com')).rejects.toBeInstanceOf(UserNotEditableError)
	})

	it('throws ValidationError with the backend message', async () => {
		server.use(
			http.post('/api/users/invite/resend', () =>
				HttpResponse.json({ error: 'invalid email address', code: 'email_invalid' }, { status: 422 }),
			),
		)

		const attempt = resendInvite('not an address')

		await expect(attempt).rejects.toBeInstanceOf(ValidationError)
		await expect(attempt).rejects.toMatchObject({ code: 'email_invalid', message: 'invalid email address' })
	})

	it('names the write and the status for any other failure', async () => {
		server.use(
			http.post('/api/users/invite/resend', () => HttpResponse.json({ error: 'internal error' }, { status: 500 })),
		)

		await expect(resendInvite('maria@example.com')).rejects.toThrow('resending invitation failed with status 500')
	})
})

describe('resendInvite through a configured transport', () => {
	it('carries the address to the transport and answers its report', async () => {
		const injected = vi.fn().mockResolvedValue({ delivered: true })
		configureAuthTransport({ resendInvite: injected })

		await expect(resendInvite('maria@example.com')).resolves.toEqual({ delivered: true })
		expect(injected).toHaveBeenCalledWith('maria@example.com', undefined)
	})

	it('carries the signal to the transport', async () => {
		const injected = vi.fn().mockResolvedValue({ delivered: true })
		configureAuthTransport({ resendInvite: injected })
		const { signal } = new AbortController()

		await resendInvite('maria@example.com', signal)

		expect(injected).toHaveBeenCalledWith('maria@example.com', signal)
	})

	it('holds the transport to the invitation contract', async () => {
		configureAuthTransport({
			resendInvite: () => Promise.resolve({ delivered: false, activation_link: '' }),
		})

		await expect(resendInvite('maria@example.com')).rejects.toThrow()
	})
})
