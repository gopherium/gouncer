// SPDX-License-Identifier: Apache-2.0

import { badgeClasses, setViewport } from '@gopherium/godmin/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, delay, http } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import { server, usersFailure } from '../src/testing'
import { withStylesheet } from './stylesheet'
import { accountOf, barred, colleague, holding, renderUsers, rowOf, shownNames, signedIn } from './users-harness'

const roleLabels = { admin: 'Administrator', member: 'Member' }

const longEmail = 'visiting.correspondent.for.the.regional.weekend.supplement@example.com'

beforeEach(() => holding([signedIn, colleague, barred]))

function clipping(element: HTMLElement) {
	const style = getComputedStyle(element)
	return {
		display: style.display,
		maxWidth: style.maxWidth,
		overflow: style.overflow,
		textOverflow: style.textOverflow,
		whiteSpace: style.whiteSpace,
	}
}

const clippedLine = {
	display: 'block',
	maxWidth: '100%',
	overflow: 'hidden',
	textOverflow: 'ellipsis',
	whiteSpace: 'nowrap',
}

async function viewOptions() {
	await screen.findByRole('table')
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))
	return within(await waitFor(() => document.querySelector('.dataviews-config__popover') as HTMLElement))
}

async function showCreated() {
	await userEvent.click((await viewOptions()).getByRole('button', { name: 'Created' }))
}

test('keeps the list toolbar while the accounts arrive', async () => {
	server.use(http.get('/api/users', () => new Promise(() => {})))
	renderUsers()

	expect(await screen.findByRole('searchbox', { name: 'Search users…' })).toBeInTheDocument()
	expect(screen.queryByRole('row')).not.toBeInTheDocument()
})

test('titles the page Users with what the page is for under it', async () => {
	renderUsers()

	const heading = await screen.findByRole('heading', { level: 1, name: 'Users' })
	expect(heading.closest('.godmin-page')).not.toBeNull()
	expect(screen.getByText('Manage who can sign in and what they can do.')).toHaveClass('godmin-page__subtitle')
})

test('draws the accounts inside a list region named Users, out to the canvas edges', async () => {
	renderUsers()

	const region = await screen.findByRole('region', { name: 'Users' })
	expect(region).toHaveClass('godmin-page__list')
	expect(within(region).getByRole('searchbox', { name: 'Search users…' })).toBeInTheDocument()
})

test('says the list is empty, and how to fill it, when no account exists', async () => {
	holding([])
	renderUsers()

	expect(await screen.findByRole('heading', { level: 2, name: 'No users yet.' })).toBeInTheDocument()
	expect(screen.getByText('Add one with Add User.').closest('.godmin-empty')).not.toBeNull()
})

test('says nobody matched when a search finds no account', async () => {
	renderUsers({ at: '/users?search=nobody' })

	expect(await screen.findByText('No users found.')).toBeInTheDocument()
	expect(screen.queryByText('No users yet.')).not.toBeInTheDocument()
})

test('says the accounts could not be loaded in place of the list', async () => {
	server.use(usersFailure())
	renderUsers()

	expect(await screen.findByRole('alert')).toHaveTextContent('Users could not be loaded.')
	expect(screen.queryByRole('searchbox')).not.toBeInTheDocument()
})

test('reads each account status as a badge, disabled before invited', async () => {
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { confirmed: false }), accountOf('fc', 'Grace Hopper', {
		confirmed: false,
		disabled: true,
	})])
	renderUsers()

	expect((await rowOf(signedIn.name)).getByText('Active')).toBeInTheDocument()
	expect((await rowOf('Ana Lopez')).getByText('Disabled')).toBeInTheDocument()
	expect((await rowOf('Maria Perez')).getByText('Invited')).toBeInTheDocument()
	expect((await rowOf('Grace Hopper')).getByText('Disabled')).toBeInTheDocument()
})

test('draws each status with its own badge, a disabled account in the outline one', async () => {
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { confirmed: false })])
	renderUsers()

	expect([...(await rowOf('Ana Lopez')).getByText('Disabled').classList]).toEqual(badgeClasses('none'))
	expect([...(await rowOf(signedIn.name)).getByText('Active').classList]).toEqual(badgeClasses('stable'))
	expect([...(await rowOf('Maria Perez')).getByText('Invited').classList]).toEqual(badgeClasses('informational'))
})

test('reads the role every account holds by the label the app gives it', async () => {
	holding([signedIn, colleague])
	renderUsers({ roleLabels })

	expect((await rowOf('Ada Lovelace')).getByText('Member')).toBeInTheDocument()
	expect((await rowOf(signedIn.name)).getByText('Administrator')).toBeInTheDocument()
})

test('reads None for an account that holds no role, as WordPress does', async () => {
	holding([signedIn, accountOf('fc', 'Maria Perez', { role: '' })])
	renderUsers({ roleLabels })

	expect((await rowOf('Maria Perez')).getByText('None')).toBeInTheDocument()
})

test('reads a role the app names no label for by its stored name with a capital first letter', async () => {
	holding([signedIn, accountOf('fc', 'Maria Perez', { role: 'team_lead' })])
	renderUsers({ roleLabels })

	expect((await rowOf('Maria Perez')).getByText('Team_lead')).toBeInTheDocument()
})

test('shows each account email under its name, in the name column', async () => {
	renderUsers()

	const row = await rowOf('Ada Lovelace')
	expect(row.getByText('ada@example.com').closest('td')).toBe(row.getByText('Ada Lovelace').closest('td'))
	expect(screen.queryByRole('columnheader', { name: /Email/ })).not.toBeInTheDocument()
})

test('draws a long email on one line of its own, clipped with an ellipsis at the name column edge', async () => {
	withStylesheet()
	holding([signedIn, accountOf('fd', 'Maria Perez', { email: longEmail })])
	renderUsers()

	const row = await rowOf('Maria Perez')
	const email = row.getByText(longEmail)
	expect(email.parentElement).toHaveClass('dataviews-view-table__primary-column-content')
	expect(clipping(email)).toEqual(clippedLine)
})

test('shows the whole email in a tooltip while the pointer rests on it, as the media file name does', async () => {
	holding([signedIn, accountOf('fd', 'Maria Perez', { email: longEmail })])
	renderUsers()

	const email = (await rowOf('Maria Perez')).getByText(longEmail)
	await userEvent.hover(email)

	const tip = await screen.findByText(longEmail, { ignore: '.gopherium-user-field__email' }, { timeout: 2000 })
	expect(email.closest('tr')).not.toContainElement(tip)
	expect(email).toHaveClass('gopherium-user-field__email')
})

test('clips a long email the same way in the phone list', async () => {
	withStylesheet()
	setViewport({ matches: true })
	holding([signedIn, accountOf('fd', 'Maria Perez', { email: longEmail })])
	renderUsers({ perPage: 20 })

	const item = (await screen.findByRole('button', { name: 'Maria Perez' })).closest('[role="row"]') as HTMLElement
	const email = within(item).getByText(longEmail)
	expect(email.parentElement).toHaveClass('dataviews-view-list__field')
	expect(clipping(email)).toEqual(clippedLine)
})

test('marks every name with its account, so a dialog finds the row again', async () => {
	renderUsers()

	expect((await rowOf('Ada Lovelace')).getByText('Ada Lovelace')).toHaveAttribute('data-item-id', colleague.id)
})

test('draws an initials avatar before every name, hidden from screen readers', async () => {
	renderUsers()

	const avatar = (await rowOf('Ada Lovelace')).getByText('A', { selector: '.godmin-avatar' })
	expect(avatar).toHaveAttribute('aria-hidden', 'true')
	expect(avatar.parentElement).toHaveClass('dataviews-column-primary__media')
})

test('draws the picture the server sends for an account in place of its initial', async () => {
	holding([signedIn, accountOf('fd', 'Maria Perez', { avatar: '/avatar/maria.png' })])
	renderUsers()

	const row = await rowOf('Maria Perez')
	const picture = row.getByText('Maria Perez').closest('tr')?.querySelector('img.godmin-avatar__image')
	expect(picture).toHaveAttribute('src', '/avatar/maria.png')
	expect(row.queryByText('M', { selector: '.godmin-avatar' })).not.toBeInTheDocument()
})

test('lets a reader hide the avatars through the view options, where they read as Avatar', async () => {
	renderUsers()
	await rowOf('Ada Lovelace')

	await userEvent.click(screen.getByRole('button', { name: 'View options' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Avatar' }))

	await waitFor(async () => expect((await rowOf('Ada Lovelace')).queryByText('A')).not.toBeInTheDocument())
})

test('dates every account in the abbreviated date and time WordPress lists show, once Created is shown', async () => {
	renderUsers()
	await showCreated()

	expect((await rowOf('Ana Lopez')).getByText('Jul 8, 2026 10:00 am')).toBeInTheDocument()
})

test('sorts the accounts newest first with no Created column, as WordPress lists users without one', async () => {
	renderUsers()

	expect(await shownNames()).toEqual([barred.name, colleague.name, signedIn.name])
	expect(screen.queryByRole('columnheader', { name: /Created/ })).not.toBeInTheDocument()
})

test('offers Created under View options, unticked, and shows it sorted newest first once picked', async () => {
	renderUsers()

	const created = (await viewOptions()).getByRole('button', { name: 'Created' })
	expect(created.querySelector('svg')).toBeNull()
	await userEvent.click(created)

	expect(await screen.findByRole('columnheader', { name: /Created/ })).toHaveAttribute('aria-sort', 'descending')
	expect(await shownNames()).toEqual([barred.name, colleague.name, signedIn.name])
})

test('sorts the accounts by email when the address says so, as WordPress sorts its Email column', async () => {
	renderUsers({ at: '/users?sort=email&order=desc' })

	expect(await shownNames()).toEqual([barred.name, signedIn.name, colleague.name])
})

test('sorts the accounts by name from A to Z when the address says so', async () => {
	renderUsers({ at: '/users?sort=name&order=asc' })

	expect(await shownNames()).toEqual([colleague.name, barred.name, signedIn.name])
})

test('never sorts by Status or Role, as WordPress sorts only its Username and Email columns', async () => {
	renderUsers()
	await screen.findByRole('table')

	for (const name of ['Status', 'Role']) {
		await userEvent.click(screen.getByRole('button', { name }))
		expect(await screen.findByRole('menuitem', { name: 'Hide column' })).toBeInTheDocument()
		expect(screen.queryByRole('menuitemradio', { name: 'Sort ascending' })).not.toBeInTheDocument()
		await userEvent.keyboard('{Escape}')
	}
})

test('offers Role and Status alone as filters, as WordPress narrows users only by role', async () => {
	renderUsers()

	await userEvent.click(await screen.findByRole('button', { name: 'Add filter' }))

	const offered = (await screen.findAllByRole('menuitem')).map((item) => item.textContent)
	expect(offered).toEqual(['Role', 'Status'])
})

test('searches the accounts by name', async () => {
	renderUsers()

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search users…' }), 'Lopez')

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez']))
})

test('searches the accounts by email', async () => {
	renderUsers()

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search users…' }), 'admin@')

	await waitFor(async () => expect(await shownNames()).toEqual([signedIn.name]))
})

test('narrows the accounts to the statuses the address names', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'status', operator: 'isAny', value: ['disabled'] }]))
	renderUsers({ at: `/users?filters=${filters}` })

	expect(await shownNames()).toEqual(['Ana Lopez'])
})

test('narrows the accounts to the roles the address names', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'role', operator: 'isAny', value: ['admin'] }]))
	renderUsers({ at: `/users?filters=${filters}` })

	expect(await shownNames()).toEqual([signedIn.name])
})

test('keeps the filter chips out of sight until a filter is picked, so the filter button stays live', async () => {
	renderUsers()

	await screen.findByRole('table')
	expect(screen.getAllByRole('button', { name: 'Status' })).toHaveLength(1)
	expect(screen.getByRole('button', { name: 'Add filter' })).not.toHaveAttribute('aria-disabled', 'true')
})

test('narrows the accounts to the status picked through the filter button', async () => {
	renderUsers()

	await userEvent.click(await screen.findByRole('button', { name: 'Add filter' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: 'Status' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Disabled' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez']))
})

test('offers as roles only the ones the accounts hold, by their labels from A to Z', async () => {
	holding([colleague, signedIn, barred, accountOf('fc', 'Maria Perez', { role: '' })])
	renderUsers({
		roleLabels,
		at: '/users?filters=' + encodeURIComponent(JSON.stringify([{ field: 'role', operator: 'isAny', value: [] }])),
	})

	await userEvent.click(await screen.findByRole('button', { name: 'Filter' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Role', pressed: false }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Administrator', 'Member', 'No role'])
})

test('offers as statuses only the ones the accounts hold, as WordPress lists only the roles in use', async () => {
	renderUsers({
		at: '/users?filters=' + encodeURIComponent(JSON.stringify([{ field: 'status', operator: 'isAny', value: [] }])),
	})

	await userEvent.click(await screen.findByRole('button', { name: 'Filter' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Status', pressed: false }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Active', 'Disabled'])
})

test('pages the accounts by the page size the app names', async () => {
	renderUsers({ perPage: 2, pageSizes: [2, 4] })

	expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace'])
	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))

	await waitFor(async () => expect(await shownNames()).toEqual([signedIn.name]))
})

test('offers as page sizes the ones the app names', async () => {
	renderUsers({ perPage: 2, pageSizes: [2, 4] })

	await screen.findByRole('table')
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))

	const sizes = within(await screen.findByRole('radiogroup', { name: 'Items per page' }))
	expect(sizes.getAllByRole('radio').map((size) => size.textContent)).toEqual(['2', '4'])
})

test('shows the toolbar and no rows while the page size is on its way', async () => {
	renderUsers({ perPage: null })

	expect(await screen.findByRole('searchbox', { name: 'Search users…' })).toBeInTheDocument()
	await delay(50)
	expect(screen.queryByRole('row')).not.toBeInTheDocument()
})

test('shows every account on one page when the page size could not be read', async () => {
	renderUsers({ perPage: null, sizesFailed: true })

	expect(await shownNames()).toEqual([barred.name, colleague.name, signedIn.name])
	expect(screen.queryByRole('button', { name: 'Next page' })).not.toBeInTheDocument()
})

test('keeps the rows on screen while the list is read again', async () => {
	const { client } = renderUsers({ perPage: 20 })
	await rowOf('Ada Lovelace')
	server.use(
		http.get('/api/users', async () => {
			await delay(100)
			return HttpResponse.json([signedIn, colleague, barred])
		}),
	)

	void client.invalidateQueries({ queryKey: ['users'] })

	await delay(20)
	expect(screen.getByText('Ada Lovelace')).toBeInTheDocument()
})

test('lays the accounts out as a list on a phone, the avatar before each name', async () => {
	setViewport({ matches: true })
	renderUsers({ perPage: 20 })

	const item = (await screen.findByRole('button', { name: 'Ada Lovelace' })).closest('[role="row"]') as HTMLElement
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
	expect(within(item).getByText('A', { selector: '.godmin-avatar' }).parentElement).toHaveClass(
		'dataviews-view-list__media-wrapper',
	)
})

test('draws the phone avatar at the 48px the phone list gives it', async () => {
	setViewport({ matches: true })
	renderUsers({ perPage: 20 })

	const item = (await screen.findByRole('button', { name: 'Ada Lovelace' })).closest('[role="row"]') as HTMLElement
	expect(within(item).getByText('A', { selector: '.godmin-avatar' })).toHaveStyle({ inlineSize: '48px' })
})
