// SPDX-License-Identifier: Apache-2.0

import { buttonClasses, setViewport } from '@gopherium/godmin/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import { roleRefused, server, userWithCapabilities } from '../src/testing'
import {
	accountOf,
	act,
	barred,
	bulkBar,
	colleague,
	holding,
	longName,
	renderUsers,
	rowOf,
	select,
	signedIn,
	toast,
	writesRole,
} from './users-harness'

const roleLabels = { admin: 'Administrator', editor: 'Editor', author: 'Author', member: 'Member' }

beforeEach(() => holding([signedIn, colleague, barred]))

async function dialog() {
	return within(await screen.findByRole('dialog', { name: 'Change role' }))
}

async function pick(role: string) {
	await userEvent.click((await dialog()).getByRole('combobox', { name: 'Role' }))
	await userEvent.click(await screen.findByRole('option', { name: role }))
}

async function send() {
	await userEvent.click((await dialog()).getByRole('button', { name: 'Change role' }))
}

async function closed() {
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
}

async function nameButton(name: string) {
	const buttons = (await rowOf(name)).queryAllByRole('button', { name })
	return buttons.find((button) => button.classList.contains('dataviews-title-field'))
}

test('changes the role of another account from its row menu and names the account in a toast', async () => {
	const writes = writesRole()
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()

	expect(await toast('Role changed for "Ada Lovelace".')).toBeInTheDocument()
	expect(writes).toEqual([{ id: colleague.id, role: 'admin' }])
	await closed()
	await waitFor(async () => expect((await rowOf('Ada Lovelace')).getByText('Administrator')).toBeInTheDocument())
})

test('names a long account whole in the toast, as WordPress does', async () => {
	holding([signedIn, accountOf('fd', longName)])
	writesRole()
	renderUsers({ roleLabels })

	await act(longName, 'Change role…')
	await pick('Administrator')
	await send()

	expect(await toast(`Role changed for "${longName}".`)).toBeInTheDocument()
})

test('asks the role of the one account by its name, starting on the role it holds', async () => {
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')

	const opened = await dialog()
	expect(opened.getByText('Choose the role of Ada Lovelace.')).toBeInTheDocument()
	expect(opened.getByRole('combobox', { name: 'Role' })).toHaveTextContent('Member')
})

test('sends the role the account already holds when the reader asks, as every godmin form dialog does', async () => {
	const writes = writesRole()
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await send()

	expect(await toast('Role changed for "Ada Lovelace".')).toBeInTheDocument()
	expect(writes).toEqual([{ id: colleague.id, role: 'member' }])
})

test('offers only the roles the reader may give, the one with the fewest capabilities first', async () => {
	const session = userWithCapabilities('admin', ['manage_users'], ['admin', 'editor', 'author', 'member'])
	renderUsers({ roleLabels, session })

	await act('Ada Lovelace', 'Change role…')
	await userEvent.click((await dialog()).getByRole('combobox', { name: 'Role' }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Member', 'Author', 'Editor', 'Administrator'])
})

test('lifts the role choices into the overlay slot that stacks above the dialog', async () => {
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await userEvent.click((await dialog()).getByRole('combobox', { name: 'Role' }))

	const choice = await screen.findByRole('option', { name: 'Administrator' })
	expect(choice.closest('[data-wp-compat-overlay-slot]')).not.toBeNull()
})

test("opens as godmin's form dialog at 560px, Cancel in godmin's tone before Change role", async () => {
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')

	expect(await screen.findByRole('dialog', { name: 'Change role' })).toHaveClass('has-size-medium')
	const cancel = (await dialog()).getByRole('button', { name: 'Cancel' })
	expect([...cancel.classList]).toEqual(buttonClasses('minimal'))
	expect(cancel.compareDocumentPosition((await dialog()).getByRole('button', { name: 'Change role' })) & 4).toBeTruthy()
})

test('starts on an account whose role the reader may not give on Change role to…, Change role dimmed', async () => {
	holding([signedIn, accountOf('fd', 'Maria Perez', { role: 'auditor' })])
	renderUsers({ roleLabels })

	await act('Maria Perez', 'Change role…')

	expect((await dialog()).getByRole('combobox', { name: 'Role' })).toHaveTextContent('Change role to…')
	expect((await dialog()).getByRole('button', { name: 'Change role' })).toHaveAttribute('aria-disabled', 'true')
})

test('closes the moment Change role is pressed, before the write answers, as a WordPress row edit does', async () => {
	server.use(http.put('/api/users/:id/role', () => new Promise(() => {})))
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()

	await closed()
})

test('closes without writing when the reader cancels', async () => {
	const writes = writesRole()
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await userEvent.click((await dialog()).getByRole('button', { name: 'Cancel' }))

	await closed()
	expect(writes).toEqual([])
})

test('changes the role of every selected account from the bulk bar with one toast', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	const writes = writesRole()
	renderUsers({ roleLabels })

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Change role…' }))
	expect((await dialog()).getByText('Choose the role of 2 users.')).toBeInTheDocument()
	await pick('Administrator')
	await send()

	expect(await toast('Changed roles.')).toBeInTheDocument()
	expect(writes.map((write) => write.role)).toEqual(['admin', 'admin'])
	expect(document.querySelectorAll('.godmin-toast')).toHaveLength(1)
})

test('starts several accounts on Change role to…, with Change role dimmed until a role is picked', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	renderUsers({ roleLabels })

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Change role…' }))

	const opened = await dialog()
	expect(opened.getByRole('combobox', { name: 'Role' })).toHaveTextContent('Change role to…')
	expect(opened.getByRole('button', { name: 'Change role' })).toHaveAttribute('aria-disabled', 'true')
	await pick('Member')
	expect(opened.getByRole('button', { name: 'Change role' })).not.toHaveAttribute('aria-disabled', 'true')
	await pick('Change role to…')
	expect(opened.getByRole('button', { name: 'Change role' })).toHaveAttribute('aria-disabled', 'true')
})

test('says in a toast why the role could not be changed, in the words the error code names', async () => {
	server.use(roleRefused('role_changed'))
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()

	expect(await toast('The role of that account just changed. Reload the page and try again.')).toBeInTheDocument()
	expect(screen.queryByText(/Role changed for/)).not.toBeInTheDocument()
})

test('shows the toast as the role is written and marks the list until the accounts come back', async () => {
	writesRole()
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	server.use(http.get('/api/users', () => new Promise(() => {})))
	await send()

	expect(await toast('Role changed for "Ada Lovelace".')).toBeInTheDocument()
	await waitFor(() => expect(document.querySelector('.dataviews-footer')).toHaveAttribute('inert'))
})

test('marks nothing when every role write failed, while the accounts are read again', async () => {
	server.use(roleRefused('role_unknown'))
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	server.use(http.get('/api/users', () => new Promise(() => {})))
	await send()

	await toast('Sorry, you are not allowed to give users that role.')
	expect(document.querySelector('.dataviews-footer')).not.toHaveAttribute('inert')
})

test("says in WordPress's words that the reader may not give the role it picked", async () => {
	server.use(roleRefused('role_unknown'))
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()

	expect(await toast('Sorry, you are not allowed to give users that role.')).toBeInTheDocument()
	expect(screen.queryByText('unknown role')).not.toBeInTheDocument()
})

test('says plainly that a role could not be changed when the server names no reason', async () => {
	server.use(http.put('/api/users/:id/role', () => HttpResponse.json({ error: 'internal error' }, { status: 500 })))
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()

	expect(await toast('The role could not be changed.')).toBeInTheDocument()
})

test('counts the role changes that failed in one toast and marks the list for the one that worked', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez'), accountOf('fc', 'Grace Hopper')])
	server.use(
		http.put('/api/users/:id/role', ({ params }) =>
			params.id === colleague.id
				? new HttpResponse(null, { status: 204 })
				: HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderUsers({ roleLabels })

	await select('Ada Lovelace', 'Maria Perez', 'Grace Hopper')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Change role…' }))
	await pick('Administrator')
	server.use(http.get('/api/users', () => new Promise(() => {})))
	await send()

	expect(await toast('The roles of 2 users could not be changed.')).toBeInTheDocument()
	expect(document.querySelectorAll('.godmin-toast')).toHaveLength(1)
	await waitFor(() => expect(document.querySelector('.dataviews-footer')).toHaveAttribute('inert'))
})

test('keeps the role a failed change picked for the next time the dialog opens on that account', async () => {
	server.use(roleRefused('role_changed'))
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()
	await toast('The role of that account just changed. Reload the page and try again.')
	await act('Ada Lovelace', 'Change role…')

	expect((await dialog()).getByRole('combobox', { name: 'Role' })).toHaveTextContent('Administrator')
})

test('opens again on the role the account holds once a change went through', async () => {
	server.use(roleRefused('role_changed'))
	renderUsers({ roleLabels })
	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()
	await toast('The role of that account just changed. Reload the page and try again.')
	writesRole()
	await act('Ada Lovelace', 'Change role…')
	await pick('Member')
	await send()
	await toast('Role changed for "Ada Lovelace".')

	await act('Ada Lovelace', 'Change role…')

	expect((await dialog()).getByRole('combobox', { name: 'Role' })).toHaveTextContent('Member')
})

test('keeps no pick for a write a later one to the same account overtook', async () => {
	let answer: (response: Response) => void = () => {}
	server.use(
		http.put('/api/users/:id/role', () => new Promise<Response>((resolve) => {
			answer = resolve
		})),
	)
	renderUsers({ roleLabels })
	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()
	const first = answer
	writesRole()
	await act('Ada Lovelace', 'Change role…')
	await send()
	await toast('Role changed for "Ada Lovelace".')

	first(HttpResponse.json({ error: 'role changed', code: 'role_changed' }, { status: 409 }))
	await toast('The role of that account just changed. Reload the page and try again.')
	await act('Ada Lovelace', 'Change role…')

	expect((await dialog()).getByRole('combobox', { name: 'Role' })).toHaveTextContent('Member')
})

test('opens Change role from a click on the name of an account the reader may change', async () => {
	renderUsers({ roleLabels })

	await userEvent.click((await nameButton('Ada Lovelace')) as HTMLElement)

	expect((await dialog()).getByText('Choose the role of Ada Lovelace.')).toBeInTheDocument()
})

test('opens Change role from a click on the avatar of an account the reader may change', async () => {
	renderUsers({ roleLabels })

	const avatar = (await rowOf('Ada Lovelace')).getByText('A', { selector: '.godmin-avatar' })
	await userEvent.click(avatar.parentElement as HTMLElement)

	expect((await dialog()).getByText('Choose the role of Ada Lovelace.')).toBeInTheDocument()
})

test('keeps the name plain text on the own row and on a row the reader may not edit', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez', { editable: false })])
	renderUsers({ roleLabels })

	expect(await nameButton('Ada Lovelace')).toBeDefined()
	expect((await rowOf(signedIn.name)).queryByRole('button', { name: signedIn.name })).not.toBeInTheDocument()
	expect((await rowOf('Maria Perez')).queryByRole('button', { name: 'Maria Perez' })).not.toBeInTheDocument()
})

test('opens Change role from a tap on a phone row of an account the reader may change', async () => {
	setViewport({ matches: true })
	renderUsers({ roleLabels })

	await userEvent.click(await screen.findByRole('button', { name: 'Ada Lovelace' }))

	expect((await dialog()).getByText('Choose the role of Ada Lovelace.')).toBeInTheDocument()
})

test('opens nothing and selects nothing from a tap on a phone row the reader may not change', async () => {
	setViewport({ matches: true })
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez', { editable: false })])
	renderUsers({ roleLabels })

	const item = await screen.findByRole('button', { name: 'Maria Perez' })
	await userEvent.click(item)

	expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
	expect(item).toHaveAttribute('aria-pressed', 'false')
})

test('puts the focus back on the actions menu Change role opened from once the reader cancels', async () => {
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await userEvent.click((await dialog()).getByRole('button', { name: 'Cancel' }))

	await closed()
	await waitFor(async () => expect((await rowOf('Ada Lovelace')).getByRole('button', { name: 'Actions' })).toHaveFocus())
})

test('puts the focus back on the name Change role opened from once the role changed', async () => {
	writesRole()
	renderUsers({ roleLabels })

	await userEvent.click((await nameButton('Ada Lovelace')) as HTMLElement)
	await pick('Administrator')
	await send()

	await toast('Role changed for "Ada Lovelace".')
	await waitFor(async () => expect(await nameButton('Ada Lovelace')).toHaveFocus())
})

test('reads the list again after a refused change, the row then showing the role the account holds now', async () => {
	server.use(
		http.put('/api/users/:id/role', () => {
			holding([signedIn, { ...colleague, role: 'admin' }, barred])
			return HttpResponse.json({ error: 'role changed', code: 'role_changed' }, { status: 409 })
		}),
	)
	renderUsers({ roleLabels })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()

	await toast('The role of that account just changed. Reload the page and try again.')
	await waitFor(async () => expect((await rowOf('Ada Lovelace')).getByText('Administrator')).toBeInTheDocument())
})

test('keeps the focus in the list once a role change took the row out of the filtered view', async () => {
	writesRole()
	const filters = encodeURIComponent(JSON.stringify([{ field: 'role', operator: 'isAny', value: ['member'] }]))
	renderUsers({ roleLabels, at: `/users?filters=${filters}` })

	await act('Ada Lovelace', 'Change role…')
	await pick('Administrator')
	await send()

	await toast('Role changed for "Ada Lovelace".')
	await waitFor(() => {
		expect(screen.queryByRole('button', { name: 'Ada Lovelace' })).not.toBeInTheDocument()
		expect(screen.getByRole('region', { name: 'Users' })).toContainElement(document.activeElement as HTMLElement)
	})
})

test('puts the focus on the list once a change opened from the bulk bar ends', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	writesRole()
	renderUsers({ roleLabels })

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Change role…' }))
	await pick('Administrator')
	await send()

	await toast('Changed roles.')
	await waitFor(() => expect(screen.getByRole('region', { name: 'Users' })).toHaveFocus())
})
