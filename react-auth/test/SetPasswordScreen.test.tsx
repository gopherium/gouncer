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

test('locks the submit while the activation is in flight', async () => {
	let posts = 0
	server.use(
		http.post('/api/auth/activate', () => {
			posts += 1
			return new Promise<never>(() => {})
		}),
	)
	renderSetPassword()

	await userEvent.type(await screen.findByLabelText('Password'), 'correct horse battery')
	const submit = screen.getByRole('button', { name: 'Set password' })
	await userEvent.click(submit)

	await waitFor(() => expect(submit).toHaveAttribute('aria-disabled', 'true'))
	await userEvent.click(submit)
	expect(posts).toBe(1)
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

function renderHandoff(onAccepted: (user: unknown) => void | Promise<void>) {
	let posts = 0
	server.use(
		http.post('/api/auth/activate', () => {
			posts += 1
			return HttpResponse.json(defaultUser)
		}),
	)
	const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
	render(
		<QueryClientProvider client={client}>
			<SetPasswordScreen brand="Testbed" token="t-123" onAccepted={onAccepted} />
		</QueryClientProvider>,
	)
	return () => posts
}

test('does not blame the activation when handing the user onward fails', async () => {
	const posts = renderHandoff(() =>
		Promise.reject(new Error('the consumer could not navigate')),
	)

	await submitPassword('correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent('Something went wrong.')
	expect(screen.queryByRole('button', { name: 'Set password' })).not.toBeInTheDocument()
	expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
	expect(posts()).toBe(1)
})

test('keeps the retry on screen while the handoff runs', async () => {
	let calls = 0
	renderHandoff(() => {
		calls += 1
		return calls === 1
			? Promise.reject(new Error('the consumer could not navigate'))
			: new Promise<void>(() => {})
	})

	await submitPassword('correct horse battery')
	await userEvent.click(await screen.findByRole('button', { name: 'Try again' }))

	expect(screen.getByRole('button', { name: 'Try again' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
	expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong.')
	expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
})

test('retries the handoff without spending the token again', async () => {
	const calls: unknown[] = []
	const posts = renderHandoff((user) => {
		calls.push(user)
		return calls.length === 1
			? Promise.reject(new Error('the consumer could not navigate'))
			: Promise.resolve()
	})

	await submitPassword('correct horse battery')
	await userEvent.click(await screen.findByRole('button', { name: 'Try again' }))

	await waitFor(() => expect(calls).toHaveLength(2))
	expect(calls[1]).toEqual(defaultUser)
	expect(posts()).toBe(1)
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
