// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import {
	InvalidCredentialsError,
	RateLimitedError,
	fetchSession,
	login,
	logout,
} from '../src/api'
import {
	HttpResponse,
	http,
	loginFailure,
	loginInvalid,
	loginRateLimited,
	logoutFailure,
	sessionAnonymous,
	sessionFailure,
	sessionOk,
	server,
} from '../src/testing'

const ada = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'ada@example.com',
	name: 'Ada Lovelace',
}

describe('fetchSession', () => {
	it('returns the logged-in user', async () => {
		server.use(sessionOk(ada))

		await expect(fetchSession()).resolves.toEqual(ada)
	})

	it('returns null when there is no session', async () => {
		server.use(sessionAnonymous())

		await expect(fetchSession()).resolves.toBeNull()
	})

	it('rejects on server failure', async () => {
		server.use(sessionFailure())

		await expect(fetchSession()).rejects.toThrow('500')
	})
})

describe('login', () => {
	it('posts the credentials and returns the user', async () => {
		let body: unknown
		server.use(
			http.post('/api/auth/login', async ({ request }) => {
				body = await request.json()
				return HttpResponse.json(ada)
			}),
		)

		await expect(login('ada@example.com', 'correct horse battery')).resolves.toEqual(ada)
		expect(body).toEqual({
			email: 'ada@example.com',
			password: 'correct horse battery',
		})
	})

	it('throws InvalidCredentialsError on rejected credentials', async () => {
		server.use(loginInvalid())

		await expect(login('ada@example.com', 'wrong')).rejects.toBeInstanceOf(
			InvalidCredentialsError,
		)
	})

	it('throws RateLimitedError when the client is rate limited', async () => {
		server.use(loginRateLimited())

		await expect(
			login('ada@example.com', 'correct horse battery'),
		).rejects.toBeInstanceOf(RateLimitedError)
	})

	it('rejects on server failure', async () => {
		server.use(loginFailure())

		await expect(login('ada@example.com', 'correct horse battery')).rejects.toThrow('500')
	})
})

describe('logout', () => {
	it('posts to the logout endpoint', async () => {
		let called = false
		server.use(
			http.post('/api/auth/logout', () => {
				called = true
				return new HttpResponse(null, { status: 204 })
			}),
		)

		await logout()
		expect(called).toBe(true)
	})

	it('rejects on server failure', async () => {
		server.use(logoutFailure())

		await expect(logout()).rejects.toThrow('500')
	})
})
