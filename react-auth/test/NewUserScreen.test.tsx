// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { HttpResponse, http, server } from '../src/testing'
import { NewUserScreen } from '../src/wpds'

const grace = {
	id: '0198b2f0-0000-7000-8000-000000000002',
	email: 'grace@example.com',
	name: 'Grace Hopper',
	disabled: false,
	created_at: '2026-07-06T11:00:00Z',
}

function renderNewUser() {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	const onCreated = vi.fn()
	render(
		<QueryClientProvider client={client}>
			<NewUserScreen onCreated={onCreated} />
		</QueryClientProvider>,
	)
	return onCreated
}

async function fillForm(email: string, name: string, password: string) {
	await userEvent.type(await screen.findByLabelText('Email'), email)
	await userEvent.type(screen.getByLabelText('Name'), name)
	await userEvent.type(screen.getByLabelText('Password'), password)
	await userEvent.click(screen.getByRole('button', { name: 'Create user' }))
}

test('shows the create form with a disabled submit until it is filled', async () => {
	renderNewUser()

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
	expect(screen.getByLabelText('Name')).toBeInTheDocument()
	expect(screen.getByLabelText('Password')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Create user' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('creates a user and reports success upward', async () => {
	let body: unknown
	server.use(
		http.post('/api/users', async ({ request }) => {
			body = await request.json()
			return HttpResponse.json(grace, { status: 201 })
		}),
	)
	const onCreated = renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper', 'correct horse battery')

	await waitFor(() => expect(onCreated).toHaveBeenCalled())
	expect(body).toEqual({
		email: 'grace@example.com',
		name: 'Grace Hopper',
		password: 'correct horse battery',
	})
})

test('hints browsers not to autofill saved credentials', async () => {
	renderNewUser()

	expect(await screen.findByLabelText('Email')).toHaveAttribute(
		'autocomplete',
		'off',
	)
	expect(screen.getByLabelText('Password')).toHaveAttribute(
		'autocomplete',
		'new-password',
	)
})

test('shows the server validation message when the input is rejected', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json(
				{ error: 'password must be at least 12 characters' },
				{ status: 422 },
			),
		),
	)
	renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper', 'short')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'password must be at least 12 characters',
	)
})

test('shows a message when the email is already taken', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json({ error: 'email already taken' }, { status: 409 }),
		),
	)
	const onCreated = renderNewUser()

	await fillForm('ada@example.com', 'Ada', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'That email is already in use.',
	)
	expect(onCreated).not.toHaveBeenCalled()
	expect(screen.getByRole('heading', { name: 'New user' })).toBeInTheDocument()
})

test('shows a generic error when creation fails', async () => {
	server.use(
		http.post('/api/users', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper', 'correct horse battery')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The user could not be created.',
	)
})
