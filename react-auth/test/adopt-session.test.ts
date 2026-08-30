// SPDX-License-Identifier: Apache-2.0

import { QueryClient } from '@tanstack/react-query'
import { expect, test } from 'vitest'

import { adoptSession, sessionQueryKey } from '../src/index'
import { defaultUser } from '../src/testing'

test('adoptSession seeds the session', async () => {
	const client = new QueryClient()

	await adoptSession(client, defaultUser)

	expect(client.getQueryData(sessionQueryKey)).toEqual(defaultUser)
})

test('adoptSession cancels an in-flight session fetch so it cannot clobber the seed', async () => {
	const client = new QueryClient()
	const outstanding = client.prefetchQuery({
		queryKey: sessionQueryKey,
		queryFn: async () => {
			await new Promise((resolve) => setTimeout(resolve, 50))
			return null
		},
	})

	await adoptSession(client, defaultUser)
	await outstanding
	await new Promise((resolve) => setTimeout(resolve, 80))

	expect(client.getQueryData(sessionQueryKey)).toEqual(defaultUser)
})
