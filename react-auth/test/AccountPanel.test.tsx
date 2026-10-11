// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http } from 'msw'
import { expect, test } from 'vitest'

import { sessionQueryKey } from '../src/session'
import { logoutFailure, logoutOk, seedSession, server } from '../src/testing'
import { AccountPanel } from '../src/wpds'
import { busyClasses } from './busy'

function renderPanel(signedIn = true) {
	const client = new QueryClient({
		defaultOptions: {
			queries: { retry: false, staleTime: Infinity },
			mutations: { retry: false },
		},
	})
	seedSession(client, signedIn ? undefined : null)
	render(
		<QueryClientProvider client={client}>
			<AccountPanel className="testbed-account" />
		</QueryClientProvider>,
	)
	return client
}

test('shows the log out button busy while the session is ending', async () => {
	const busy = busyClasses()
	expect(busy.length).toBeGreaterThan(0)
	server.use(http.post('/api/auth/logout', () => new Promise(() => {})))
	renderPanel()

	await userEvent.click(await screen.findByRole('button', { name: 'Log out' }))

	const out = screen.getByRole('button', { name: 'Log out' })
	await waitFor(() => {
		for (const token of busy) {
			expect([...out.classList]).toContain(token)
		}
	})
	expect(out).toHaveAttribute('aria-disabled', 'true')
})

test('shows the signed-in user and a logout control', async () => {
	renderPanel()

	expect(await screen.findByText('Grace Hopper')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Log out' })).toBeInTheDocument()
})

test('clears the session when logging out', async () => {
	server.use(logoutOk())
	const client = renderPanel()

	await userEvent.click(await screen.findByRole('button', { name: 'Log out' }))

	await waitFor(() =>
		expect(client.getQueryData(sessionQueryKey)).toBeNull(),
	)
})

test('drops all cached data when logging out', async () => {
	server.use(logoutOk())
	const client = renderPanel()
	client.setQueryData(['things'], [{ id: '1', name: 'Ada Lovelace' }])

	await userEvent.click(await screen.findByRole('button', { name: 'Log out' }))

	await waitFor(() =>
		expect(client.getQueryData(['things'])).toBeUndefined(),
	)
	expect(client.getQueryData(sessionQueryKey)).toBeNull()
})

test('shows an error when logging out fails', async () => {
	server.use(logoutFailure())
	renderPanel()

	await userEvent.click(await screen.findByRole('button', { name: 'Log out' }))

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Logout failed, please try again.',
	)
	expect(screen.getByRole('button', { name: 'Log out' })).toBeInTheDocument()
})

test('renders nothing when no user is present', async () => {
	renderPanel(false)

	await waitFor(() =>
		expect(screen.queryByRole('button', { name: 'Log out' })).toBeNull(),
	)
	expect(screen.queryByText('Grace Hopper')).toBeNull()
})
