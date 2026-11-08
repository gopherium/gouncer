// SPDX-License-Identifier: Apache-2.0

import { focusedTrigger } from '@gopherium/godmin'
import type { BulkRun, Trigger } from '@gopherium/godmin'
import type { Action } from '@wordpress/dataviews'
import { __ } from '@wordpress/i18n'
import { check, notAllowed, pencil } from '@wordpress/icons'
import { useMemo } from 'react'

import type { Account } from '../../account.js'
import { setUserDisabled } from '../../admin/index.js'
import type { User } from '../../api.js'
import { DOMAIN } from '../../domain.js'
import { switchWords } from './words.js'
import type { ErrorMessages } from './words.js'

/** What the actions of the users list need from the page. */
export interface ActionTools {
	/** The signed-in account, none while nobody is signed in. */
	reader: User | null
	/** Runs an Enable or a Disable over the picked accounts, reads the list again and shows one toast. */
	switches: BulkRun<Account>
	/** The words the app gives the error codes only it answers. */
	errorMessages: ErrorMessages
	/** Opens Change role on the accounts, given the trigger it opened from. */
	openRole: (accounts: Account[], trigger: Trigger) => void
	/** Opens Resend invitation on the account, given the trigger it opened from. */
	openResend: (account: Account, trigger: Trigger) => void
}

/**
 * Reports whether the reader may act on an account: never its own, and only one the server says it may edit.
 * @param reader - The signed-in account.
 * @param account - The account a row shows.
 * @returns Whether a row action may apply.
 */
function mayEdit(reader: User, account: Account): boolean {
	return account.id !== reader.id && account.editable
}

/**
 * Reports whether the reader may change the role of an account, one it may edit while it may give some role.
 * @param reader - The signed-in account, none while nobody is signed in.
 * @param account - The account a row shows.
 * @returns Whether Change role applies.
 */
export function mayChangeRole(reader: User | null, account: Account): boolean {
	return reader !== null && mayEdit(reader, account) && (reader.grantable ?? []).length > 0
}

/**
 * Returns the row and bulk actions of the users list, none on the reader's own row nor on one it may not edit.
 * @param tools - The reader, the run, the app's error words and what opens Change role and Resend invitation.
 * @returns Change role, Resend invitation, Enable and Disable, in the order the bulk bar draws them, none signed out.
 */
export function useUsersActions(tools: ActionTools): Action<Account>[] {
	const { reader, switches, errorMessages, openRole, openResend } = tools
	return useMemo(() => {
		if (reader === null) {
			return []
		}
		const grantable = reader.grantable ?? []
		const switchAccounts = (accounts: Account[], disabled: boolean) =>
			switches(accounts, (account) => setUserDisabled(account.id, disabled), switchWords(disabled, errorMessages))
		const actions: Action<Account>[] = [
			{
				id: 'change-role',
				label: __('Change role…', DOMAIN),
				icon: pencil,
				supportsBulk: true,
				isEligible: (account) => mayChangeRole(reader, account),
				callback: (accounts) => openRole(accounts, focusedTrigger()),
			},
			{
				id: 'resend-invitation',
				label: __('Resend invitation…', DOMAIN),
				isEligible: (account) => mayEdit(reader, account) && !account.confirmed && !account.disabled,
				callback: ([account]) => openResend(account, focusedTrigger()),
			},
			{
				id: 'enable',
				label: __('Enable', DOMAIN),
				icon: check,
				isPrimary: true,
				supportsBulk: true,
				isEligible: (account) => mayEdit(reader, account) && account.disabled && grantable.includes(account.role),
				callback: (accounts) => switchAccounts(accounts, false),
			},
			{
				id: 'disable',
				label: __('Disable', DOMAIN),
				icon: notAllowed,
				supportsBulk: true,
				isEligible: (account) => mayEdit(reader, account) && !account.disabled,
				callback: (accounts) => switchAccounts(accounts, true),
			},
		]
		return actions
	}, [reader, switches, errorMessages, openRole, openResend])
}
