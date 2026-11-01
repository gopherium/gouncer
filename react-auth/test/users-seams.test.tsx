// SPDX-License-Identifier: Apache-2.0

import { buttonClasses, setViewport } from '@gopherium/godmin/testing'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ButtonLink } from '@wordpress/ui'
import { useState } from 'react'
import { afterEach, beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { configureAuthTransport, resetAuthTransport } from '../src/transport'
import { RoleSelect, roleLabelOf } from '../src/wpds/users'
import type { UsersColumn } from '../src/wpds/users'
import { errorMessageOf } from '../src/wpds/users/words'
import { accountOf, act, barred, colleague, holding, renderUsers, rowOf, signedIn, toast } from './users-harness'

beforeEach(() => holding([signedIn, colleague, barred]))

afterEach(() => resetAuthTransport())

const posts: UsersColumn = {
	id: 'posts',
	label: 'Posts',
	cells: {
		[colleague.id]: { text: '3', hidden: '3 posts by this author', link: <a href="/posts?author=ada" /> },
		[barred.id]: { text: '0' },
	},
}

class CodedError extends Error {
	readonly code: string

	constructor(code: string) {
		super(code)
		this.code = code
	}
}

function buttonLinkClasses() {
	const { container, unmount } = render(
		<ButtonLink variant="solid" size="compact" href="/probe">
			Probe
		</ButtonLink>,
	)
	const classes = [...(container.firstElementChild as Element).classList]
	unmount()
	return classes
}

test("draws the Add User words in the app's link element as WordPress's compact solid ButtonLink", async () => {
	const look = buttonLinkClasses()
	renderUsers({ addLink: <a href="/users/new" /> })

	const add = await screen.findByRole('link', { name: 'Add User' })
	expect(add).toHaveAttribute('href', '/users/new')
	expect([...add.classList]).toEqual(look)
	expect(look).toEqual(expect.arrayContaining(buttonClasses('solid', 'compact').slice(-3)))
	expect(add.closest('header')).not.toBeNull()
})

test("repeats the Add User link at its default size in an empty list", async () => {
	holding([])
	renderUsers({ addLink: <a href="/users/new" /> })

	const heading = await screen.findByRole('heading', { level: 2, name: 'No users yet' })
	const add = within(heading.closest('.godmin-empty') as HTMLElement).getByRole('link', { name: 'Add User' })
	expect(add).toHaveAttribute('href', '/users/new')
	expect(add).not.toHaveClass(...buttonClasses('solid', 'compact').slice(-1))
})

test('tells the design system the Add User button is a link, so it keeps link semantics', async () => {
	const warned = vi.spyOn(console, 'error').mockImplementation(() => {})
	onTestFinished(() => warned.mockRestore())
	renderUsers({ addLink: <a href="/users/new" /> })

	await screen.findByRole('link', { name: 'Add User' })
	expect(warned.mock.calls.flat().join(' ')).not.toContain('nativeButton')
})

test('draws no header button when the app hands no link', async () => {
	renderUsers()

	await screen.findByRole('heading', { name: 'Users' })
	expect(screen.queryByRole('link', { name: 'Add User' })).not.toBeInTheDocument()
})

test('draws the tabs the app hands under the header', async () => {
	renderUsers({ tabs: <nav aria-label="User sections">Users and tokens</nav> })

	const tabs = await screen.findByRole('navigation', { name: 'User sections' })
	expect(tabs.parentElement).toHaveClass('godmin-page__tabs')
})

test('titles the page at the level the app names, the empty state one level under it', async () => {
	holding([])
	renderUsers({ headingLevel: 2 })

	expect(await screen.findByRole('heading', { level: 2, name: 'Users' })).toBeInTheDocument()
	expect(await screen.findByRole('heading', { level: 3, name: 'No users yet' })).toBeInTheDocument()
})

test('declares no avatar at all while the site shows none, so View options offers none', async () => {
	holding([signedIn, accountOf('fd', 'Maria Perez', { avatar: '/avatar/maria.png' })])
	renderUsers({ showAvatars: false })

	await rowOf('Maria Perez')
	expect(document.querySelector('.godmin-avatar')).toBeNull()
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))
	await screen.findByRole('radiogroup', { name: 'Items per page' })
	expect(screen.queryByRole('button', { name: 'Avatar' })).not.toBeInTheDocument()
})

test('draws a column the app adds after Role, its count in the link with the words a screen reader hears', async () => {
	renderUsers({ columns: [posts], columnIds: ['posts'] })

	const headers = (await screen.findAllByRole('columnheader')).map((header) => header.textContent)
	expect(headers.indexOf('Posts')).toBe(headers.indexOf('Role') + 1)
	const link = (await rowOf('Ada Lovelace')).getByRole('link', { name: '3 posts by this author' })
	expect(link).toHaveAttribute('href', '/posts?author=ada')
	expect(within(link).getByText('3')).toHaveAttribute('aria-hidden', 'true')
})

test('draws a cell with no link as plain words, and nothing for an account the column names no cell for', async () => {
	renderUsers({ columns: [posts], columnIds: ['posts'] })

	const zero = (await rowOf('Ana Lopez')).getByText('0')
	expect(zero.closest('a')).toBeNull()
	expect(zero).not.toHaveAttribute('aria-hidden')
	const cells = (await rowOf(signedIn.name)).getAllByRole('cell').map((cell) => cell.textContent)
	expect(cells).not.toContain('0')
	expect(cells).not.toContain('3')
})

test('never sorts by a column the app adds', async () => {
	renderUsers({ columns: [posts], columnIds: ['posts'] })

	await userEvent.click(await screen.findByRole('button', { name: 'Posts' }))

	expect(await screen.findByRole('menuitem', { name: 'Hide column' })).toBeInTheDocument()
	expect(screen.queryByRole('menuitemradio', { name: 'Sort ascending' })).not.toBeInTheDocument()
})

test('draws a column the reader shows on a phone as plain words, since the row is one button there', async () => {
	setViewport({ matches: true })
	renderUsers({ columns: [posts], columnIds: ['posts'] })

	await screen.findByRole('button', { name: 'Ada Lovelace' })
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Posts' }))

	const row = (await screen.findByRole('button', { name: /Ada Lovelace/ })).closest('[role="row"]') as HTMLElement
	await waitFor(() => expect(within(row).getByText('3 posts by this author')).toBeInTheDocument())
	expect(within(row).queryByRole('link')).not.toBeInTheDocument()
})

test('reads the accounts and writes through the transport the app configures', async () => {
	const writes: [string, boolean][] = []
	configureAuthTransport({
		fetchUsers: async () => [signedIn, colleague],
		setUserDisabled: async (id, disabled) => {
			writes.push([id, disabled])
		},
	})
	renderUsers()

	await act('Ada Lovelace', 'Disable')

	expect(await toast('"Ada Lovelace" disabled.')).toBeInTheDocument()
	expect(writes).toEqual([[colleague.id, true]])
})

test('says the words the app gives an error code only it answers', async () => {
	configureAuthTransport({
		fetchUsers: async () => [signedIn, colleague],
		setUserDisabled: async () => {
			throw new CodedError('capability_missing')
		},
	})
	renderUsers({ errorMessages: { capability_missing: 'You need the manage users capability.' } })

	await act('Ada Lovelace', 'Disable')

	expect(await toast('You need the manage users capability.')).toBeInTheDocument()
})

test.each([
	['user_not_editable', 'Sorry, you are not allowed to edit this user.'],
	['role_not_grantable', 'Sorry, you are not allowed to give users that role.'],
	['role_unknown', 'Sorry, you are not allowed to give users that role.'],
	['self_role_refused', 'You cannot change your own role.'],
	['self_disable_refused', 'You cannot disable your own account.'],
	['last_privileged_refused', 'Somebody must be able to manage users.'],
	['role_changed', 'The role of that account just changed. Reload the page and try again.'],
	['user_not_found', 'Invalid user ID.'],
	['role_insufficient', 'Sorry, you are not allowed to edit users.'],
])('words the error code %s as %s', (code, words) => {
	expect(errorMessageOf(new CodedError(code), {}, 'Fallback.')).toBe(words)
})

test('keeps its own words for a code it knows, whatever the app gives for it', () => {
	expect(errorMessageOf(new CodedError('user_not_found'), { user_not_found: 'Gone.' }, 'Fallback.')).toBe(
		'Invalid user ID.',
	)
})

test.each([
	['an error with no code', new Error('internal error')],
	['an error whose code nobody words', new CodedError('rate_limited')],
	['an error whose code is no string', Object.assign(new Error('odd'), { code: 429 })],
	['a thrown string', 'internal error'],
	['nothing at all', undefined],
])('falls back to its plain words for %s', (_name, error) => {
	expect(errorMessageOf(error, {}, 'Fallback.')).toBe('Fallback.')
})

test('reads a role by the label the app gives it, its own empty role included', () => {
	expect(roleLabelOf('admin', { admin: 'Administrator' })).toBe('Administrator')
	expect(roleLabelOf('', { '': 'No role for this site' })).toBe('No role for this site')
})

test('reads an empty role as No role and any other unnamed role by its stored name with a capital', () => {
	expect(roleLabelOf('')).toBe('No role')
	expect(roleLabelOf('team_lead')).toBe('Team_lead')
})

test('reads a plugin role named after a built in object key by its stored name, never the key', () => {
	expect(roleLabelOf('constructor', { admin: 'Administrator' })).toBe('Constructor')
	expect(roleLabelOf('__proto__', {})).toBe('__proto__')
})

function Picker({ placeholder, start = '' }: { placeholder?: string; start?: string }) {
	const [role, setRole] = useState(start)
	return (
		<>
			<RoleSelect
				grantable={['admin', 'editor', 'member']}
				value={role}
				onChange={setRole}
				roleLabels={{ admin: 'Administrator', member: 'Member' }}
				placeholder={placeholder}
			/>
			<output>{role === '' ? 'none' : role}</output>
		</>
	)
}

test('offers the roles the reader may give with the fewest capabilities first, for any form to use', async () => {
	render(<Picker start="member" />)

	expect(screen.getByRole('combobox', { name: 'Role' })).toHaveTextContent('Member')
	await userEvent.click(screen.getByRole('combobox', { name: 'Role' }))
	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Member', 'Editor', 'Administrator'])
	await userEvent.click(screen.getByRole('option', { name: 'Editor' }))
	expect(screen.getByRole('status')).toHaveTextContent('editor')
})

test('offers an empty first option under the words the form names, handing back no role for it', async () => {
	render(<Picker placeholder="Change role to…" start="admin" />)

	await userEvent.click(screen.getByRole('combobox', { name: 'Role' }))
	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Change role to…', 'Member', 'Editor', 'Administrator'])
	await userEvent.click(screen.getByRole('option', { name: 'Change role to…' }))
	expect(screen.getByRole('status')).toHaveTextContent('none')
	expect(screen.getByRole('combobox', { name: 'Role' })).toHaveTextContent('Change role to…')
})
