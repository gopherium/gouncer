// SPDX-License-Identifier: Apache-2.0

import { __, _n, sprintf } from '@wordpress/i18n'
import type { TransformedText } from '@wordpress/i18n'

import { ValidationError } from '../api.js'
import { DOMAIN } from '../domain.js'
import { EmailTakenError } from './errors.js'

/** UserField is a field of the Add User form an error can be about. */
export type UserField = 'email' | 'name' | 'password'

/** FieldError is the Add User field an error is about and the message shown under it. */
export interface FieldError {
	field: UserField
	message: string
}

const fieldMessages = new Map<string, [UserField, (meta: Record<string, unknown>) => string | undefined]>([
	['email_taken', ['email', () => __('This email is already registered. Please choose another one.', DOMAIN)]],
	['email_invalid', ['email', () => __('The email address is not correct.', DOMAIN)]],
	['name_required', ['name', () => __('Please provide a name.', DOMAIN)]],
	['name_too_long', ['name', () => __('This name is too long.', DOMAIN)]],
	['password_too_short', ['password', (meta) => (typeof meta.min === 'number' ? passwordHint(meta.min) : undefined)]],
	['password_too_long', ['password', () => __('This password is too long.', DOMAIN)]],
])

/**
 * Returns the password hint with the server's minimum as its number.
 * @param min - The fewest characters the server accepts.
 * @returns The hint.
 */
export function passwordHint(min: number): string {
	const hint = _n(
		'Hint: The password should be at least %d character long. ' +
			'To make it stronger, use upper and lower case letters, numbers, and symbols like ! " ? $ %% ^ & ).',
		'Hint: The password should be at least %d characters long. ' +
			'To make it stronger, use upper and lower case letters, numbers, and symbols like ! " ? $ %% ^ & ).',
		min,
		DOMAIN,
	) as TransformedText<'%d'>
	return sprintf(hint, min)
}

/**
 * Returns the code and meta an error a create threw carries.
 * @param error - The error the create threw.
 * @returns The code, empty when it carries none, and the meta.
 */
function detailsOf(error: unknown): [code: string, meta: Record<string, unknown>] {
	if (error instanceof ValidationError) {
		return [error.code ?? '', error.meta ?? {}]
	}
	return [error instanceof EmailTakenError ? error.code : '', {}]
}

/**
 * Returns the Add User field an error is about and the message shown under it.
 * @param error - The error createUser threw.
 * @returns The field and its message, or undefined for an error about no one field.
 */
export function fieldError(error: unknown): FieldError | undefined {
	const [code, meta] = detailsOf(error)
	const known = fieldMessages.get(code)
	const message = known?.[1](meta)
	return known && message !== undefined ? { field: known[0], message } : undefined
}

/**
 * Returns the words a link page shows for a password the server rejected.
 * @param error - The validation error the activation or the reset threw.
 * @returns The words Add User shows for the password, else the server's own.
 */
export function passwordWordsOf(error: ValidationError): string {
	const known = fieldError(error)
	return known?.field === 'password' ? known.message : error.message
}
