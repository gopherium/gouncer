// SPDX-License-Identifier: Apache-2.0

import { QueryClientProvider, hashKey } from '@tanstack/react-query'
import type { QueryClient, QueryKey } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import { createElement } from 'react'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import type { User } from '../src/api'
import { createAuthQueryClient } from '../src/queryClient'
import { adoptSession, sessionQueryKey, useLogout } from '../src/session'
import { defaultUser, logoutOk, seedSession, server, userWithRole } from '../src/testing'

/** otherAccount is an account whose id differs from the default one. */
const otherAccount: User = userWithRole('editor')

/** listKey is a read keyed without the account. */
const listKey = ['things'] as const

/** lastAccountRows is what the list read answered for the last account. */
const lastAccountRows = ['row of the last account']

/** keyUnderSessionKey is a read keyed by the session key and one more part. */
const keyUnderSessionKey = ['session', 'devices'] as const

/**
 * Hashes a key apart from the default hash.
 * @param queryKey - The key to hash.
 * @returns The default hash under a prefix.
 */
function prefixedKeyHash(queryKey: QueryKey): string {
	return `prefixed:${hashKey(queryKey)}`
}

/**
 * Builds the auth client with its own key hash.
 * @returns The query client.
 */
function clientWithOwnKeyHash(): QueryClient {
	return createAuthQueryClient({ queries: { queryKeyHashFn: prefixedKeyHash, retry: false } })
}

test('files the session under the custom key hash', () => {
	const client = clientWithOwnKeyHash()

	seedSession(client)

	expect(client.getQueryCache().find({ queryKey: sessionQueryKey })?.queryHash).toBe(
		prefixedKeyHash(sessionQueryKey),
	)
})

test('drops every other cached read when a different account answers on a client with its own key hash', () => {
	const client = clientWithOwnKeyHash()
	seedSession(client)
	client.setQueryData(listKey, lastAccountRows)

	seedSession(client, otherAccount)

	expect(client.getQueryData(listKey)).toBeUndefined()
	expect(client.getQueryData(sessionQueryKey)).toEqual(otherAccount)
})

test('drops a read whose key only starts with the session key when a different account answers', () => {
	const client = clientWithOwnKeyHash()
	seedSession(client)
	client.setQueryData(keyUnderSessionKey, lastAccountRows)

	seedSession(client, otherAccount)

	expect(client.getQueryData(keyUnderSessionKey)).toBeUndefined()
	expect(client.getQueryData(sessionQueryKey)).toEqual(otherAccount)
})

test('adoptSession keeps the session on a client with its own key hash', async () => {
	const client = clientWithOwnKeyHash()
	client.setQueryData(listKey, lastAccountRows)

	await adoptSession(client, defaultUser)

	expect(client.getQueryData(sessionQueryKey)).toEqual(defaultUser)
	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('useLogout keeps the signed-out session on a client with its own key hash', async () => {
	server.use(logoutOk())
	const client = clientWithOwnKeyHash()
	seedSession(client)
	client.setQueryData(listKey, lastAccountRows)
	const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children)
	const { result } = renderHook(() => useLogout(), { wrapper })

	await act(() => result.current.mutateAsync())

	expect(client.getQueryData(sessionQueryKey)).toBeNull()
	expect(client.getQueryData(listKey)).toBeUndefined()
})
