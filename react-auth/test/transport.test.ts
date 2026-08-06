// SPDX-License-Identifier: Apache-2.0

import { afterEach, expect, test, vi } from 'vitest'

import { createUser, fetchUsers } from '../src/admin'
import { configureAuthTransport, fetchSession, login, resetAuthTransport } from '../src/index'
import { defaultUser, loginOk, server, sessionOk } from '../src/testing'

afterEach(() => {
	resetAuthTransport()
})

test('a configured transport carries the session operations', async () => {
	const injectedUser = { id: 'u1', email: 'maria@example.com', name: 'Maria Perez' }
	const injectedLogin = vi.fn().mockResolvedValue(injectedUser)
	const injectedSession = vi.fn().mockResolvedValue(injectedUser)
	configureAuthTransport({ login: injectedLogin, fetchSession: injectedSession })

	await expect(login('maria@example.com', 'password1234')).resolves.toEqual(injectedUser)
	await expect(fetchSession()).resolves.toEqual(injectedUser)

	expect(injectedLogin).toHaveBeenCalledWith('maria@example.com', 'password1234')
	expect(injectedSession).toHaveBeenCalledOnce()
})

test('a configured transport carries the admin operations', async () => {
	const injectedUsers = vi.fn().mockResolvedValue([])
	const injectedCreate = vi.fn().mockResolvedValue({
		id: 'u2',
		email: 'maria@example.com',
		name: 'Maria Perez',
		disabled: false,
		created_at: new Date(),
	})
	configureAuthTransport({ fetchUsers: injectedUsers, createUser: injectedCreate })

	await expect(fetchUsers()).resolves.toEqual([])
	await createUser({ email: 'maria@example.com', name: 'Maria Perez', password: 'password1234' })

	expect(injectedUsers).toHaveBeenCalledOnce()
	expect(injectedCreate).toHaveBeenCalledOnce()
})

test('unconfigured operations keep using the REST transport', async () => {
	server.use(sessionOk(), loginOk())
	configureAuthTransport({ fetchUsers: vi.fn().mockResolvedValue([]) })

	await expect(fetchSession()).resolves.toEqual(defaultUser)
	await expect(login(defaultUser.email, 'password1234')).resolves.toEqual(defaultUser)
})

test('resetAuthTransport restores the REST default', async () => {
	server.use(sessionOk())
	configureAuthTransport({ fetchSession: vi.fn().mockResolvedValue(null) })

	await expect(fetchSession()).resolves.toBeNull()

	resetAuthTransport()
	await expect(fetchSession()).resolves.toEqual(defaultUser)
})
