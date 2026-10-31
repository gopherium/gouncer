// SPDX-License-Identifier: Apache-2.0

import type { BulkFailure, BulkWords } from '@gopherium/godmin'
import { formatNumber } from '@gopherium/gottext'
import { __, _n, _x, sprintf } from '@wordpress/i18n'

import type { Account } from '../../account.js'
import { DOMAIN } from '../../domain.js'

/** The label an app gives each role it names, keyed by the role the server stores. */
export type RoleLabels = Readonly<Record<string, string>>

/** The words an app gives each error code only it answers, keyed by the code. */
export type ErrorMessages = Readonly<Record<string, string>>

/**
 * Returns the label a role reads as, a role the app names no label for reading as its stored name with a capital.
 * @param role - The role the server stores, empty for an account holding none.
 * @param roleLabels - The labels the app gives its own roles.
 * @returns The label, No role for an empty role the app names no label for.
 */
export function roleLabelOf(role: string, roleLabels: RoleLabels = {}): string {
	if (Object.hasOwn(roleLabels, role)) {
		return roleLabels[role]
	}
	return role === '' ? _x('No role', 'role', DOMAIN) : role.charAt(0).toUpperCase() + role.slice(1)
}

/**
 * Returns the words a Role cell reads for an account, None for one holding no role as WordPress writes it.
 * @param role - The role the server stores, empty for an account holding none.
 * @param roleLabels - The labels the app gives its own roles.
 * @returns The words.
 */
export function roleCellOf(role: string, roleLabels: RoleLabels): string {
	return role === '' ? _x('None', 'no user roles', DOMAIN) : roleLabelOf(role, roleLabels)
}

/**
 * Returns the words for each error code the account writes answer, WordPress's sentence wherever WordPress has one.
 * @returns The words, keyed by code, read fresh so the loaded catalogue answers.
 */
function codeMessages(): Record<string, string> {
	const notGrantable = __('Sorry, you are not allowed to give users that role.', DOMAIN)
	return {
		user_not_editable: __('Sorry, you are not allowed to edit this user.', DOMAIN),
		role_not_grantable: notGrantable,
		role_unknown: notGrantable,
		self_role_refused: __('You cannot change your own role.', DOMAIN),
		self_disable_refused: __('You cannot disable your own account.', DOMAIN),
		last_privileged_refused: __('Somebody must be able to manage users.', DOMAIN),
		role_changed: __('The role of that account just changed. Reload the page and try again.', DOMAIN),
		user_not_found: __('Invalid user ID.', DOMAIN),
		role_insufficient: __('Sorry, you are not allowed to edit users.', DOMAIN),
	}
}

/**
 * Returns the code an error carries, as every typed admin error does, and a plain write error when the server sent one.
 * @param error - What a write failed with.
 * @returns The code, none for an error carrying no code.
 */
function codeOf(error: unknown): string | undefined {
	const code = (error as { code?: unknown } | null | undefined)?.code
	return typeof code === 'string' ? code : undefined
}

/**
 * Returns the words a failed account write reads as: the words for its code, else the app's, else the fallback.
 * @param error - What the write failed with.
 * @param errorMessages - The words the app gives the codes only it answers.
 * @param fallback - The words for an error with no code either side words.
 * @returns The words.
 */
export function errorMessageOf(error: unknown, errorMessages: ErrorMessages, fallback: string): string {
	const code = codeOf(error)
	if (code === undefined) {
		return fallback
	}
	const known = codeMessages()
	if (Object.hasOwn(known, code)) {
		return known[code]
	}
	return Object.hasOwn(errorMessages, code) ? errorMessages[code] : fallback
}

/**
 * Returns the toast confirming the accounts one Enable or Disable changed.
 * @param disabled - Whether the accounts were disabled rather than enabled.
 * @param done - How many accounts changed.
 * @param only - The one account asked, when only one was.
 * @returns The confirmation, naming the account whole when only one was asked.
 */
function switchedMessage(disabled: boolean, done: number, only: Account | undefined): string {
	if (only !== undefined) {
		const template = disabled ? __('"%(name)s" disabled.', DOMAIN) : __('"%(name)s" enabled.', DOMAIN)
		return sprintf(template, { name: only.name })
	}
	const template = disabled
		? _n('%s user disabled.', '%s users disabled.', done, DOMAIN)
		: _n('%s user enabled.', '%s users enabled.', done, DOMAIN)
	return sprintf(template, formatNumber(done))
}

/**
 * Returns the toast naming the accounts one Enable or Disable could not change.
 * @param disabled - Whether the accounts were to be disabled rather than enabled.
 * @param failures - Each unchanged account with the error it failed with.
 * @param asked - How many accounts the action was asked to change.
 * @param errorMessages - The words the app gives the codes only it answers.
 * @returns The reason for one account, the count for several.
 */
function unswitchedMessage(
	disabled: boolean,
	failures: BulkFailure<Account>[],
	asked: number,
	errorMessages: ErrorMessages,
): string {
	if (asked === 1) {
		const fallback = disabled
			? __('The user could not be disabled.', DOMAIN)
			: __('The user could not be enabled.', DOMAIN)
		return errorMessageOf(failures[0].error, errorMessages, fallback)
	}
	const template = disabled
		? _n('%s user could not be disabled.', '%s users could not be disabled.', failures.length, DOMAIN)
		: _n('%s user could not be enabled.', '%s users could not be enabled.', failures.length, DOMAIN)
	return sprintf(template, formatNumber(failures.length))
}

/**
 * Returns the line Change role opens with, naming the one account or counting several.
 * @param accounts - The accounts the dialog acts on.
 * @returns The line.
 */
export function roleLineOf(accounts: readonly Account[]): string {
	if (accounts.length === 1) {
		return sprintf(__('Choose the role of %(name)s.', DOMAIN), { name: accounts[0].name })
	}
	const template = _n('Choose the role of %s user.', 'Choose the role of %s users.', accounts.length, DOMAIN)
	return sprintf(template, formatNumber(accounts.length))
}

/**
 * Returns the words a Change role run reports its outcome in, WordPress's Changed roles. for several.
 * @param errorMessages - The words the app gives the codes only it answers.
 * @returns The words for the accounts changed, naming one account whole, and for the ones that failed.
 */
export function roleWords(errorMessages: ErrorMessages): BulkWords<Account> {
	return {
		done: (_done, only) =>
			only === undefined
				? __('Changed roles.', DOMAIN)
				: sprintf(__('Role changed for "%(name)s".', DOMAIN), { name: only.name }),
		failed: (failures, asked) => {
			if (asked === 1) {
				return errorMessageOf(failures[0].error, errorMessages, __('The role could not be changed.', DOMAIN))
			}
			const count = failures.length
			const template = _n(
				'The role of %s user could not be changed.',
				'The roles of %s users could not be changed.',
				count,
				DOMAIN,
			)
			return sprintf(template, formatNumber(count))
		},
	}
}

/**
 * Returns the words an Enable or a Disable run reports its outcome in.
 * @param disabled - Whether the run disables the accounts rather than enables them.
 * @param errorMessages - The words the app gives the codes only it answers.
 * @returns The words for the accounts changed and for the ones that failed.
 */
export function switchWords(disabled: boolean, errorMessages: ErrorMessages): BulkWords<Account> {
	return {
		done: (done, only) => switchedMessage(disabled, done, only),
		failed: (failures, asked) => unswitchedMessage(disabled, failures, asked, errorMessages),
	}
}
