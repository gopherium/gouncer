// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import {
	HttpResponse,
	activateInvalidToken,
	activateOk,
	defaultUser,
	http,
	server,
} from '../src/testing'
import { SetPasswordScreen } from '../src/wpds'

function renderSetPassword() {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	const onAccepted = vi.fn()
	render(
		<QueryClientProvider client={client}>
			<SetPasswordScreen brand="Testbed" token="t-123" onAccepted={onAccepted} />
		</QueryClientProvider>,
	)
	return onAccepted
}

async function submitPassword(password: string) {
	await userEvent.type(await screen.findByLabelText('Password'), password)
	await userEvent.click(screen.getByRole('button', { name: 'Set password' }))
}

test('shows the form with a disabled submit until a password is typed', async () => {
	renderSetPassword()

	expect(await screen.findByRole('heading', { name: 'Testbed' })).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Set password' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('hints browsers this is a fresh password', async () => {
	renderSetPassword()

	expect(await screen.findByLabelText('Password')).toHaveAttribute(
		'autocomplete',
		'new-password',
	)
})

test('posts the token and password and hands the user upward', async () => {
	let body: unknown
	server.use(
		http.post('/api/auth/activate', async ({ request }) => {
			body = await request.json()
			return HttpResponse.json(defaultUser)
		}),
	)
	const onAccepted = renderSetPassword()

	await submitPassword('correct horse battery')

	await waitFor(() => expect(onAccepted).toHaveBeenCalledWith(defaultUser))
	expect(body).toEqual({ token: 't-123', password: 'correct horse battery' })
})

test('tells the user when the link is dead', async () => {
	server.use(activateInvalidToken())
	const onAccepted = renderSetPassword()

	await submitPassword('correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'This link is no longer valid. Ask the person who invited you for a new one.',
	)
	expect(onAccepted).not.toHaveBeenCalled()
})

test('shows the server validation message for a weak password', async () => {
	server.use(
		http.post('/api/auth/activate', () =>
			HttpResponse.json({ error: 'password must be at least 12 characters' }, { status: 422 }),
		),
	)
	renderSetPassword()

	await submitPassword('short-but-typed')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'password must be at least 12 characters',
	)
})

test('tells a throttled user to wait instead of retrying', async () => {
	server.use(
		http.post('/api/auth/activate', () =>
			HttpResponse.json({ error: 'slow down' }, { status: 429 }),
		),
	)
	const onAccepted = renderSetPassword()

	await submitPassword('correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Too many attempts. Please wait a minute and try again.',
	)
	expect(onAccepted).not.toHaveBeenCalled()
})

test('shows a generic message when activation fails', async () => {
	server.use(
		http.post('/api/auth/activate', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderSetPassword()

	await submitPassword('correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The password could not be set, please try again.',
	)
})

test('accepts through the canned handler', async () => {
	server.use(activateOk())
	const onAccepted = renderSetPassword()

	await submitPassword('correct horse battery')

	await waitFor(() => expect(onAccepted).toHaveBeenCalledWith(defaultUser))
})
