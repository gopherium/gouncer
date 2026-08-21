// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'

import { setUserRank } from '../src/admin'
import { fetchSession } from '../src/api'
import { defaultUser, rankedUser, rankOk, server } from '../src/testing'

describe('the canned accounts', () => {
	it('leaves the default account unranked', () => {
		expect(defaultUser.rank).toBe('')
	})

	it('cans an account under any rank asked for', () => {
		const editor = rankedUser('editor')

		expect(editor.rank).toBe('editor')
		expect(editor.id).not.toBe(defaultUser.id)
	})

	it('cans two ranks apart, so a gate can be walked from both sides', () => {
		expect(rankedUser('admin').id).not.toBe(rankedUser('editor').id)
	})
})

describe('rankOk', () => {
	it('answers a rank change', async () => {
		server.use(rankOk())

		await expect(setUserRank(defaultUser.id, 'admin')).resolves.toBeUndefined()
	})
})

describe('a canned session', () => {
	it('carries the rank of the account it belongs to', async () => {
		const { sessionOk } = await import('../src/testing')
		server.use(sessionOk(rankedUser('admin')))

		const held = await fetchSession()

		expect(held?.rank).toBe('admin')
	})
})
