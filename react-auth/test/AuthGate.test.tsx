// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { AuthGate } from '../src/AuthGate'
import { useLogout } from '../src/session'
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
} from '../src/testing'
import { LoginScreen } from '../src/wpds'

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
