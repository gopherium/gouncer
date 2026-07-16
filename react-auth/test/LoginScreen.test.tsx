// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import {
	defaultUser,
	loginFailure,
	loginInvalid,
	loginOk,
	loginRateLimited,
	server,
} from '../src/testing'
import { LoginScreen } from '../src/wpds'

function renderLogin() {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	const onLogin = vi.fn()
	render(
		<QueryClientProvider client={client}>
			<LoginScreen brand="Testbed" onLogin={onLogin} />
		</QueryClientProvider>,
	)
	return onLogin
}

async function submitCredentials(email: string, password: string) {
	await userEvent.type(screen.getByLabelText('Email'), email)
	await userEvent.type(screen.getByLabelText('Password'), password)
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))
}

test('shows the login form under the given brand', () => {
	renderLogin()

	expect(screen.getByRole('heading', { name: 'Testbed' })).toBeInTheDocument()
	expect(screen.getByLabelText('Email')).toBeInTheDocument()
	expect(screen.getByLabelText('Password')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Log in' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('reports the user after a successful login', async () => {
	server.use(loginOk())
	const onLogin = renderLogin()

	await submitCredentials('grace@example.com', 'correct horse battery')

	expect(onLogin).toHaveBeenCalledWith(defaultUser)
})

test('shows an error on rejected credentials', async () => {
	server.use(loginInvalid())
	const onLogin = renderLogin()

	await submitCredentials('grace@example.com', 'wrong password!')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Invalid email or password.',
	)
	expect(onLogin).not.toHaveBeenCalled()
})

test('shows a rate-limit message when too many attempts are made', async () => {
	server.use(loginRateLimited())
	const onLogin = renderLogin()

	await submitCredentials('grace@example.com', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Too many attempts. Please wait a minute and try again.',
	)
	expect(onLogin).not.toHaveBeenCalled()
})

test('shows a generic error when the backend fails', async () => {
	server.use(loginFailure())
	const onLogin = renderLogin()

	await submitCredentials('grace@example.com', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Login failed, please try again.',
	)
	expect(onLogin).not.toHaveBeenCalled()
})
