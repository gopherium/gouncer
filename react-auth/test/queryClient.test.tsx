// SPDX-License-Identifier: Apache-2.0

import { QueryClientProvider, useMutation } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { UnauthorizedError } from '../src/api'
import { createAuthQueryClient } from '../src/queryClient'
import { sessionQueryKey } from '../src/session'
import { defaultUser, seedSession } from '../src/testing'

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
