// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider, dehydrate, hydrate, useMutation } from '@tanstack/react-query'
import type { DehydratedState } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { UnauthorizedError } from '../src/api'
import type { User } from '../src/api'
import { createAuthQueryClient } from '../src/queryClient'
import { sessionQueryKey } from '../src/session'
import { defaultUser, seedSession, userWithRole } from '../src/testing'

/** otherAccount is an account whose id differs from the default one. */
const otherAccount: User = userWithRole('editor')

/** sameAccountChanged is the default account with a new name and role. */
const sameAccountChanged: User = { ...defaultUser, name: 'Renamed Account', role: 'editor' }

/** sessionWithoutId is the default account with no id. */
const sessionWithoutId = { ...defaultUser, id: undefined }

/** listKey is a read keyed without the account. */
const listKey = ['things'] as const

/** lastAccountRows is what the list read answered for the last account. */
const lastAccountRows = ['row of the last account']

/** otherAccountRows is what the list read answered for the other account. */
const otherAccountRows = ['row of the other account']

/**
 * Builds an answer the test settles by hand.
 * @returns The pending answer with its resolve and reject.
 */
function heldAnswer() {
	let resolve: (rows: string[]) => void = () => {}
	let reject: (error: Error) => void = () => {}
	const promise = new Promise<string[]>((settle, fail) => {
		resolve = settle
		reject = fail
	})
	return { promise, resolve, reject }
}

/**
 * Saves a cache the test fills.
 * @param fill - Writes the reads to save.
 * @returns The saved cache.
 */
function savedCache(fill: (source: QueryClient) => void): DehydratedState {
	const source = new QueryClient()
	fill(source)
	return dehydrate(source)
}

/**
 * Saves the list read of the last account and then its session.
 * @param source - The client the saved cache comes from.
 */
function fillLastAccount(source: QueryClient) {
	source.setQueryData(listKey, lastAccountRows)
	seedSession(source)
}

test('drops the cached session when a query reports it gone', async () => {
	const client = createAuthQueryClient({ queries: { retry: false } })
	seedSession(client)

	await expect(
		client.fetchQuery({
			queryKey: ['things'],
			queryFn: () => Promise.reject(new UnauthorizedError('session expired')),
		}),
	).rejects.toBeInstanceOf(UnauthorizedError)

	expect(client.getQueryData(sessionQueryKey)).toBeNull()
})

test('keeps the cached session on unrelated query failures', async () => {
	const client = createAuthQueryClient({ queries: { retry: false } })
	seedSession(client)

	await expect(
		client.fetchQuery({
			queryKey: ['things'],
			queryFn: () => Promise.reject(new Error('backend down')),
		}),
	).rejects.toThrow('backend down')

	expect(client.getQueryData(sessionQueryKey)).toEqual(defaultUser)
})

test('drops the cached session when a mutation reports it gone', async () => {
	const client = createAuthQueryClient({ mutations: { retry: false } })
	seedSession(client)
	const wrapper = ({ children }: { children: ReactNode }) => (
		<QueryClientProvider client={client}>{children}</QueryClientProvider>
	)
	const { result } = renderHook(
		() =>
			useMutation({
				mutationFn: () => Promise.reject(new UnauthorizedError('session expired')),
			}),
		{ wrapper },
	)

	result.current.mutate()

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

test('drops every other cached read when the session answers a different account', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	client.setQueryData(listKey, lastAccountRows)

	seedSession(client, otherAccount)

	expect(client.getQueryData(listKey)).toBeUndefined()
	expect(client.getQueryData(sessionQueryKey)).toEqual(otherAccount)
})

test('drops every other cached read when a different account answers after the session went null', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	client.setQueryData(listKey, lastAccountRows)
	seedSession(client, null)
	expect(client.getQueryData(listKey)).toEqual(lastAccountRows)

	seedSession(client, otherAccount)

	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('keeps the cached reads when the same account answers after the session went null', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	client.setQueryData(listKey, lastAccountRows)
	seedSession(client, null)

	seedSession(client)

	expect(client.getQueryData(listKey)).toEqual(lastAccountRows)
})

test('keeps the cached reads when the first account answers', () => {
	const client = createAuthQueryClient()
	client.setQueryData(listKey, lastAccountRows)

	seedSession(client)

	expect(client.getQueryData(listKey)).toEqual(lastAccountRows)
})

test('keeps the cached reads when the same account answers with changed fields', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	client.setQueryData(listKey, lastAccountRows)

	seedSession(client, sameAccountChanged)

	expect(client.getQueryData(listKey)).toEqual(lastAccountRows)
})

test('drops every other cached read when the session answers the first account again after a different one', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	seedSession(client, otherAccount)
	client.setQueryData(listKey, otherAccountRows)

	seedSession(client)

	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('drops the answer of a read the last account started once a different account answers', async () => {
	const client = createAuthQueryClient({ queries: { retry: false } })
	seedSession(client)
	const held = heldAnswer()
	const started = client.fetchQuery({ queryKey: listKey, queryFn: () => held.promise }).catch(() => undefined)
	const lastAccountRead = client.getQueryCache().find({ queryKey: listKey })

	seedSession(client, otherAccount)
	held.resolve(lastAccountRows)
	await started

	expect(lastAccountRead?.state.data).toBeUndefined()
	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('keeps the next account signed in when a read the last account started then fails as unauthorized', async () => {
	const client = createAuthQueryClient({ queries: { retry: false } })
	seedSession(client)
	const held = heldAnswer()
	const started = client.fetchQuery({ queryKey: listKey, queryFn: () => held.promise }).catch(() => undefined)

	seedSession(client, otherAccount)
	held.reject(new UnauthorizedError('session expired'))
	await started

	expect(client.getQueryData(sessionQueryKey)).toEqual(otherAccount)
})

test('keeps the restored reads when the session is restored together with them', () => {
	const client = createAuthQueryClient()

	hydrate(client, savedCache(fillLastAccount))

	expect(client.getQueryData(sessionQueryKey)).toEqual(defaultUser)
	expect(client.getQueryData(listKey)).toEqual(lastAccountRows)
})

test('drops every other cached read when a different account answers after the session was restored', () => {
	const client = createAuthQueryClient()
	hydrate(client, savedCache(fillLastAccount))

	seedSession(client, otherAccount)

	expect(client.getQueryData(listKey)).toBeUndefined()
	expect(client.getQueryData(sessionQueryKey)).toEqual(otherAccount)
})

test('drops every other cached read when a newer restored session answers a different account', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	client.setQueryData(listKey, lastAccountRows)
	const sessionEntry = client.getQueryCache().find({ queryKey: sessionQueryKey })
	const newerUpdate = (sessionEntry?.state.dataUpdatedAt ?? 0) + 1
	const updatedActions: string[] = []
	client.getQueryCache().subscribe((event) => {
		if (event.type === 'updated') {
			updatedActions.push(event.action.type)
		}
	})

	hydrate(client, savedCache((source) => source.setQueryData(sessionQueryKey, otherAccount, { updatedAt: newerUpdate })))

	expect(updatedActions).toEqual(['setState'])
	expect(client.getQueryCache().find({ queryKey: sessionQueryKey })).toBe(sessionEntry)
	expect(sessionEntry?.state.data).toEqual(otherAccount)
	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('drops every other cached read when a different account follows a session given as initial data', async () => {
	const client = createAuthQueryClient()
	await client.ensureQueryData({
		queryKey: sessionQueryKey,
		queryFn: () => Promise.resolve(defaultUser),
		initialData: defaultUser,
	})
	client.setQueryData(listKey, lastAccountRows)

	seedSession(client, otherAccount)

	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('keeps the cached reads when a session entry the cache replaced is removed later', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	const [replacedEntry] = client.getQueryCache().getAll()
	client.removeQueries({ queryKey: sessionQueryKey })
	seedSession(client, otherAccount)
	client.setQueryData(listKey, otherAccountRows)

	client.getQueryCache().remove(replacedEntry)

	expect(replacedEntry.state.data).toEqual(defaultUser)
	expect(client.getQueryData(listKey)).toEqual(otherAccountRows)
})

test('keeps the cached reads when a session entry the cache no longer holds reports a change', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	const [replacedEntry] = client.getQueryCache().getAll()
	client.removeQueries({ queryKey: sessionQueryKey })
	seedSession(client, otherAccount)
	client.setQueryData(listKey, otherAccountRows)

	replacedEntry.invalidate()

	expect(client.getQueryData(listKey)).toEqual(otherAccountRows)
	seedSession(client)
	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('drops every other cached read when a different account follows a restored session with no id', () => {
	const client = createAuthQueryClient()
	seedSession(client)
	const newerUpdate = (client.getQueryState(sessionQueryKey)?.dataUpdatedAt ?? 0) + 1
	hydrate(
		client,
		savedCache((source) => source.setQueryData(sessionQueryKey, sessionWithoutId, { updatedAt: newerUpdate })),
	)
	client.setQueryData(listKey, lastAccountRows)

	seedSession(client, otherAccount)

	expect(client.getQueryData(listKey)).toBeUndefined()
	expect(client.getQueryData(sessionQueryKey)).toEqual(otherAccount)
})
