// SPDX-License-Identifier: Apache-2.0

import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { beforeEach, describe, expect, it, test } from 'vitest'

import { ValidationError, createUser, invite, resendInvite, setUserDisabled, setUserRole } from '../src/admin'
import { server } from '../src/testing'
import { errorMessageOf } from '../src/wpds/users'
import { act, barred, colleague, holding, renderUsers, signedIn, toast } from './users-harness'

const id = '0198b2f0-0000-7000-8000-000000000002'

const appWords = { capability_missing: 'You need the manage users capability.' }

const missing = { error: 'capability missing', code: 'capability_missing' }

const lastPrivileged = { code: 'last_privileged_refused', words: 'Somebody must be able to manage users.' }

const writes = [
	{
		name: 'createUser',
		failure: 'creating user',
		handler: (response: () => Response) => http.post('/api/users', response),
		run: () => createUser({ email: 'maria@example.com', name: 'Maria Perez', password: 'correct horse battery' }),
		worded: lastPrivileged,
	},
	{
		name: 'invite',
		failure: 'inviting',
		handler: (response: () => Response) => http.post('/api/users/invite', response),
		run: () => invite({ email: 'maria@example.com', name: 'Maria Perez' }),
		worded: lastPrivileged,
	},
	{
		name: 'resendInvite',
		failure: 'resending invitation',
		handler: (response: () => Response) => http.post('/api/users/invite/resend', response),
		run: () => resendInvite('maria@example.com'),
		worded: lastPrivileged,
	},
	{
		name: 'setUserRole',
		failure: 'updating role',
		handler: (response: () => Response) => http.put('/api/users/:id/role', response),
		run: () => setUserRole(id, 'member'),
		worded: { code: 'self_disable_refused', words: 'You cannot disable your own account.' },
	},
	{
		name: 'setUserDisabled',
		failure: 'updating user',
		handler: (response: () => Response) => http.patch('/api/users/:id', response),
		run: () => setUserDisabled(id, true),
		worded: { code: 'self_role_refused', words: 'You cannot change your own role.' },
	},
]

async function rejectionOf(attempt: Promise<unknown>): Promise<unknown> {
	return attempt.then(
		() => undefined,
		(held: unknown) => held,
	)
}

describe.each(writes)('$name over REST', ({ failure, handler, run, worded }) => {
	it('keeps a code it has no class for on the plain error, so the words the app gives it show', async () => {
		server.use(handler(() => HttpResponse.json(missing, { status: 403 })))

		const thrown = await rejectionOf(run())

		expect(thrown).toMatchObject({ code: 'capability_missing', message: `${failure} failed with status 403` })
		expect(thrown).not.toBeInstanceOf(ValidationError)
		expect(errorMessageOf(thrown, appWords, 'Fallback.')).toBe('You need the manage users capability.')
	})

	it('keeps a code it words but throws no class for, so its own words show', async () => {
		server.use(handler(() => HttpResponse.json({ error: 'odd', code: worded.code }, { status: 403 })))

		const thrown = await rejectionOf(run())

		expect(thrown).toMatchObject({ code: worded.code, message: `${failure} failed with status 403` })
		expect(errorMessageOf(thrown, appWords, 'Fallback.')).toBe(worded.words)
	})

	it.each([
		['a body naming no code', () => HttpResponse.json({ error: 'internal error' }, { status: 500 })],
		['an unreadable body', () => new HttpResponse('not json at all', { status: 500 })],
	])('throws a plain error carrying no code for %s, so the fallback shows', async (_name, response) => {
		server.use(handler(response))

		const thrown = await rejectionOf(run())

		expect(thrown).toBeInstanceOf(Error)
		expect(thrown).not.toHaveProperty('code')
		expect(errorMessageOf(thrown, appWords, 'Fallback.')).toBe('Fallback.')
	})
})

describe('the Users page on the REST default', () => {
	beforeEach(() => holding([signedIn, colleague, barred]))

	test('says the words the app gives a code the server answers a Disable with', async () => {
		server.use(http.patch('/api/users/:id', () => HttpResponse.json(missing, { status: 403 })))
		renderUsers({ errorMessages: appWords })

		await act('Ada Lovelace', 'Disable')

		expect(await toast('You need the manage users capability.')).toBeInTheDocument()
		expect(screen.queryByText('The user could not be disabled.')).not.toBeInTheDocument()
	})

	test('says the words the app gives a code the server answers a role change with', async () => {
		server.use(http.put('/api/users/:id/role', () => HttpResponse.json(missing, { status: 403 })))
		renderUsers({ errorMessages: appWords, roleLabels: { admin: 'Administrator', member: 'Member' } })

		await act('Ada Lovelace', 'Change role…')
		const dialog = within(await screen.findByRole('dialog', { name: 'Change role' }))
		await userEvent.click(dialog.getByRole('combobox', { name: 'Role' }))
		await userEvent.click(await screen.findByRole('option', { name: 'Administrator' }))
		await userEvent.click(dialog.getByRole('button', { name: 'Change role' }))

		expect(await toast('You need the manage users capability.')).toBeInTheDocument()
		expect(screen.queryByText('The role could not be changed.')).not.toBeInTheDocument()
	})
})
