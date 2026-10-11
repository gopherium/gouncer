// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import { can } from '../src'
import { setUserRole } from '../src/admin'
import { fetchSession } from '../src/api'
import { defaultUser, userWithCapabilities, userWithRole, roleOk, server, sessionOk } from '../src/testing'

describe('the canned accounts', () => {
	it('leaves the default account holding no role', () => {
		expect(defaultUser.role).toBe('')
	})

	it('keeps the default account under its name, address and identifier', () => {
		expect(defaultUser).toEqual({
			id: '0198b2f0-0000-7000-8000-000000000001',
			email: 'grace@example.com',
			name: 'Grace Hopper',
			role: '',
		})
	})

	it('keeps a role account to its four keys', () => {
		expect(Object.keys(userWithRole('editor')).sort()).toEqual(['email', 'id', 'name', 'role'])
	})

	it('cans an account holding capabilities and the roles it may hand out', () => {
		const editor = userWithCapabilities('editor', ['posts.edit'], ['author'])

		expect(editor).toEqual({ ...userWithRole('editor'), capabilities: ['posts.edit'], grantable: ['author'] })
		expect(can(editor, 'posts.edit')).toBe(true)
	})

	it('cans an account that may hand out no role', () => {
		expect(userWithCapabilities('member', []).grantable).toEqual([])
	})

	it('serves a canned account with its capabilities on the session', async () => {
		server.use(sessionOk(userWithCapabilities('admin', ['users.manage'])))

		const held = await fetchSession()

		expect(can(held, 'users.manage')).toBe(true)
	})

	it('cans an account under any role asked for', () => {
		const editor = userWithRole('editor')

		expect(editor.role).toBe('editor')
		expect(editor.id).not.toBe(defaultUser.id)
	})

	it('cans two roles apart, so a gate can be walked from both sides', () => {
		expect(userWithRole('admin').id).not.toBe(userWithRole('editor').id)
	})
})

describe('roleOk', () => {
	it('answers a role change', async () => {
		server.use(roleOk())

		await expect(setUserRole(defaultUser.id, 'admin')).resolves.toBeUndefined()
	})
})

describe('a canned session', () => {
	it('carries the role of the account it belongs to', async () => {
		const { sessionOk } = await import('../src/testing')
		server.use(sessionOk(userWithRole('admin')))

		const held = await fetchSession()

		expect(held?.role).toBe('admin')
	})
})

describe('a canned account identifier', () => {
	it('differs for roles holding the same letters in another order', () => {
		expect(userWithRole('ab').id).not.toBe(userWithRole('ba').id)
	})

	it('differs for every role a consumer may name', () => {
		const roles = ['admin', 'editor', 'author', 'contributor', 'subscriber']
		const ids = new Set(roles.map((role) => userWithRole(role).id))

		expect(ids.size).toBe(roles.length)
	})
})
