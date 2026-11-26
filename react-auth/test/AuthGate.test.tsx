// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode, useState } from 'react'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { AuthGate } from '../src/AuthGate'
import type { User } from '../src/api'
import { createAuthQueryClient } from '../src/queryClient'
import { sessionQueryKey, useLogout, useSession } from '../src/session'
import {
	HttpResponse,
	defaultUser,
	http,
	loginOk,
	logoutOk,
	sessionAnonymous,
	sessionFailure,
	sessionOk,
	server,
	userWithRole,
} from '../src/testing'
import { LoginScreen } from '../src/wpds'

/** otherAccount is an account whose id differs from the default one. */
const otherAccount: User = userWithRole('editor')

/** sameAccountChanged is the default account with a new name and role. */
const sameAccountChanged: User = { ...defaultUser, name: 'Renamed Account', role: 'editor' }

/** listKey is a read keyed without the account. */
const listKey = ['things'] as const

/** lastAccountRow is the row the list read answered for the last account. */
const lastAccountRow = 'row of the last account'

/** otherAccountRow is the row the list read answers for the other account. */
const otherAccountRow = 'row of the other account'

/** lastAccountRows is what the list read answered for the last account. */
const lastAccountRows = [lastAccountRow]

/** otherAccountRows is what the list read answers for the other account. */
const otherAccountRows = [otherAccountRow]

/** servedRows is what the static list read answers. */
let servedRows: string[] = []

/** servedAnswer is what the loading list read answers. */
let servedAnswer: Promise<string[]> = Promise.resolve([])

function renderGate(children: ReactNode = <p>Protected area</p>) {
	const client = new QueryClient({
		defaultOptions: {
			queries: { retry: false },
			mutations: { retry: false },
		},
	})
	render(
		<QueryClientProvider client={client}>
			<AuthGate
				loginScreen={(onLogin) => <LoginScreen brand="Testbed" onLogin={onLogin} />}
			>
				{children}
			</AuthGate>
		</QueryClientProvider>,
	)
	return client
}

function SignOutProbe() {
	const signOut = useLogout()
	return (
		<button type="button" onClick={() => signOut.mutate()}>
			Sign out
		</button>
	)
}

/**
 * Renders AuthGate under the strict checks, on the client createAuthQueryClient builds.
 * @param children - The protected area.
 */
function renderAuthClientGate(children: ReactNode) {
	const client = createAuthQueryClient({ queries: { retry: false }, mutations: { retry: false } })
	render(
		<StrictMode>
			<QueryClientProvider client={client}>
				<AuthGate loginScreen={() => <p>Login screen</p>}>{children}</AuthGate>
			</QueryClientProvider>
		</StrictMode>,
	)
}

/** Fires the visibility change react-query reads as the window getting focus again. */
async function fireVisibilityChange() {
	await act(async () => {
		window.dispatchEvent(new Event('visibilitychange'))
	})
}

/** Shows the signed-in name and holds a count of its own. */
function CountedChild() {
	const session = useSession()
	const [count, setCount] = useState(0)
	return (
		<>
			<p>{`Signed in as ${session.data?.name}`}</p>
			<button type="button" onClick={() => setCount((held) => held + 1)}>{`Count ${count}`}</button>
		</>
	)
}

/** Shows the rows of a list read that never goes stale. */
function StaticListReader() {
	const list = useQuery({ queryKey: listKey, queryFn: () => Promise.resolve(servedRows), staleTime: 'static' })
	return <p>{list.data?.join(', ')}</p>
}

/** Shows the rows of a list read, or a loading line until it answers. */
function LoadingListReader() {
	const list = useQuery({ queryKey: listKey, queryFn: () => servedAnswer })
	return <p>{list.data?.join(', ') ?? 'Loading list'}</p>
}

test('shows a loading indicator while the session resolves', () => {
	server.use(sessionOk())
	renderGate()

	expect(screen.getByRole('status')).toBeInTheDocument()
})

test('renders the children when a session is active', async () => {
	server.use(sessionOk())
	renderGate()

	expect(await screen.findByText('Protected area')).toBeInTheDocument()
	expect(screen.queryByLabelText('Email')).not.toBeInTheDocument()
})

test('shows the login screen when there is no session', async () => {
	server.use(sessionAnonymous())
	renderGate()

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
	expect(screen.getByText('Testbed')).toBeInTheDocument()
	expect(screen.queryByText('Protected area')).not.toBeInTheDocument()
})

test('shows an error when the session cannot be loaded', async () => {
	server.use(sessionFailure())
	renderGate()

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Something went wrong.',
	)
})

test('keeps the app mounted when a background session refetch fails', async () => {
	server.use(sessionOk())
	const client = renderGate()
	expect(await screen.findByText('Protected area')).toBeInTheDocument()

	server.use(sessionFailure())
	await act(() => client.invalidateQueries())
	await act(() => new Promise((resolve) => setTimeout(resolve, 50)))

	expect(screen.getByText('Protected area')).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('reveals the children after a successful login', async () => {
	server.use(sessionAnonymous(), loginOk(defaultUser))
	renderGate()

	await userEvent.type(await screen.findByLabelText('Email'), 'grace@example.com')
	await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))

	expect(await screen.findByText('Protected area')).toBeInTheDocument()
})

test('returns to the login screen after logging out', async () => {
	server.use(sessionOk(), logoutOk())
	renderGate(<SignOutProbe />)

	await userEvent.click(await screen.findByRole('button', { name: 'Sign out' }))

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
})

test('ignores a stale session response that resolves after login', async () => {
	server.use(sessionAnonymous(), loginOk(defaultUser))
	const client = renderGate()
	await screen.findByLabelText('Email')

	let releaseStaleSession = () => {}
	const staleSession = new Promise<void>((resolve) => {
		releaseStaleSession = resolve
	})
	server.use(
		http.get('/api/auth/session', async () => {
			await staleSession
			return HttpResponse.json({ error: 'no session' }, { status: 401 })
		}),
	)
	void client.invalidateQueries()

	await userEvent.type(screen.getByLabelText('Email'), 'grace@example.com')
	await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))
	expect(await screen.findByText('Protected area')).toBeInTheDocument()

	releaseStaleSession()
	await act(() => new Promise((resolve) => setTimeout(resolve, 50)))

	expect(screen.getByText('Protected area')).toBeInTheDocument()
})

test('mounts the children again when the session read answers a different account', async () => {
	server.use(sessionOk())
	const client = renderGate(
		<StrictMode>
			<CountedChild />
		</StrictMode>,
	)
	await userEvent.click(await screen.findByRole('button', { name: 'Count 0' }))
	expect(screen.getByRole('button')).toHaveTextContent('Count 1')

	server.use(sessionOk(otherAccount))
	await act(() => client.invalidateQueries({ queryKey: sessionQueryKey }))

	expect(await screen.findByText(`Signed in as ${otherAccount.name}`)).toBeInTheDocument()
	expect(screen.getByRole('button')).toHaveTextContent('Count 0')
})

test('keeps the children mounted when the same account answers with changed fields', async () => {
	server.use(sessionOk())
	const client = renderGate(
		<StrictMode>
			<CountedChild />
		</StrictMode>,
	)
	await userEvent.click(await screen.findByRole('button', { name: 'Count 0' }))
	expect(screen.getByRole('button')).toHaveTextContent('Count 1')

	server.use(sessionOk(sameAccountChanged))
	await act(() => client.invalidateQueries({ queryKey: sessionQueryKey }))

	expect(await screen.findByText('Signed in as Renamed Account')).toBeInTheDocument()
	expect(screen.getByRole('button')).toHaveTextContent('Count 1')
})

test('shows only the next account answer on a mounted screen whose read never goes stale', async () => {
	servedRows = lastAccountRows
	server.use(sessionOk())
	renderAuthClientGate(<StaticListReader />)
	expect(await screen.findByText(lastAccountRow)).toBeInTheDocument()

	servedRows = otherAccountRows
	server.use(sessionOk(otherAccount))
	await fireVisibilityChange()

	expect(await screen.findByText(otherAccountRow)).toBeInTheDocument()
	expect(screen.queryByText(lastAccountRow)).not.toBeInTheDocument()
})

test('shows only the next account answer on a mounted screen whose read is still loading at the switch', async () => {
	let releaseLastAccountRows = () => {}
	servedAnswer = new Promise((resolve) => {
		releaseLastAccountRows = () => resolve(lastAccountRows)
	})
	server.use(sessionOk())
	renderAuthClientGate(<LoadingListReader />)
	expect(await screen.findByText('Loading list')).toBeInTheDocument()

	servedAnswer = Promise.resolve(otherAccountRows)
	server.use(sessionOk(otherAccount))
	await fireVisibilityChange()
	expect(await screen.findByText(otherAccountRow)).toBeInTheDocument()

	releaseLastAccountRows()
	await act(() => new Promise((resolve) => setTimeout(resolve, 50)))

	expect(screen.getByText(otherAccountRow)).toBeInTheDocument()
	expect(screen.queryByText(lastAccountRow)).not.toBeInTheDocument()
})
