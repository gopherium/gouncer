// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import { setUserRole } from '../src/admin'
import { fetchSession } from '../src/api'
import { defaultUser, userWithRole, roleOk, server } from '../src/testing'

describe('the canned accounts', () => {
	it('leaves the default account holding no role', () => {
		expect(defaultUser.role).toBe('')
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
