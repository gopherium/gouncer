// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import {
	HttpResponse,
	http,
	resetRequestOk,
	resetRequestRateLimited,
	server,
} from '../src/testing'
import { RequestResetScreen } from '../src/wpds'

function renderRequestReset(onBack?: () => void) {
	const client = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	})
	render(
		<QueryClientProvider client={client}>
			<RequestResetScreen brand="Testbed" onBack={onBack} />
		</QueryClientProvider>,
	)
}

async function submitEmail(email: string) {
	await userEvent.type(await screen.findByLabelText('Email'), email)
	await userEvent.click(screen.getByRole('button', { name: 'Send reset link' }))
}

const neutralSentence = 'If that address has an account, a reset link is on its way.'

test('shows the form with a disabled submit until an email is typed', async () => {
	renderRequestReset()

	expect(await screen.findByRole('heading', { name: 'Testbed' })).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Send reset link' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('hints browsers to offer the email address', async () => {
	renderRequestReset()

	expect(await screen.findByLabelText('Email')).toHaveAttribute('autocomplete', 'email')
})

test('replaces the form with the neutral sentence and moves focus to it', async () => {
	server.use(resetRequestOk())
	renderRequestReset()

	await submitEmail('maria@example.com')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent(neutralSentence)
	expect(status).toHaveFocus()
	expect(screen.queryByRole('button', { name: 'Send reset link' })).not.toBeInTheDocument()
})

test('locks the submit while the request is in flight', async () => {
	let posts = 0
	server.use(
		http.post('/api/auth/password-reset', () => {
			posts += 1
			return new Promise<never>(() => {})
		}),
	)
	renderRequestReset()

	await userEvent.type(await screen.findByLabelText('Email'), 'maria@example.com')
	const submit = screen.getByRole('button', { name: 'Send reset link' })
	await userEvent.click(submit)

	await waitFor(() => expect(submit).toHaveAttribute('aria-disabled', 'true'))
	await userEvent.click(submit)
	expect(posts).toBe(1)
})

test('posts the trimmed address the form carries', async () => {
	let body: unknown
	server.use(
		http.post('/api/auth/password-reset', async ({ request }) => {
			body = await request.json()
			return new HttpResponse(null, { status: 204 })
		}),
	)
	renderRequestReset()

	await submitEmail('  maria@example.com  ')

	await screen.findByRole('status')
	expect(body).toEqual({ email: 'maria@example.com' })
})

test('answers the same sentence whatever the address', async () => {
	server.use(resetRequestOk())
	renderRequestReset()

	await submitEmail('nobody-here@example.com')

	expect(await screen.findByRole('status')).toHaveTextContent(neutralSentence)
})

test('shows the throttling message when rate limited', async () => {
	server.use(resetRequestRateLimited())
	renderRequestReset()

	await submitEmail('maria@example.com')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'Too many attempts. Please wait a minute and try again.',
	)
})

test('shows a generic message when the request fails', async () => {
	server.use(
		http.post('/api/auth/password-reset', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderRequestReset()

	await submitEmail('maria@example.com')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'The request failed, please try again.',
	)
})

test('offers the way back only when one is provided', async () => {
	const onBack = vi.fn()
	renderRequestReset(onBack)

	await userEvent.click(await screen.findByRole('button', { name: 'Back to login' }))

	await waitFor(() => expect(onBack).toHaveBeenCalled())
})

test('renders no way back when none is provided', async () => {
	renderRequestReset()

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Back to login' })).not.toBeInTheDocument()
})
