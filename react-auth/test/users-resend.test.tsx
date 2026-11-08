// SPDX-License-Identifier: Apache-2.0

import { buttonClasses } from '@gopherium/godmin/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { sessionQueryKey } from '../src/session'
import { resendDelivered, resendRefused, resendUndelivered, server } from '../src/testing'
import { accountOf, holding, longName, menuOf, renderUsers, rowOf, signedIn, toast } from './users-harness'

const active = accountOf('ff', 'Ada Lovelace', { created_at: new Date('2026-07-06T10:00:00Z') })
const invited = accountOf('fd', 'Maria Perez', { confirmed: false })
const barredInvite = accountOf('fe', 'Ana Lopez', { confirmed: false, disabled: true })

beforeEach(() => holding([signedIn, active, invited, barredInvite]))

function held() {
	let release: () => void = () => {}
	const gate = new Promise<void>((resolve) => {
		release = resolve
	})
	return { gate, release }
}

async function openResend(name = 'Maria Perez') {
	await userEvent.click((await rowOf(name)).getByRole('button', { name: 'Actions' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: 'Resend invitation…' }))
	return within(await screen.findByRole('dialog', { name: 'Resend invitation' }))
}

async function closed() {
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
}

test('offers a resend only on the accounts still awaiting activation', async () => {
	renderUsers()

	expect(await menuOf('Maria Perez')).toContain('Resend invitation…')
	expect(await menuOf('Ada Lovelace')).not.toContain('Resend invitation…')
	expect(await menuOf('Ana Lopez')).not.toContain('Resend invitation…')
})

test('offers no resend on a pending account the reader may not edit', async () => {
	holding([signedIn, accountOf('fd', 'Maria Perez', { confirmed: false, editable: false })])
	renderUsers()

	await rowOf('Maria Perez')
	expect((await rowOf('Maria Perez')).queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
})

test('lists the actions that open a dialog first, with an ellipsis, and the one that acts at once last', async () => {
	renderUsers()

	expect(await menuOf('Maria Perez')).toEqual(['Change role…', 'Resend invitation…', 'Disable'])
})

test('opens the resend under its title, the question as its help sentence, an outline neutral Cancel', async () => {
	renderUsers()

	const modal = await openResend()

	const question = 'Send a new invitation to "Maria Perez" (maria@example.com)? The link sent before stops working.'
	expect(modal.getByRole('heading', { name: 'Resend invitation' })).toBeInTheDocument()
	expect(screen.getByRole('dialog', { name: 'Resend invitation', description: question })).toBeVisible()
	expect([...modal.getByRole('button', { name: 'Cancel' }).classList]).toEqual(
		buttonClasses('outline', undefined, 'neutral'),
	)
	expect(modal.getByRole('button', { name: 'Resend invitation' })).toBeInTheDocument()
})

test('names a long account whole in the resend question', async () => {
	holding([signedIn, accountOf('fd', longName, { confirmed: false })])
	renderUsers()

	const modal = await openResend(longName)

	expect(
		modal.getByText(`Send a new invitation to "${longName}" (maria@example.com)? The link sent before stops working.`),
	).toBeInTheDocument()
})

test('names a long account whole in the toast once its invitation went out, as WordPress does', async () => {
	holding([signedIn, accountOf('fd', longName, { confirmed: false })])
	server.use(resendDelivered())
	renderUsers()
	const modal = await openResend(longName)

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	expect(await toast(`Invitation sent to "${longName}".`)).toBeInTheDocument()
})

test('puts the focus on Cancel when the resend opens, as a WordPress confirm does', async () => {
	const laidOut = vi
		.spyOn(HTMLElement.prototype, 'getClientRects')
		.mockReturnValue([new DOMRect(0, 0, 10, 10)] as unknown as DOMRectList)
	onTestFinished(() => laidOut.mockRestore())
	renderUsers()

	const modal = await openResend()

	await waitFor(() => expect(modal.getByRole('button', { name: 'Cancel' })).toHaveFocus())
})

test('resends the invitation of a pending account and names the account in a toast', async () => {
	let sentTo: unknown
	server.use(
		http.post('/api/users/invite/resend', async ({ request }) => {
			sentTo = await request.json()
			return HttpResponse.json({ delivered: true })
		}),
	)
	renderUsers()
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	expect(await toast('Invitation sent to "Maria Perez".')).toBeInTheDocument()
	expect(sentTo).toEqual({ email: 'maria@example.com' })
	await closed()
})

test('shows the activation link in the dialog when no mail server delivered the resend', async () => {
	server.use(resendUndelivered('/activate?token=t-9'))
	renderUsers()
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	const line = 'No mail server is configured. Deliver the activation link by hand.'
	expect(await modal.findByText(line)).toBeInTheDocument()
	expect(screen.getByRole('dialog', { name: 'Resend invitation', description: line })).toBeVisible()
	expect(modal.getByLabelText('Activation link')).toHaveValue('/activate?token=t-9')
	expect(modal.getByLabelText('Activation link')).toHaveAttribute('readonly')
	await waitFor(() => expect(modal.getByLabelText('Activation link')).toHaveFocus())
	expect(modal.queryByRole('button', { name: 'Cancel' })).not.toBeInTheDocument()
	expect(screen.queryByText(/Invitation sent/)).not.toBeInTheDocument()
	await userEvent.click(modal.getByRole('button', { name: 'Done' }))
	await closed()
})

test('closes the resend without sending when the reader cancels', async () => {
	const sent = vi.fn()
	server.use(
		http.post('/api/users/invite/resend', () => {
			sent()
			return HttpResponse.json({ delivered: true })
		}),
	)
	renderUsers()
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Cancel' }))

	await closed()
	expect(sent).not.toHaveBeenCalled()
})

test('keeps the resend open while it sends, Cancel, the close icon and Escape doing nothing, then closes', async () => {
	const answer = held()
	server.use(
		http.post('/api/users/invite/resend', async () => {
			await answer.gate
			return HttpResponse.json({ delivered: true })
		}),
	)
	renderUsers()
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	await waitFor(() => expect(modal.getByRole('button', { name: 'Cancel' })).toHaveAttribute('aria-disabled', 'true'))
	expect(modal.getByRole('button', { name: 'Close' })).toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(modal.getByRole('button', { name: 'Cancel' }))
	await userEvent.click(modal.getByRole('button', { name: 'Close' }))
	await userEvent.keyboard('{Escape}')
	expect(screen.getByRole('dialog', { name: 'Resend invitation' })).toBeVisible()
	answer.release()
	expect(await toast('Invitation sent to "Maria Perez".')).toBeInTheDocument()
	await closed()
})

test('opens on the question again once a resend that showed its activation link closed', async () => {
	server.use(resendUndelivered('/activate?token=t-9'))
	renderUsers()
	const modal = await openResend()
	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))
	await modal.findByLabelText('Activation link')

	await userEvent.click(modal.getByRole('button', { name: 'Close' }))
	await closed()

	expect((await openResend()).getByRole('button', { name: 'Resend invitation' })).toBeInTheDocument()
})

test('signs the reader out when the session ended before the resend', async () => {
	server.use(http.post('/api/users/invite/resend', () => HttpResponse.json({ error: 'no session' }, { status: 401 })))
	const { client } = renderUsers()
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test("says inside the open dialog why a resend failed, in WordPress's words for the error code", async () => {
	server.use(resendRefused('user_not_editable'))
	renderUsers()
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	const words = 'Sorry, you are not allowed to edit this user.'
	expect(await modal.findByText(words)).toBeInTheDocument()
	expect(screen.queryByText(words, { selector: '.godmin-toast' })).not.toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.getByRole('dialog', { name: 'Resend invitation' })).toBeVisible()
})

test('says plainly inside the open dialog that a resend failed when the server names no reason', async () => {
	server.use(
		http.post('/api/users/invite/resend', () => HttpResponse.json({ error: 'the relay refused' }, { status: 500 })),
	)
	renderUsers()
	const modal = await openResend()

	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	const words = 'The invitation could not be sent.'
	expect(await modal.findByText(words)).toBeInTheDocument()
	expect(screen.queryByText(words, { selector: '.godmin-toast' })).not.toBeInTheDocument()
	expect(screen.queryByText('the relay refused')).not.toBeInTheDocument()
	expect(screen.getByRole('dialog', { name: 'Resend invitation' })).toBeVisible()
})

test('sends again on a second press after a refused resend, and closes with the toast once it went out', async () => {
	server.use(resendRefused('user_not_editable'))
	renderUsers()
	const modal = await openResend()
	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))
	await modal.findByText('Sorry, you are not allowed to edit this user.')

	server.use(resendDelivered())
	await userEvent.click(modal.getByRole('button', { name: 'Resend invitation' }))

	expect(await toast('Invitation sent to "Maria Perez".')).toBeInTheDocument()
	await closed()
})
