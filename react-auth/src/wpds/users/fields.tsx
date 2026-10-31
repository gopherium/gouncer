// SPDX-License-Identifier: Apache-2.0

import { InitialsAvatar } from '@gopherium/godmin'
import { listDate } from '@gopherium/godmin/dates'
import type { Field } from '@wordpress/dataviews'
import { __, _x } from '@wordpress/i18n'
import { Badge, Tooltip, VisuallyHidden } from '@wordpress/ui'
import { cloneElement } from 'react'
import type { ReactElement, ReactNode } from 'react'

import type { Account } from '../../account.js'
import { DOMAIN } from '../../domain.js'
import { roleCellOf, roleLabelOf } from './words.js'
import type { RoleLabels } from './words.js'

/** One cell of a data column: the words shown, the words a screen reader hears and the link the words go in. */
export interface UsersColumnCell {
	/** The words shown in the cell. */
	text: string
	/** The words a screen reader hears in place of the shown ones, the shown ones when absent. */
	hidden?: string
	/** The link element the words go in on a wide screen, none for plain words. */
	link?: ReactElement
}

/** A column an app adds to the users list as data, shown after Role. */
export interface UsersColumn {
	/** The id the view names the column by. */
	id: string
	/** The column heading. */
	label: string
	/** The cell of each account, keyed by account id, an account missing from it showing an empty cell. */
	cells: Readonly<Record<string, UsersColumnCell>>
}

/** What the fields of the users list are built from beside the accounts. */
export interface FieldOptions {
	/** The labels the app gives its own roles. */
	roleLabels: RoleLabels
	/** Whether the site shows avatars, the list declaring no avatar field when it does not. */
	showAvatars: boolean
	/** The data columns the app adds. */
	columns: readonly UsersColumn[]
	/** Whether the viewport takes the phone layout, where a column draws no link. */
	phone: boolean
}

/** The state an account stands in. */
type UserStatus = 'active' | 'invited' | 'disabled'

/** Every state an account stands in, in the order the filter offers them. */
const STATUSES: readonly UserStatus[] = ['active', 'invited', 'disabled']

/** The badge intent each state is drawn with, disabled in the outline one. */
const STATUS_INTENTS: Record<UserStatus, 'stable' | 'informational' | 'none'> = {
	active: 'stable',
	invited: 'informational',
	disabled: 'none',
}

/** The width in pixels of the media box DataViews draws before each name in the phone list. */
const PHONE_AVATAR_SIZE = 48

/**
 * Returns the state one account stands in.
 * @param account - The account.
 * @returns Disabled before invited, invited before active.
 */
function statusOf(account: Account): UserStatus {
	if (account.disabled) {
		return 'disabled'
	}
	return account.confirmed ? 'active' : 'invited'
}

/**
 * Returns the label each state reads as, read fresh so the loaded catalogue answers.
 * @returns The labels, keyed by state.
 */
function statusLabels(): Record<UserStatus, string> {
	return {
		active: _x('Active', 'user status', DOMAIN),
		invited: _x('Invited', 'user status', DOMAIN),
		disabled: _x('Disabled', 'user status', DOMAIN),
	}
}

/**
 * Returns the name field, the title marked with its account so a dialog finds the row again.
 * @returns The field.
 */
function nameField(): Field<Account> {
	return {
		id: 'name',
		label: __('Name', DOMAIN),
		type: 'text',
		enableGlobalSearch: true,
		enableHiding: false,
		filterBy: false,
		render: ({ item }) => <span data-item-id={item.id}>{item.name}</span>,
	}
}

/**
 * Returns the email field, the line under the name, clipped with an ellipsis at the column edge, whole in a tooltip.
 * @returns The field.
 */
function emailField(): Field<Account> {
	return {
		id: 'email',
		label: __('Email', DOMAIN),
		type: 'text',
		enableGlobalSearch: true,
		filterBy: false,
		render: ({ item }) => (
			<Tooltip.Root>
				<Tooltip.Trigger render={<span className="gopherium-user-field__email">{item.email}</span>} />
				<Tooltip.Popup>{item.email}</Tooltip.Popup>
			</Tooltip.Root>
		),
	}
}

/**
 * Returns the avatar field, the picture the server sends or the initial of the name.
 * @param phone - Whether the viewport takes the phone layout, whose media box is larger.
 * @returns The field.
 */
function avatarField(phone: boolean): Field<Account> {
	const size = phone ? PHONE_AVATAR_SIZE : undefined
	return {
		id: 'avatar',
		label: __('Avatar', DOMAIN),
		render: ({ item }) => <InitialsAvatar name={item.name} src={item.avatar} size={size} />,
		enableSorting: false,
		filterBy: false,
	}
}

/**
 * Returns the status field, offering as filter values only the states the accounts stand in.
 * @param accounts - Every account the list holds.
 * @returns The field.
 */
function statusField(accounts: readonly Account[]): Field<Account> {
	const labels = statusLabels()
	const held = new Set(accounts.map(statusOf))
	return {
		id: 'status',
		label: __('Status', DOMAIN),
		type: 'text',
		getValue: ({ item }) => statusOf(item),
		elements: STATUSES.filter((status) => held.has(status)).map((status) => ({ value: status, label: labels[status] })),
		filterBy: { operators: ['isAny'] },
		enableSorting: false,
		render: ({ item }) => <Badge intent={STATUS_INTENTS[statusOf(item)]}>{labels[statusOf(item)]}</Badge>,
	}
}

/**
 * Returns the role field, offering as filter values only the roles the accounts hold, by label from A to Z.
 * @param accounts - Every account the list holds.
 * @param roleLabels - The labels the app gives its own roles.
 * @returns The field.
 */
function roleField(accounts: readonly Account[], roleLabels: RoleLabels): Field<Account> {
	const roles = [...new Set(accounts.map((account) => account.role))]
		.map((role) => ({ value: role, label: roleLabelOf(role, roleLabels) }))
		.sort((a, b) => a.label.localeCompare(b.label))
	return {
		id: 'role',
		label: __('Role', DOMAIN),
		type: 'text',
		elements: roles,
		filterBy: { operators: ['isAny'] },
		enableSorting: false,
		render: ({ item }) => roleCellOf(item.role, roleLabels),
	}
}

/**
 * Returns the words of one data column cell, a screen reader hearing its own words when the app gives them.
 * @param cell - The cell the app gives the account.
 * @param phone - Whether the viewport takes the phone layout, where the words go in no link.
 * @returns The words, in the link on a wide screen.
 */
function cellWords(cell: UsersColumnCell, phone: boolean): ReactNode {
	const words =
		cell.hidden === undefined ? (
			cell.text
		) : (
			<>
				<span aria-hidden="true">{cell.text}</span>
				<VisuallyHidden>{cell.hidden}</VisuallyHidden>
			</>
		)
	return phone || cell.link === undefined ? words : cloneElement(cell.link, undefined, words)
}

/**
 * Returns the field one data column becomes, never sorted nor filtered.
 * @param column - The column the app adds.
 * @param phone - Whether the viewport takes the phone layout.
 * @returns The field.
 */
function columnField(column: UsersColumn, phone: boolean): Field<Account> {
	return {
		id: column.id,
		label: column.label,
		enableSorting: false,
		filterBy: false,
		render: ({ item }) => (Object.hasOwn(column.cells, item.id) ? cellWords(column.cells[item.id], phone) : null),
	}
}

/**
 * Returns the created field, the abbreviated date and time WordPress lists show, sorted by the moment.
 * @returns The field.
 */
function createdField(): Field<Account> {
	return {
		id: 'created',
		label: __('Created', DOMAIN),
		getValue: ({ item }) => item.created_at.getTime(),
		render: ({ item }) => listDate(item.created_at),
		filterBy: false,
	}
}

/**
 * Returns the fields the users list shows.
 * @param accounts - Every account the list holds.
 * @param options - The role labels, whether avatars show, the data columns and whether the viewport is a phone.
 * @returns The fields.
 */
export function usersFields(accounts: readonly Account[], options: FieldOptions): Field<Account>[] {
	return [
		nameField(),
		emailField(),
		...(options.showAvatars ? [avatarField(options.phone)] : []),
		statusField(accounts),
		roleField(accounts, options.roleLabels),
		...options.columns.map((column) => columnField(column, options.phone)),
		createdField(),
	]
}
