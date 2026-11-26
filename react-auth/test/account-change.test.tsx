// SPDX-License-Identifier: Apache-2.0

import { QueryClientProvider } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { AuthGate } from '../src/AuthGate'
import { UnauthorizedError } from '../src/api'
import type { User } from '../src/api'
import { createAuthQueryClient } from '../src/queryClient'
import { sessionQueryKey } from '../src/session'
import { defaultUser, server, sessionOk, userWithRole } from '../src/testing'

/** otherAccount is an account whose id differs from the default one. */
const otherAccount: User = userWithRole('editor')

/** listKey is a read keyed without the account. */
const listKey = ['things'] as const

/** lastAccountRows is what the list read answered for the last account. */
const lastAccountRows = ['row of the last account']

/**
 * Renders AuthGate under the strict checks, on the client createAuthQueryClient builds.
 * @param children - The protected area.
 * @returns The query client under test.
 */
function renderGate(children: ReactNode = <p>Protected area</p>) {
	const client = createAuthQueryClient({ queries: { retry: false }, mutations: { retry: false } })
	render(
		<StrictMode>
			<QueryClientProvider client={client}>
				<AuthGate loginScreen={() => <p>Login screen</p>}>{children}</AuthGate>
			</QueryClientProvider>
		</StrictMode>,
	)
	return client
}

/** Fires the visibility change react-query reads as the window getting focus again. */
async function fireVisibilityChange() {
	await act(async () => {
		window.dispatchEvent(new Event('visibilitychange'))
	})
}

/**
 * Expires the session with an unauthorized read.
 * @param client - The query client under test.
 */
async function expireSession(client: QueryClient) {
	await act(async () => {
		await client
			.fetchQuery({
				queryKey: ['expiring-read'],
				queryFn: () => Promise.reject(new UnauthorizedError('session expired')),
			})
			.catch(() => undefined)
	})
}

test('drops the last account reads when a focus read of the session answers a different account', async () => {
	server.use(sessionOk())
	const client = renderGate()
	expect(await screen.findByText('Protected area')).toBeInTheDocument()
	client.setQueryData(listKey, lastAccountRows)

	server.use(sessionOk(otherAccount))
	await fireVisibilityChange()
	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toEqual(otherAccount))

	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('drops the last account reads when the session answers a different account after it expired', async () => {
	server.use(sessionOk())
	const client = renderGate()
	expect(await screen.findByText('Protected area')).toBeInTheDocument()
	client.setQueryData(listKey, lastAccountRows)
	await expireSession(client)
	expect(await screen.findByText('Login screen')).toBeInTheDocument()

	server.use(sessionOk(otherAccount))
	await fireVisibilityChange()

	expect(await screen.findByText('Protected area')).toBeInTheDocument()
	expect(client.getQueryData(sessionQueryKey)).toEqual(otherAccount)
	expect(client.getQueryData(listKey)).toBeUndefined()
})

test('keeps the account reads when the same account answers after its session expired', async () => {
	server.use(sessionOk())
	const client = renderGate()
	expect(await screen.findByText('Protected area')).toBeInTheDocument()
	client.setQueryData(listKey, lastAccountRows)
	await expireSession(client)
	expect(await screen.findByText('Login screen')).toBeInTheDocument()

	await fireVisibilityChange()

	expect(await screen.findByText('Protected area')).toBeInTheDocument()
	expect(client.getQueryData(sessionQueryKey)).toEqual(defaultUser)
	expect(client.getQueryData(listKey)).toEqual(lastAccountRows)
})
