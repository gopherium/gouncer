// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { usersQueryKey } from '../src/admin'
import {
	HttpResponse,
	http,
	inviteDelivered,
	inviteUndelivered,
	server,
} from '../src/testing'
import { NewUserScreen } from '../src/wpds'

function renderNewUser(handler?: () => void | Promise<void>) {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	client.setQueryData(usersQueryKey, [])
	const onCreated = vi.fn(handler)
	render(
		<QueryClientProvider client={client}>
			<NewUserScreen onCreated={onCreated} />
		</QueryClientProvider>,
	)
	return { client, onCreated }
}

async function fillForm(email: string, name: string) {
	await userEvent.type(await screen.findByLabelText('Email'), email)
	await userEvent.type(screen.getByLabelText('Name'), name)
	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))
}

test('shows the invite form with no password field', async () => {
	renderNewUser()

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
	expect(screen.getByLabelText('Name')).toBeInTheDocument()
	expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Send invitation' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('locks the submit while the invitation is in flight', async () => {
	let posts = 0
	server.use(
		http.post('/api/users/invite', () => {
			posts += 1
			return new Promise<never>(() => {})
		}),
	)
	renderNewUser()

	await userEvent.type(await screen.findByLabelText('Email'), 'grace@example.com')
	await userEvent.type(screen.getByLabelText('Name'), 'Grace Hopper')
	const submit = screen.getByRole('button', { name: 'Send invitation' })
	await userEvent.click(submit)

	await waitFor(() => expect(submit).toHaveAttribute('aria-disabled', 'true'))
	await userEvent.click(submit)
	expect(posts).toBe(1)
})

test('sends the invitation and reports success upward', async () => {
	let body: unknown
	server.use(
		http.post('/api/users/invite', async ({ request }) => {
			body = await request.json()
			return HttpResponse.json({ delivered: true })
		}),
	)
	const { client, onCreated } = renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper')

	await waitFor(() => expect(onCreated).toHaveBeenCalled())
	expect(body).toEqual({ email: 'grace@example.com', name: 'Grace Hopper' })
	expect(client.getQueryState(usersQueryKey)?.isInvalidated).toBe(true)
})

test('stays on screen with the activation link when nothing mailed it', async () => {
	server.use(inviteUndelivered('https://crm.example.com/activate?token=t-123'))
	const { client, onCreated } = renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent(
		'No mail server is configured. Deliver the activation link by hand.',
	)
	expect(status).toHaveFocus()
	expect(screen.getByLabelText('Activation link')).toHaveValue(
		'https://crm.example.com/activate?token=t-123',
	)
	expect(screen.getByLabelText('Activation link')).toHaveAttribute('readonly')
	expect(onCreated).not.toHaveBeenCalled()
	expect(client.getQueryState(usersQueryKey)?.isInvalidated).toBe(true)

	await userEvent.click(screen.getByRole('button', { name: 'Done' }))
	await waitFor(() => expect(onCreated).toHaveBeenCalled())
})

test('keeps the form when the undelivered answer carries no link', async () => {
	server.use(
		http.post('/api/users/invite', () => HttpResponse.json({ delivered: false })),
	)
	const { onCreated } = renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The invitation could not be sent.',
	)
	expect(screen.queryByLabelText('Activation link')).not.toBeInTheDocument()
	expect(screen.getByLabelText('Email')).toHaveValue('grace@example.com')
	expect(onCreated).not.toHaveBeenCalled()
})

test('keeps the form locked while a delivered invitation hands off', async () => {
	let posts = 0
	server.use(
		http.post('/api/users/invite', () => {
			posts += 1
			return HttpResponse.json({ delivered: true })
		}),
	)
	const { onCreated } = renderNewUser(() => new Promise<void>(() => {}))

	await fillForm('grace@example.com', 'Grace Hopper')
	await waitFor(() => expect(onCreated).toHaveBeenCalled())

	const submit = screen.getByRole('button', { name: 'Send invitation' })
	expect(submit).toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(submit)
	expect(posts).toBe(1)
})

test('clears a failed handoff before the next invitation', async () => {
	let sent = 0
	server.use(
		http.post('/api/users/invite', () => {
			sent += 1
			return sent === 1
				? HttpResponse.json({ delivered: true })
				: HttpResponse.json({
						delivered: false,
						activation_link: 'https://crm.example.com/activate?token=t-2',
					})
		}),
	)
	renderNewUser(() => Promise.reject(new Error('the consumer could not navigate')))

	await fillForm('grace@example.com', 'Grace Hopper')
	expect(await screen.findByRole('alert')).toHaveTextContent('Something went wrong.')

	await userEvent.clear(screen.getByLabelText('Email'))
	await userEvent.clear(screen.getByLabelText('Name'))
	await fillForm('ada@example.com', 'Ada Lovelace')

	expect(await screen.findByLabelText('Activation link')).toHaveValue(
		'https://crm.example.com/activate?token=t-2',
	)
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('keeps the link on screen when handing off fails', async () => {
	server.use(inviteUndelivered('https://crm.example.com/activate?token=t-123'))
	renderNewUser(() => Promise.reject(new Error('the consumer could not navigate')))

	await fillForm('grace@example.com', 'Grace Hopper')
	await userEvent.click(await screen.findByRole('button', { name: 'Done' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('Something went wrong.')
	expect(screen.getByLabelText('Activation link')).toHaveValue(
		'https://crm.example.com/activate?token=t-123',
	)
})

test('locks Done while the handoff is in flight', async () => {
	server.use(inviteUndelivered('https://crm.example.com/activate?token=t-123'))
	const { onCreated } = renderNewUser(() => new Promise<void>(() => {}))

	await fillForm('grace@example.com', 'Grace Hopper')
	const done = await screen.findByRole('button', { name: 'Done' })
	await userEvent.click(done)

	await waitFor(() => expect(done).toHaveAttribute('aria-disabled', 'true'))
	await userEvent.click(done)
	expect(onCreated).toHaveBeenCalledTimes(1)
})

test('answers the same way whatever the address', async () => {
	server.use(inviteDelivered())
	const { onCreated } = renderNewUser()

	await fillForm('taken@example.com', 'Maria Perez')

	await waitFor(() => expect(onCreated).toHaveBeenCalled())
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('shows the server validation message when the input is rejected', async () => {
	server.use(
		http.post('/api/users/invite', () =>
			HttpResponse.json({ error: 'the name is too long' }, { status: 422 }),
		),
	)
	renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper')

	expect(await screen.findByRole('alert')).toHaveTextContent('the name is too long')
})

test('shows a generic error when the invitation fails', async () => {
	server.use(
		http.post('/api/users/invite', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	const { onCreated } = renderNewUser()

	await fillForm('grace@example.com', 'Grace Hopper')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The invitation could not be sent.',
	)
	expect(onCreated).not.toHaveBeenCalled()
})
