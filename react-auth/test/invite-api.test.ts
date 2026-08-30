// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import { EmailTakenError, ValidationError, invite } from '../src/admin'
import { RateLimitedError, UnauthorizedError } from '../src/api'
import {
	HttpResponse,
	http,
	inviteDelivered,
	inviteFailure,
	inviteUndelivered,
	server,
} from '../src/testing'

describe('invite', () => {
	it('posts the invitation and answers the delivery report', async () => {
		let body: unknown
		server.use(
			http.post('/api/users/invite', async ({ request }) => {
				body = await request.json()
				return HttpResponse.json({ delivered: true })
			}),
		)

		await expect(
			invite({ email: 'maria@example.com', name: 'Maria Perez', role: 'member' }),
		).resolves.toEqual({ delivered: true })
		expect(body).toEqual({ email: 'maria@example.com', name: 'Maria Perez', role: 'member' })
	})

	it('answers the by-hand link when the server has no mailer', async () => {
		server.use(inviteUndelivered('https://crm.example.com/activate?token=t-123'))

		await expect(invite({ email: 'maria@example.com', name: 'Maria Perez' })).resolves.toEqual({
			delivered: false,
			activation_link: 'https://crm.example.com/activate?token=t-123',
		})
	})

	it('resolves through the canned delivered handler', async () => {
		server.use(inviteDelivered())

		await expect(
			invite({ email: 'maria@example.com', name: 'Maria Perez' }),
		).resolves.toEqual({ delivered: true })
	})

	it('never surfaces a taken address as EmailTakenError', async () => {
		server.use(
			http.post('/api/users/invite', () =>
				HttpResponse.json({ error: 'conflict' }, { status: 409 }),
			),
		)

		const attempt = invite({ email: 'maria@example.com', name: 'Maria Perez' })

		await expect(attempt).rejects.toThrow('409')
		await expect(attempt).rejects.not.toBeInstanceOf(EmailTakenError)
	})

	it('throws UnauthorizedError without a session', async () => {
		server.use(
			http.post('/api/users/invite', () =>
				HttpResponse.json({ error: 'no session' }, { status: 401 }),
			),
		)

		await expect(
			invite({ email: 'maria@example.com', name: 'Maria Perez' }),
		).rejects.toBeInstanceOf(UnauthorizedError)
	})

	it('throws ValidationError with the backend message', async () => {
		server.use(
			http.post('/api/users/invite', () =>
				HttpResponse.json({ error: 'the name is required' }, { status: 422 }),
			),
		)

		await expect(invite({ email: 'maria@example.com', name: '' })).rejects.toThrow(
			'the name is required',
		)
	})

	it('throws RateLimitedError when throttled', async () => {
		server.use(
			http.post('/api/users/invite', () =>
				HttpResponse.json({ error: 'slow down' }, { status: 429 }),
			),
		)

		await expect(
			invite({ email: 'maria@example.com', name: 'Maria Perez' }),
		).rejects.toBeInstanceOf(RateLimitedError)
	})

	it('falls back to a readable message when the refusal body is not JSON', async () => {
		server.use(
			http.post('/api/users/invite', () => new HttpResponse('boom', { status: 422 })),
		)

		await expect(
			invite({ email: 'maria@example.com', name: 'Maria Perez' }),
		).rejects.toBeInstanceOf(ValidationError)
	})

	it('rejects on server failure', async () => {
		server.use(inviteFailure())

		await expect(invite({ email: 'maria@example.com', name: 'Maria Perez' })).rejects.toThrow(
			'500',
		)
	})
})
