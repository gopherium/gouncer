// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import {
	LastPrivilegedError,
	RankRefusedError,
	SelfRankError,
	createUser,
	fetchUsers,
	setUserRank,
} from '../src/admin'
import { UnauthorizedError, fetchSession } from '../src/api'
import { HttpResponse, http, server } from '../src/testing'

const maria = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'maria@example.com',
	name: 'Maria Perez',
	disabled: false,
	rank: 'editor',
	created_at: '2026-07-06T10:00:00Z',
}

describe('the rank on a session', () => {
	it('carries the rank the server sends', async () => {
		server.use(
			http.get('/api/auth/session', () =>
				HttpResponse.json({ id: maria.id, email: maria.email, name: maria.name, rank: 'admin' }),
			),
		)

		const held = await fetchSession()

		expect(held?.rank).toBe('admin')
	})

	it('reads a server that sends no rank as unranked', async () => {
		server.use(
			http.get('/api/auth/session', () =>
				HttpResponse.json({ id: maria.id, email: maria.email, name: maria.name }),
			),
		)

		const held = await fetchSession()

		expect(held?.rank).toBe('')
	})
})

describe('the rank on a listing', () => {
	it('carries the rank each account holds', async () => {
		server.use(http.get('/api/users', () => HttpResponse.json([maria])))

		const listed = await fetchUsers()

		expect(listed[0].rank).toBe('editor')
	})

	it('reads an account without a rank as unranked', async () => {
		const { rank: _rank, ...rankless } = maria
		server.use(http.get('/api/users', () => HttpResponse.json([rankless])))

		const listed = await fetchUsers()

		expect(listed[0].rank).toBe('')
	})
})

describe('createUser', () => {
	it('names the rank the account starts under', async () => {
		let sent: unknown
		server.use(
			http.post('/api/users', async ({ request }) => {
				sent = await request.json()
				return HttpResponse.json(maria, { status: 201 })
			}),
		)

		await createUser({
			email: maria.email,
			name: maria.name,
			password: 'correct horse battery',
			rank: 'editor',
		})

		expect(sent).toMatchObject({ rank: 'editor' })
	})
})

describe('setUserRank', () => {
	it('writes the rank through the rank route', async () => {
		let seen = { url: '', body: {} as unknown }
		server.use(
			http.put('/api/users/:id/rank', async ({ request, params }) => {
				seen = { url: String(params.id), body: await request.json() }
				return new HttpResponse(null, { status: 204 })
			}),
		)

		await setUserRank(maria.id, 'admin')

		expect(seen.url).toBe(maria.id)
		expect(seen.body).toEqual({ rank: 'admin' })
	})

	it('reports an actor holding no privileged rank', async () => {
		server.use(
			http.put('/api/users/:id/rank', () =>
				HttpResponse.json({ error: 'rank insufficient', code: 'rank_insufficient' }, { status: 403 }),
			),
		)

		await expect(setUserRank(maria.id, 'admin')).rejects.toBeInstanceOf(RankRefusedError)
	})

	it('reports an actor changing its own rank', async () => {
		server.use(
			http.put('/api/users/:id/rank', () =>
				HttpResponse.json(
					{ error: 'cannot change your own rank', code: 'self_rank_refused' },
					{ status: 422 },
				),
			),
		)

		await expect(setUserRank(maria.id, 'editor')).rejects.toBeInstanceOf(SelfRankError)
	})

	it('reports a write leaving no privileged account', async () => {
		server.use(
			http.put('/api/users/:id/rank', () =>
				HttpResponse.json(
					{ error: 'the last privileged account must remain', code: 'last_privileged_refused' },
					{ status: 422 },
				),
			),
		)

		await expect(setUserRank(maria.id, 'editor')).rejects.toBeInstanceOf(LastPrivilegedError)
	})
})

describe('setUserRank when the response is not a refusal it knows', () => {
	it('reports an expired session', async () => {
		server.use(
			http.put('/api/users/:id/rank', () =>
				HttpResponse.json({ error: 'no session' }, { status: 401 }),
			),
		)

		await expect(setUserRank(maria.id, 'admin')).rejects.toBeInstanceOf(UnauthorizedError)
	})

	it('reports a status carrying no refusal code', async () => {
		server.use(
			http.put('/api/users/:id/rank', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(setUserRank(maria.id, 'admin')).rejects.toThrow(/500/)
	})

	it('reports a refusal whose body cannot be read', async () => {
		server.use(
			http.put('/api/users/:id/rank', () =>
				new HttpResponse('not json at all', { status: 422 }),
			),
		)

		await expect(setUserRank(maria.id, 'admin')).rejects.toThrow(/422/)
	})
})
