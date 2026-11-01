// SPDX-License-Identifier: Apache-2.0

import { setViewport } from '@gopherium/godmin/testing'
import { rememberFormatLocale } from '@gopherium/gottext'
import { resetLocale } from '@gopherium/gottext/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { disabledRefused, server, userWithCapabilities } from '../src/testing'
import {
	accountOf,
	act,
	barred,
	bulkBar,
	colleague,
	holding,
	longName,
	menuOf,
	renderUsers,
	rowOf,
	select,
	signedIn,
	toast,
	writesDisabled,
} from './users-harness'

beforeEach(() => holding([signedIn, colleague, barred]))

function inFormatLocale(locale: string) {
	rememberFormatLocale(locale)
	onTestFinished(() => resetLocale())
}

test('counts the accounts, because the list offers bulk actions', async () => {
	renderUsers()

	expect(await screen.findByText('3 Items')).toBeInTheDocument()
})

test('offers no row action on the signed-in account', async () => {
	renderUsers()

	await rowOf('Ada Lovelace')
	expect((await rowOf(signedIn.name)).queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
	expect((await rowOf(signedIn.name)).getByRole('checkbox')).toHaveAttribute('aria-disabled', 'true')
})

test('offers no enable on the signed-in account, even while it reads disabled', async () => {
	holding([{ ...signedIn, disabled: true }, colleague])
	renderUsers()

	await rowOf('Ada Lovelace')
	const own = await rowOf(signedIn.name)
	expect(own.getByText('Disabled')).toBeInTheDocument()
	expect(own.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
	expect(own.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
})

test('offers nothing at all on an account the server says the reader may not edit', async () => {
	holding([
		signedIn,
		accountOf('fd', 'Maria Perez'),
		accountOf('fb', 'Grace Hopper', { role: 'steward', editable: false }),
		accountOf('fa', 'Alan Turing', { role: 'steward', disabled: true, editable: false }),
	])
	renderUsers()

	expect(await menuOf('Maria Perez')).toEqual(['Change role…', 'Disable'])
	for (const name of ['Grace Hopper', 'Alan Turing']) {
		const row = await rowOf(name)
		expect(row.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
		expect(row.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
		expect(row.getByRole('checkbox')).toHaveAttribute('aria-disabled', 'true')
	}
})

test('offers Enable only on a disabled account whose role the reader may give', async () => {
	holding([
		signedIn,
		accountOf('fd', 'Maria Perez', { disabled: true }),
		accountOf('fc', 'Grace Hopper', { disabled: true, role: '' }),
		accountOf('fb', 'Alan Turing', { disabled: true, role: 'auditor' }),
	])
	renderUsers()

	expect((await rowOf('Maria Perez')).getByRole('button', { name: 'Enable' })).toBeInTheDocument()
	for (const name of ['Grace Hopper', 'Alan Turing']) {
		const row = await rowOf(name)
		expect(row.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
		expect(await menuOf(name)).toEqual(['Change role…'])
	}
})

test('offers no Change role when the reader may give no role at all, a list the server leaves out', async () => {
	renderUsers({ session: { ...userWithCapabilities('admin', ['manage_users']), grantable: undefined } })

	expect(await menuOf('Ada Lovelace')).toEqual(['Disable'])
})

test('offers no action at all while nobody is signed in', async () => {
	renderUsers({ session: null })

	await rowOf('Ada Lovelace')
	expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
})

test('lists Change role first and Disable, the one that locks the account out, last', async () => {
	renderUsers()

	expect(await menuOf('Ada Lovelace')).toEqual(['Change role…', 'Disable'])
})

test('lists the primary one click action first, as a WordPress row menu does', async () => {
	renderUsers()

	expect(await menuOf('Ana Lopez')).toEqual(['Enable', 'Change role…'])
})

test('draws Enable on a disabled row as its primary action, and never Disable', async () => {
	renderUsers()

	expect((await rowOf('Ana Lopez')).getByRole('button', { name: 'Enable' })).toBeInTheDocument()
	const active = await rowOf('Ada Lovelace')
	expect(active.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
	expect(active.queryByRole('button', { name: 'Disable' })).not.toBeInTheDocument()
})

test('disables an account from its row actions and names it in a toast', async () => {
	const writes = writesDisabled()
	renderUsers()

	await act('Ada Lovelace', 'Disable')

	expect(await toast('"Ada Lovelace" disabled.')).toBeInTheDocument()
	expect(writes).toEqual([{ id: colleague.id, disabled: true }])
	await waitFor(async () => expect((await rowOf('Ada Lovelace')).getByText('Disabled')).toBeInTheDocument())
})

test('enables a disabled account from its row actions and names it in a toast', async () => {
	const writes = writesDisabled()
	renderUsers()

	await act('Ana Lopez', 'Enable')

	expect(await toast('"Ana Lopez" enabled.')).toBeInTheDocument()
	expect(writes).toEqual([{ id: barred.id, disabled: false }])
})

test('enables a disabled account from the Enable its row draws', async () => {
	const writes = writesDisabled()
	renderUsers()

	await userEvent.click((await rowOf('Ana Lopez')).getByRole('button', { name: 'Enable' }))

	expect(await toast('"Ana Lopez" enabled.')).toBeInTheDocument()
	expect(writes).toEqual([{ id: barred.id, disabled: false }])
})

test.each([
	{ action: 'Disable', disabled: false, said: 'disabled' },
	{ action: 'Enable', disabled: true, said: 'enabled' },
])('names a long account whole in the $action toast, as WordPress does', async ({ action, disabled, said }) => {
	holding([signedIn, accountOf('fd', longName, { disabled })])
	writesDisabled()
	renderUsers()

	await act(longName, action)

	expect(await toast(`"${longName}" ${said}.`)).toBeInTheDocument()
})

test('says in a toast why an account could not be disabled, in the words the error code names', async () => {
	server.use(disabledRefused('last_privileged_refused'))
	renderUsers()

	await act('Ada Lovelace', 'Disable')

	expect(await toast('Somebody must be able to manage users.')).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.queryByText('"Ada Lovelace" disabled.')).not.toBeInTheDocument()
})

test('says in WordPress words that the reader may not give the role of the account it enables', async () => {
	holding([signedIn, accountOf('fd', 'Maria Perez', { disabled: true })])
	server.use(disabledRefused('role_not_grantable'))
	renderUsers()

	await act('Maria Perez', 'Enable')

	expect(await toast('Sorry, you are not allowed to give users that role.')).toBeInTheDocument()
	expect(screen.queryByText('"Maria Perez" enabled.')).not.toBeInTheDocument()
})

test('says in WordPress words that the reader may no longer edit the account', async () => {
	server.use(disabledRefused('user_not_editable'))
	renderUsers()

	await act('Ada Lovelace', 'Disable')

	expect(await toast('Sorry, you are not allowed to edit this user.')).toBeInTheDocument()
})

test('says in WordPress words that the reader may no longer edit users at all', async () => {
	server.use(disabledRefused('role_insufficient'))
	renderUsers()

	await act('Ada Lovelace', 'Disable')

	expect(await toast('Sorry, you are not allowed to edit users.')).toBeInTheDocument()
})

test('says plainly that an account could not be enabled when the server names no reason', async () => {
	server.use(http.patch('/api/users/:id', () => HttpResponse.json({ error: 'internal error' }, { status: 500 })))
	renderUsers()

	await act('Ana Lopez', 'Enable')

	expect(await toast('The user could not be enabled.')).toBeInTheDocument()
	expect(screen.queryByText('internal error')).not.toBeInTheDocument()
})

test('says plainly that an account could not be disabled when the server names no reason', async () => {
	server.use(http.patch('/api/users/:id', () => HttpResponse.json({ error: 'internal error' }, { status: 500 })))
	renderUsers()

	await act('Ada Lovelace', 'Disable')

	expect(await toast('The user could not be disabled.')).toBeInTheDocument()
})

test('disables every selected account at once with one toast counting them in the format locale', async () => {
	inFormatLocale('es-ES-u-nu-deva')
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	const writes = writesDisabled()
	renderUsers()

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Disable' }))

	expect(await toast('२ users disabled.')).toBeInTheDocument()
	expect(writes.map((write) => write.disabled)).toEqual([true, true])
})

test('offers Change role and Enable before Disable in the bulk bar, the one that locks accounts out last', async () => {
	renderUsers()

	await select('Ada Lovelace', 'Ana Lopez')

	const bar = await bulkBar()
	const change = await bar.findByRole('button', { name: 'Change role…' })
	const enable = bar.getByRole('button', { name: 'Enable' })
	const disable = bar.getByRole('button', { name: 'Disable' })
	expect(change.compareDocumentPosition(enable) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
	expect(enable.compareDocumentPosition(disable) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('counts only the accounts a bulk disable could not change, in one toast', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	server.use(
		http.patch('/api/users/:id', ({ params }) =>
			params.id === colleague.id
				? new HttpResponse(null, { status: 204 })
				: HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)
	renderUsers()

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Disable' }))

	expect(await toast('1 user could not be disabled.')).toBeInTheDocument()
	expect(screen.queryByText('1 user disabled.')).not.toBeInTheDocument()
	expect(document.querySelectorAll('.godmin-toast')).toHaveLength(1)
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('enables every selected account at once with one toast', async () => {
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { disabled: true })])
	writesDisabled()
	renderUsers()

	await select('Ana Lopez', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Enable' }))

	expect(await toast('2 users enabled.')).toBeInTheDocument()
})

test('counts the accounts a bulk enable could not change in one toast, in the format locale', async () => {
	inFormatLocale('es-ES-u-nu-deva')
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { disabled: true })])
	server.use(http.patch('/api/users/:id', () => HttpResponse.json({ error: 'internal error' }, { status: 500 })))
	renderUsers()

	await select('Ana Lopez', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Enable' }))

	expect(await toast('२ users could not be enabled.')).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('draws each bulk action with an icon, the only part of it a medium screen shows', async () => {
	setViewport({ width: 700 })
	renderUsers()

	await select('Ada Lovelace', 'Ana Lopez')

	const bar = await bulkBar()
	for (const name of ['Change role…', 'Enable', 'Disable']) {
		expect((await bar.findByRole('button', { name })).querySelector('svg'), name).not.toBeNull()
	}
})

test('puts the focus on the list once a bulk disable ends with the reader still on it', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	writesDisabled()
	renderUsers()

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Disable' }))

	await toast('2 users disabled.')
	await waitFor(() => expect(screen.getByRole('region', { name: 'Users' })).toHaveFocus())
})

test('marks the list reloading after a bulk run until the accounts come back', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	writesDisabled()
	renderUsers()

	await select('Ada Lovelace', 'Maria Perez')
	server.use(http.get('/api/users', () => new Promise(() => {})))
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Disable' }))

	await toast('2 users disabled.')
	await waitFor(() => expect(document.querySelector('.dataviews-footer')).toHaveAttribute('inert'))
})

test('reads the list again once after a bulk run, whatever the count', async () => {
	const reads = vi.fn()
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	writesDisabled()
	server.use(
		http.get('/api/users', () => {
			reads()
			return HttpResponse.json([signedIn, colleague, accountOf('fd', 'Maria Perez')])
		}),
	)
	renderUsers()
	await rowOf('Maria Perez')
	reads.mockClear()

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Disable' }))

	await toast('2 users disabled.')
	expect(reads).toHaveBeenCalledTimes(1)
	expect(within(screen.getByRole('region', { name: 'Users' })).getAllByRole('row')).toHaveLength(4)
})
