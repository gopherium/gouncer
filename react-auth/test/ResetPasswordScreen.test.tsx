// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { HttpResponse, http, resetInvalidToken, resetOk, server } from '../src/testing'
import { ResetPasswordScreen } from '../src/wpds'

function renderResetPassword(onDone?: () => void) {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	render(
		<QueryClientProvider client={client}>
			<ResetPasswordScreen brand="Testbed" token="t-123" onDone={onDone} />
		</QueryClientProvider>,
	)
}

async function submitPassword(password: string) {
	await userEvent.type(await screen.findByLabelText('Password'), password)
	await userEvent.click(screen.getByRole('button', { name: 'Set password' }))
}

test('shows the form with a disabled submit until a password is typed', async () => {
	renderResetPassword()

	expect(await screen.findByRole('heading', { name: 'Testbed' })).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Set password' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('hints browsers this is a fresh password', async () => {
	renderResetPassword()

	expect(await screen.findByLabelText('Password')).toHaveAttribute(
		'autocomplete',
		'new-password',
	)
})

test('posts the token and password and replaces the form with the outcome', async () => {
	let body: unknown
	server.use(
		http.post('/api/auth/password-reset/confirm', async ({ request }) => {
			body = await request.json()
			return new HttpResponse(null, { status: 204 })
		}),
	)
	renderResetPassword()

	await submitPassword('another good password')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Your password is set. Sign in with it.')
	expect(status).toHaveFocus()
	expect(body).toEqual({ token: 't-123', password: 'another good password' })
	expect(screen.queryByRole('button', { name: 'Set password' })).not.toBeInTheDocument()
})

test('offers the way to login after success when one is provided', async () => {
	server.use(resetOk())
	const onDone = vi.fn()
	renderResetPassword(onDone)

	await submitPassword('another good password')
	await userEvent.click(await screen.findByRole('button', { name: 'Go to login' }))

	await waitFor(() => expect(onDone).toHaveBeenCalled())
})

test('renders no way to login when none is provided', async () => {
	server.use(resetOk())
	renderResetPassword()

	await submitPassword('another good password')

	expect(await screen.findByRole('status')).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Go to login' })).not.toBeInTheDocument()
})

test('tells the user when the link is dead', async () => {
	server.use(resetInvalidToken())
	renderResetPassword()

	await submitPassword('another good password')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'This link is no longer valid. Ask for a new one.',
	)
})

test('shows the server validation message for a weak password', async () => {
	server.use(
		http.post('/api/auth/password-reset/confirm', () =>
			HttpResponse.json({ error: 'password must be at least 12 characters' }, { status: 422 }),
		),
	)
	renderResetPassword()

	await submitPassword('short-but-typed')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'password must be at least 12 characters',
	)
})

test('shows a generic message when the reset fails', async () => {
	server.use(
		http.post('/api/auth/password-reset/confirm', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderResetPassword()

	await submitPassword('another good password')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The password could not be set, please try again.',
	)
})
