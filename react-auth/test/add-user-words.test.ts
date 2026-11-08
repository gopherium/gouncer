// SPDX-License-Identifier: Apache-2.0

import { HttpResponse, http } from 'msw'
import { describe, expect, it, test } from 'vitest'

import { RoleNotGrantableError, RoleRefusedError, createUser, invite } from '../src/admin'
import { server } from '../src/testing'
import { addUserErrorMessageOf, errorMessageOf } from '../src/wpds/users'

const appWords = { capability_missing: 'You need the manage users capability.' }

const create = 'The user could not be created.'

const sends = [
	{
		name: 'createUser',
		path: '/api/users',
		run: () => createUser({ email: 'maria@example.com', name: 'Maria Perez', password: 'correct horse battery' }),
	},
	{
		name: 'invite',
		path: '/api/users/invite',
		run: () => invite({ email: 'maria@example.com', name: 'Maria Perez' }),
	},
]

async function refusalOf(attempt: Promise<unknown>): Promise<unknown> {
	return attempt.then(
		() => undefined,
		(held: unknown) => held,
	)
}

function refusing(path: string, code: string) {
	server.use(http.post(path, () => HttpResponse.json({ error: 'refused', code }, { status: 403 })))
}

describe.each(sends)('$name over REST', ({ path, run }) => {
	it("words the refusal of a reader who may no longer manage users as WordPress's Add User does", async () => {
		refusing(path, 'role_insufficient')

		const thrown = await refusalOf(run())

		expect(thrown).toMatchObject({ code: 'role_insufficient' })
		expect(addUserErrorMessageOf(thrown, appWords, create)).toBe('Sorry, you are not allowed to create users.')
		expect(errorMessageOf(thrown, appWords, create)).toBe('Sorry, you are not allowed to edit users.')
	})

	it('says the words the app gives a code only it answers, as the list does', async () => {
		refusing(path, 'capability_missing')

		const thrown = await refusalOf(run())

		expect(addUserErrorMessageOf(thrown, appWords, create)).toBe('You need the manage users capability.')
	})
})

test('words the refusal an app transport throws as its class as the create sentence too', () => {
	expect(addUserErrorMessageOf(new RoleRefusedError('role insufficient'), {}, create)).toBe(
		'Sorry, you are not allowed to create users.',
	)
})

test('reads every other refusal as the list reads it, and the fallback for an error with no code', () => {
	expect(addUserErrorMessageOf(new RoleNotGrantableError('role not grantable'), {}, create)).toBe(
		'Sorry, you are not allowed to give users that role.',
	)
	expect(addUserErrorMessageOf(new Error('500'), {}, create)).toBe(create)
})
