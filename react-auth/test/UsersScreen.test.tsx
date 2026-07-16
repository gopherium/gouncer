// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import { HttpResponse, http, seedSession, server } from '../src/testing'
import { UsersScreen } from '../src/wpds'

const ada = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'ada@example.com',
	name: 'Ada Lovelace',
	disabled: false,
	created_at: '2026-07-06T10:00:00Z',
}
const grace = {
	id: '0198b2f0-0000-7000-8000-000000000002',
	email: 'grace@example.com',
	name: 'Grace Hopper',
	disabled: false,
	created_at: '2026-07-06T11:00:00Z',
}

function renderUsers() {
	const client = new QueryClient({
		defaultOptions: {
			queries: { retry: false, staleTime: Infinity },
			mutations: { retry: false },
		},
	})
	seedSession(client, ada)
	render(
		<QueryClientProvider client={client}>
			<UsersScreen newUserRender={<a href="/users/new" />} />
		</QueryClientProvider>,
	)
}

function rowFor(name: string) {
	return screen.getByRole('row', { name: new RegExp(name) })
}

test('shows a loading state, then lists the users', async () => {
	server.use(
		http.get('/api/users', async () => {
			await new Promise((resolve) => setTimeout(resolve, 20))
			return HttpResponse.json([ada, grace])
		}),
	)
	renderUsers()

	expect(await screen.findByRole('status')).toHaveTextContent('Loading users…')

	expect(await screen.findByRole('row', { name: /Grace Hopper/ })).toBeInTheDocument()
	expect(within(rowFor('Grace Hopper')).getByText('grace@example.com')).toBeInTheDocument()
})

test('shows an error when the users cannot be loaded', async () => {
	server.use(
		http.get('/api/users', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderUsers()

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Users could not be loaded.',
	)
})

test('does not offer to disable the signed-in account', async () => {
	server.use(http.get('/api/users', () => HttpResponse.json([ada, grace])))
	renderUsers()

	const ownRow = await screen.findByRole('row', { name: /Ada Lovelace/ })
	expect(within(ownRow).queryByRole('button')).toBeNull()
})

test('disables another user and reflects the new status', async () => {
	let disabled = false
	let patchedId = ''
	server.use(
		http.get('/api/users', () =>
			HttpResponse.json([ada, { ...grace, disabled }]),
		),
		http.patch('/api/users/:id', async ({ request, params }) => {
			patchedId = params.id as string
			disabled = ((await request.json()) as { disabled: boolean }).disabled
			return new HttpResponse(null, { status: 204 })
		}),
	)
	renderUsers()

	await userEvent.click(
		within(await screen.findByRole('row', { name: /Grace Hopper/ })).getByRole('button', {
			name: 'Disable Grace Hopper',
		}),
	)

	await waitFor(() =>
		expect(
			within(rowFor('Grace Hopper')).getByRole('button', { name: 'Enable Grace Hopper' }),
		).toBeInTheDocument(),
	)
	expect(within(rowFor('Grace Hopper')).getByText('Disabled')).toBeInTheDocument()
	expect(patchedId).toBe(grace.id)
})

test('surfaces an error when a toggle fails', async () => {
	server.use(
		http.get('/api/users', () => HttpResponse.json([ada, grace])),
		http.patch('/api/users/:id', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderUsers()

	await userEvent.click(
		within(await screen.findByRole('row', { name: /Grace Hopper/ })).getByRole('button', {
			name: 'Disable Grace Hopper',
		}),
	)

	expect(await screen.findByRole('alert')).toHaveTextContent('Update failed.')
})

test('links to the new user form as a single control', async () => {
	server.use(http.get('/api/users', () => HttpResponse.json([ada])))
	renderUsers()

	await screen.findByRole('row', { name: /Ada Lovelace/ })
	const link = screen.getByRole('link', { name: 'New user' })
	expect(link).toHaveAttribute('href', '/users/new')
	expect(within(link).queryByRole('button')).toBeNull()
})

test('omits the new user control without a navigation target', async () => {
	server.use(http.get('/api/users', () => HttpResponse.json([ada])))
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false, staleTime: Infinity } },
	})
	seedSession(client, ada)
	render(
		<QueryClientProvider client={client}>
			<UsersScreen />
		</QueryClientProvider>,
	)

	await screen.findByRole('row', { name: /Ada Lovelace/ })
	expect(screen.queryByRole('link', { name: 'New user' })).toBeNull()
})
