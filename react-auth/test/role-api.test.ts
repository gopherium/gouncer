// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import {
	LastPrivilegedError,
	RoleRefusedError,
	SelfRoleError,
	createUser,
	fetchUsers,
	setUserRole,
} from '../src/admin'
import { UnauthorizedError, fetchSession } from '../src/api'
import { HttpResponse, http, server } from '../src/testing'

const maria = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'maria@example.com',
	name: 'Maria Perez',
	disabled: false,
	role: 'editor',
	created_at: '2026-07-06T10:00:00Z',
}

describe('the role on a session', () => {
	it('carries the role the server sends', async () => {
		server.use(
			http.get('/api/auth/session', () =>
				HttpResponse.json({ id: maria.id, email: maria.email, name: maria.name, role: 'admin' }),
			),
		)

		const held = await fetchSession()

		expect(held?.role).toBe('admin')
	})

	it('reads a server that sends no role as holding none', async () => {
		server.use(
			http.get('/api/auth/session', () =>
				HttpResponse.json({ id: maria.id, email: maria.email, name: maria.name }),
			),
		)

		const held = await fetchSession()

		expect(held?.role).toBe('')
	})
})

describe('the role on a listing', () => {
	it('carries the role each account holds', async () => {
		server.use(http.get('/api/users', () => HttpResponse.json([maria])))

		const listed = await fetchUsers()

		expect(listed[0].role).toBe('editor')
	})

	it('reads an account without a role as holding none', async () => {
		const { role: _role, ...roleless } = maria
		server.use(http.get('/api/users', () => HttpResponse.json([roleless])))

		const listed = await fetchUsers()

		expect(listed[0].role).toBe('')
	})
})

describe('createUser', () => {
	it('names the role the account starts under', async () => {
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
			role: 'editor',
		})

		expect(sent).toMatchObject({ role: 'editor' })
	})
})

describe('setUserRole', () => {
	it('writes the role through the role route', async () => {
		let seen = { url: '', body: {} as unknown }
		server.use(
			http.put('/api/users/:id/role', async ({ request, params }) => {
				seen = { url: String(params.id), body: await request.json() }
				return new HttpResponse(null, { status: 204 })
			}),
		)

		await setUserRole(maria.id, 'admin')

		expect(seen.url).toBe(maria.id)
		expect(seen.body).toEqual({ role: 'admin' })
	})

	it('reports an actor holding no privileged role', async () => {
		server.use(
			http.put('/api/users/:id/role', () =>
				HttpResponse.json({ error: 'role insufficient', code: 'role_insufficient' }, { status: 403 }),
			),
		)

		await expect(setUserRole(maria.id, 'admin')).rejects.toBeInstanceOf(RoleRefusedError)
	})

	it('reports an actor changing its own role', async () => {
		server.use(
			http.put('/api/users/:id/role', () =>
				HttpResponse.json(
					{ error: 'cannot change your own role', code: 'self_role_refused' },
					{ status: 422 },
				),
			),
		)

		await expect(setUserRole(maria.id, 'editor')).rejects.toBeInstanceOf(SelfRoleError)
	})

	it('reports a write leaving no privileged account', async () => {
		server.use(
			http.put('/api/users/:id/role', () =>
				HttpResponse.json(
					{ error: 'the last privileged account must remain', code: 'last_privileged_refused' },
					{ status: 422 },
				),
			),
		)

		await expect(setUserRole(maria.id, 'editor')).rejects.toBeInstanceOf(LastPrivilegedError)
	})
})

describe('setUserRole when the response is not a refusal it knows', () => {
	it('reports an expired session', async () => {
		server.use(
			http.put('/api/users/:id/role', () =>
				HttpResponse.json({ error: 'no session' }, { status: 401 }),
			),
		)

		await expect(setUserRole(maria.id, 'admin')).rejects.toBeInstanceOf(UnauthorizedError)
	})

	it('reports a status carrying no refusal code', async () => {
		server.use(
			http.put('/api/users/:id/role', () =>
				HttpResponse.json({ error: 'internal error' }, { status: 500 }),
			),
		)

		await expect(setUserRole(maria.id, 'admin')).rejects.toThrow(/500/)
	})

	it('reports a refusal whose body cannot be read', async () => {
		server.use(
			http.put('/api/users/:id/role', () =>
				new HttpResponse('not json at all', { status: 422 }),
			),
		)

		await expect(setUserRole(maria.id, 'admin')).rejects.toThrow(/422/)
	})
})
