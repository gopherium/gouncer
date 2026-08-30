// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import {
	InvalidTokenError,
	RateLimitedError,
	acceptInvite,
	requestPasswordReset,
	resetPassword,
} from '../src/api'
import {
	HttpResponse,
	activateInvalidToken,
	activateOk,
	defaultUser,
	http,
	resetInvalidToken,
	resetOk,
	resetRequestOk,
	resetRequestRateLimited,
	server,
} from '../src/testing'

describe('acceptInvite', () => {
	it('posts the token and password and answers the signed-in user', async () => {
		let body: unknown
		server.use(
			http.post('/api/auth/activate', async ({ request }) => {
				body = await request.json()
				return HttpResponse.json(defaultUser)
			}),
		)

		await expect(acceptInvite('t-123', 'correct horse battery')).resolves.toEqual(defaultUser)
		expect(body).toEqual({ token: 't-123', password: 'correct horse battery' })
	})

	it('resolves through the canned handler', async () => {
		server.use(activateOk())

		await expect(acceptInvite('t-123', 'correct horse battery')).resolves.toEqual(defaultUser)
	})

	it('throws InvalidTokenError for a dead link', async () => {
		server.use(activateInvalidToken())

		await expect(acceptInvite('t-spent', 'correct horse battery')).rejects.toBeInstanceOf(
			InvalidTokenError,
		)
	})

	it('throws ValidationError with the backend message for a weak password', async () => {
		server.use(
			http.post('/api/auth/activate', () =>
				HttpResponse.json({ error: 'the password is too short' }, { status: 422 }),
			),
		)

		await expect(acceptInvite('t-123', 'tiny')).rejects.toThrow('the password is too short')
	})

	it('falls back to a readable message when the refusal body is not JSON', async () => {
		server.use(http.post('/api/auth/activate', () => new HttpResponse('boom', { status: 422 })))

		await expect(acceptInvite('t-123', 'correct horse battery')).rejects.toThrow(
			'invalid password details',
		)
	})

	it('throws RateLimitedError when throttled', async () => {
		server.use(
			http.post('/api/auth/activate', () =>
				HttpResponse.json({ error: 'slow down' }, { status: 429 }),
			),
		)

		await expect(acceptInvite('t-123', 'correct horse battery')).rejects.toBeInstanceOf(
			RateLimitedError,
		)
	})

	it('rejects on server failure', async () => {
		server.use(
			http.post('/api/auth/activate', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(acceptInvite('t-123', 'correct horse battery')).rejects.toThrow('500')
	})
})

describe('requestPasswordReset', () => {
	it('posts the email and resolves on the neutral answer', async () => {
		let body: unknown
		server.use(
			http.post('/api/auth/password-reset', async ({ request }) => {
				body = await request.json()
				return new HttpResponse(null, { status: 204 })
			}),
		)

		await expect(requestPasswordReset('maria@example.com')).resolves.toBeUndefined()
		expect(body).toEqual({ email: 'maria@example.com' })
	})

	it('resolves through the canned handler', async () => {
		server.use(resetRequestOk())

		await expect(requestPasswordReset('maria@example.com')).resolves.toBeUndefined()
	})

	it('throws RateLimitedError when throttled', async () => {
		server.use(resetRequestRateLimited())

		await expect(requestPasswordReset('maria@example.com')).rejects.toBeInstanceOf(
			RateLimitedError,
		)
	})

	it('rejects on server failure', async () => {
		server.use(
			http.post('/api/auth/password-reset', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(requestPasswordReset('maria@example.com')).rejects.toThrow('500')
	})
})

describe('resetPassword', () => {
	it('posts the token and password and resolves', async () => {
		let body: unknown
		server.use(
			http.post('/api/auth/password-reset/confirm', async ({ request }) => {
				body = await request.json()
				return new HttpResponse(null, { status: 204 })
			}),
		)

		await expect(resetPassword('t-123', 'another good password')).resolves.toBeUndefined()
		expect(body).toEqual({ token: 't-123', password: 'another good password' })
	})

	it('resolves through the canned handler', async () => {
		server.use(resetOk())

		await expect(resetPassword('t-123', 'another good password')).resolves.toBeUndefined()
	})

	it('throws InvalidTokenError for a dead link', async () => {
		server.use(resetInvalidToken())

		await expect(resetPassword('t-spent', 'another good password')).rejects.toBeInstanceOf(
			InvalidTokenError,
		)
	})

	it('throws ValidationError with the backend message for a weak password', async () => {
		server.use(
			http.post('/api/auth/password-reset/confirm', () =>
				HttpResponse.json({ error: 'the password is too short' }, { status: 422 }),
			),
		)

		await expect(resetPassword('t-123', 'tiny')).rejects.toThrow('the password is too short')
	})

	it('throws RateLimitedError when throttled', async () => {
		server.use(
			http.post('/api/auth/password-reset/confirm', () =>
				HttpResponse.json({ error: 'slow down' }, { status: 429 }),
			),
		)

		await expect(resetPassword('t-123', 'another good password')).rejects.toBeInstanceOf(
			RateLimitedError,
		)
	})

	it('rejects on server failure', async () => {
		server.use(
			http.post('/api/auth/password-reset/confirm', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(resetPassword('t-123', 'another good password')).rejects.toThrow('500')
	})
})
