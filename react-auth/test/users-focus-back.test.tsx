// SPDX-License-Identifier: Apache-2.0

import { setViewport } from '@gopherium/godmin/testing'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { accountOf, act, bulkBar, colleague, holding, renderUsers, rowOf, select, signedIn } from './users-harness'

const focus = vi.hoisted(() => ({ back: vi.fn() }))

vi.mock('@gopherium/godmin', async (importOriginal) => ({
	...(await importOriginal<typeof import('@gopherium/godmin')>()),
	useFocusBack: () => focus.back,
}))

const maria = accountOf('fd', 'Maria Perez')

beforeEach(() => {
	focus.back.mockClear()
	holding([signedIn, colleague, maria])
})

async function cancel() {
	await userEvent.click(await screen.findByRole('button', { name: 'Cancel' }))
}

async function nameButton() {
	const buttons = (await rowOf('Ada Lovelace')).queryAllByRole('button', { name: 'Ada Lovelace' })
	return buttons.find((button) => button.classList.contains('dataviews-title-field')) as HTMLElement
}

async function avatarButton() {
	return (await rowOf('Ada Lovelace')).getByText('A', { selector: '.godmin-avatar' }).parentElement as HTMLElement
}

test.each([
	{ door: 'title', open: async () => userEvent.click(await nameButton()) },
	{ door: 'media', open: async () => userEvent.click(await avatarButton()) },
	{ door: 'actions', open: async () => act('Ada Lovelace', 'Change role…') },
])('hands back the $door Change role opened from once it closes', async ({ door, open }) => {
	renderUsers()

	await open()
	await cancel()

	await waitFor(() => expect(focus.back).toHaveBeenLastCalledWith(colleague.id, door))
})

test('hands back the phone row Change role opened from once it closes', async () => {
	setViewport({ matches: true })
	renderUsers()

	await userEvent.click(await screen.findByRole('button', { name: 'Ada Lovelace' }))
	await cancel()

	await waitFor(() => expect(focus.back).toHaveBeenLastCalledWith(colleague.id, 'row'))
})

test('hands back the bulk bar Change role opened from once it closes, the list then taking the focus', async () => {
	renderUsers()

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Change role…' }))
	await cancel()

	await waitFor(() => expect(focus.back).toHaveBeenLastCalledWith(expect.any(String), 'bulk'))
})

test('hands back the actions menu Resend invitation opened from once it closes', async () => {
	const invited = accountOf('fd', 'Maria Perez', { confirmed: false })
	holding([signedIn, invited])
	renderUsers()

	await act('Maria Perez', 'Resend invitation…')
	await cancel()

	await waitFor(() => expect(focus.back).toHaveBeenLastCalledWith(invited.id, 'actions'))
})
