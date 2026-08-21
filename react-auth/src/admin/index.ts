// SPDX-License-Identifier: Apache-2.0

import { __ } from '@wordpress/i18n'
import { z } from 'zod'

import { UnauthorizedError } from '../api.js'
import { DOMAIN } from '../domain.js'
import { resolveTransport } from '../transport.js'

export const usersQueryKey = ['users'] as const

const userSchema = z.object({
	id: z.string(),
	email: z.string(),
	name: z.string(),
	disabled: z.boolean(),
	rank: z.string().default(''),
	created_at: z.coerce.date(),
})

export type User = z.infer<typeof userSchema>

export interface NewUser {
	email: string
	name: string
	password: string
	rank?: string
}

/**
 * EmailTakenError is thrown when a new user's email is already in use.
 */
export class EmailTakenError extends Error {}

/**
 * ValidationError is thrown when the backend rejects the submitted user
 * details; its message is the backend's human-readable explanation.
 */
export class ValidationError extends Error {}

/**
 * RankRefusedError is thrown when the actor holds no rank the server admits.
 */
export class RankRefusedError extends Error {}

/**
 * SelfRankError is thrown when an account changes its own rank.
 */
export class SelfRankError extends Error {}

/**
 * LastPrivilegedError is thrown when a write would leave no enabled
 * account holding a privileged rank.
 */
export class LastPrivilegedError extends Error {}

const errorSchema = z.object({ error: z.string(), code: z.string().optional() })

/**
 * Extracts the backend's explanation from a failed response body.
 * @param response - The failed HTTP response.
 * @param fallback - The message to use when the body is unreadable.
 * @returns The message explaining the failure.
 */
async function errorMessage(response: Response, fallback: string): Promise<string> {
	try {
		return errorSchema.parse(await response.json()).error
	} catch {
		return fallback
	}
}

/**
 * Lists the accounts over REST.
 * @param signal - Aborts the in-flight request.
 * @returns The parsed list of users.
 */
async function restFetchUsers(signal?: AbortSignal): Promise<User[]> {
	const response = await fetch('/api/users', { signal })
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (!response.ok) {
		throw new Error(`listing users failed with status ${response.status}`)
	}
	return z.array(userSchema).parse(await response.json())
}

/**
 * Creates an account over REST.
 * @param input - The email, name, and password of the new account.
 * @returns The created user.
 */
async function restCreateUser(input: NewUser): Promise<User> {
	const response = await fetch('/api/users', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (response.status === 409) {
		throw new EmailTakenError('email already in use')
	}
	if (response.status === 422) {
		throw new ValidationError(await errorMessage(response, __('invalid user details', DOMAIN)))
	}
	if (!response.ok) {
		throw new Error(`creating user failed with status ${response.status}`)
	}
	return userSchema.parse(await response.json())
}

/**
 * Updates an account's disabled flag over REST.
 * @param id - The identifier of the user to update.
 * @param disabled - Whether the account should be disabled.
 */
async function restSetUserDisabled(id: string, disabled: boolean): Promise<void> {
	const response = await fetch(`/api/users/${id}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ disabled }),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (!response.ok) {
		throw new Error(`updating user failed with status ${response.status}`)
	}
}

/**
 * Writes the rank an account holds over REST.
 * @param id - The identifier of the account to update.
 * @param rank - The rank the account is to hold.
 */
async function restSetUserRank(id: string, rank: string): Promise<void> {
	const response = await fetch(`/api/users/${id}/rank`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ rank }),
	})
	if (response.ok) {
		return
	}
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	const refusal = await refusalOf(response)
	if (refusal.code === 'rank_insufficient') {
		throw new RankRefusedError(refusal.message)
	}
	if (refusal.code === 'self_rank_refused') {
		throw new SelfRankError(refusal.message)
	}
	if (refusal.code === 'last_privileged_refused') {
		throw new LastPrivilegedError(refusal.message)
	}
	throw new Error(`updating rank failed with status ${response.status}`)
}

/**
 * Returns the code and message a refused response carries.
 * @param response - The refused HTTP response.
 * @returns The refusal code, if any, and its message.
 */
async function refusalOf(response: Response): Promise<{ code?: string, message: string }> {
	try {
		const held = errorSchema.parse(await response.json())
		return { code: held.code, message: held.error }
	} catch {
		return { message: __('the account could not be updated', DOMAIN) }
	}
}

/**
 * Returns every user account.
 * @param signal - Aborts the in-flight request.
 * @returns The parsed list of users.
 */
export async function fetchUsers(signal?: AbortSignal): Promise<User[]> {
	return resolveTransport('fetchUsers', restFetchUsers)(signal)
}

/**
 * Creates a user account and returns it.
 * @param input - The email, name, and password of the new account.
 * @returns The created user.
 */
export async function createUser(input: NewUser): Promise<User> {
	return resolveTransport('createUser', restCreateUser)(input)
}

/**
 * Sets whether a user may log in.
 * @param id - The identifier of the user to update.
 * @param disabled - Whether the account should be disabled.
 */
export async function setUserDisabled(id: string, disabled: boolean): Promise<void> {
	await resolveTransport('setUserDisabled', restSetUserDisabled)(id, disabled)
}

/**
 * Sets the rank an account holds.
 * @param id - The identifier of the account to update.
 * @param rank - The rank the account is to hold.
 */
export async function setUserRank(id: string, rank: string): Promise<void> {
	await resolveTransport('setUserRank', restSetUserRank)(id, rank)
}
