// SPDX-License-Identifier: Apache-2.0

import { AdminRoot, Toaster } from '@gopherium/godmin'
import { listSearch, useListView } from '@gopherium/godmin/router'
import { installTestEnvironment } from '@gopherium/godmin/testing'
import { QueryClientProvider } from '@tanstack/react-query'
import {
	Outlet,
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { View } from '@wordpress/dataviews'
import { HttpResponse, http } from 'msw'
import { expect } from 'vitest'

import type { Account } from '../src/account'
import type { User } from '../src/api'
import { createAuthQueryClient } from '../src/queryClient'
import { seedSession, server, userWithCapabilities } from '../src/testing'
import { UsersPage, usersListDefaults } from '../src/wpds/users'
import type { UsersPageProps } from '../src/wpds/users'

installTestEnvironment()

export const reader: User = userWithCapabilities('admin', ['manage_users'], ['admin', 'member'])

export function accountOf(id: string, name: string, changes: Partial<Account> = {}): Account {
	return {
		id: `0198b2f0-0000-7000-8000-0000000000${id}`,
		email: `${name.split(' ')[0].toLowerCase()}@example.com`,
		name,
		disabled: false,
		confirmed: true,
		editable: true,
		role: 'member',
		created_at: new Date('2026-07-07T10:00:00Z'),
		...changes,
	}
}

export const signedIn = accountOf('01', reader.name, {
	id: reader.id,
	email: reader.email,
	role: 'admin',
	created_at: new Date('2026-07-06T10:00:00Z'),
})
export const colleague = accountOf('ff', 'Ada Lovelace')
export const barred = accountOf('fe', 'Ana Lopez', { disabled: true, created_at: new Date('2026-07-08T10:00:00Z') })
export const longName = 'Maria Perez of the accounting and payroll office team'

let held: Account[] = []

export function holding(accounts: Account[]) {
	held = accounts
	server.use(http.get('/api/users', () => HttpResponse.json(held)))
}

export function heldAccounts(): Account[] {
	return held
}

type Written<Field extends 'disabled' | 'role'> = { id: string } & Pick<Account, Field>

const writeRoutes = {
	disabled: { route: http.patch, path: '/api/users/:id' },
	role: { route: http.put, path: '/api/users/:id/role' },
}

function writesOf<Field extends 'disabled' | 'role'>(field: Field) {
	const writes: Written<Field>[] = []
	const { route, path } = writeRoutes[field]
	server.use(
		route(path, async ({ params, request }) => {
			const value = ((await request.json()) as Pick<Account, Field>)[field]
			const write = { id: String(params.id), [field]: value } as Written<Field>
			writes.push(write)
			held = held.map((user) => (user.id === write.id ? { ...user, [field]: value } : user))
			return new HttpResponse(null, { status: 204 })
		}),
	)
	return writes
}

export function writesDisabled() {
	return writesOf('disabled')
}

export function writesRole() {
	return writesOf('role')
}

export interface RenderOptions extends Partial<Omit<UsersPageProps, 'list'>> {
	at?: string
	session?: User | null
	perPage?: number | null
	columnIds?: string[]
	writeTimeout?: number
}

export function renderUsers(options: RenderOptions = {}) {
	const { at = '/users', session = reader, perPage = 20, columnIds, writeTimeout = Infinity, ...props } = options
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } })
	seedSession(client, session)
	function Mount() {
		const list = useListView<View>(usersListDefaults({ perPage: perPage ?? undefined, columns: columnIds }))
		return <UsersPage list={list} {...props} />
	}
	const root = createRootRoute({ component: Outlet })
	const users = createRoute({ getParentRoute: () => root, path: '/users', validateSearch: listSearch, component: Mount })
	const router = createRouter({
		routeTree: root.addChildren([users]),
		history: createMemoryHistory({ initialEntries: [at] }),
	})
	render(
		<QueryClientProvider client={client}>
			<AdminRoot writeTimeout={writeTimeout}>
				<Toaster>
					<RouterProvider router={router} />
				</Toaster>
			</AdminRoot>
		</QueryClientProvider>,
	)
	return { client, router }
}

export async function rowOf(text: string) {
	return within(
		await waitFor(() => {
			const row = screen.getAllByRole('row').find((candidate) => within(candidate).queryByText(text) !== null)
			expect(row).toBeDefined()
			return row as HTMLElement
		}),
	)
}

export async function shownNames(accounts: Account[] = held) {
	await screen.findByRole('table')
	return screen
		.getAllByRole('row')
		.slice(1)
		.map((row) => accounts.find((user) => row.textContent?.includes(user.email))?.name)
}

export async function act(name: string, action: string) {
	await userEvent.click((await rowOf(name)).getByRole('button', { name: 'Actions' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: action }))
}

export async function menuOf(name: string) {
	await userEvent.click((await rowOf(name)).getByRole('button', { name: 'Actions' }))
	const labels = (await screen.findAllByRole('menuitem')).map((item) => item.textContent)
	await userEvent.keyboard('{Escape}')
	await waitFor(() => expect(screen.queryByRole('menuitem')).not.toBeInTheDocument())
	return labels
}

export async function select(...names: string[]) {
	for (const name of names) {
		await userEvent.click((await rowOf(name)).getByRole('checkbox', { name }))
	}
}

export async function bulkBar() {
	return within(
		await waitFor(() => {
			const bar = document.querySelector<HTMLElement>('.dataviews-bulk-actions__actions')
			expect(bar).not.toBeNull()
			return bar as HTMLElement
		}),
	)
}

export function toast(text: string) {
	return screen.findByText(text, { selector: '.godmin-toast' })
}
