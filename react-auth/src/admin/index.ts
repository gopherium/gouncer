// SPDX-License-Identifier: Apache-2.0

import { __ } from '@wordpress/i18n'
import { z } from 'zod'

import { RateLimitedError, UnauthorizedError, ValidationError } from '../api.js'
import { DOMAIN } from '../domain.js'
import { resolveTransport } from '../transport.js'

export const usersQueryKey = ['users'] as const

const userSchema = z.object({
	id: z.string(),
	email: z.string(),
	name: z.string(),
	disabled: z.boolean(),
	role: z.string().default(''),
	created_at: z.coerce.date(),
})

export type User = z.infer<typeof userSchema>

export interface NewUser {
	email: string
	name: string
	password: string
	role?: string
}

export interface NewInvite {
	email: string
	name: string
	role?: string
}

const invitationSchema = z.discriminatedUnion('delivered', [
	z.object({ delivered: z.literal(true) }),
	z.object({ delivered: z.literal(false), activation_link: z.string().min(1) }),
])

export type Invitation = z.infer<typeof invitationSchema>

/**
 * EmailTakenError is thrown when a new user's email is already in use.
 */
export class EmailTakenError extends Error {}

export { ValidationError } from '../api.js'

/**
 * RoleRefusedError is thrown when the actor holds no role the server admits.
 */
export class RoleRefusedError extends Error {}

/**
 * SelfRoleError is thrown when an account changes its own role.
 */
export class SelfRoleError extends Error {}

/**
 * LastPrivilegedError is thrown when a write would leave no enabled
 * account holding a privileged role.
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
 * Sends an invitation over REST, answering the same shape whatever the
 * address's state.
 * @param input - The email, name, and optional role of the invited account.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
async function restInvite(input: NewInvite): Promise<Invitation> {
	const response = await fetch('/api/users/invite', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (response.status === 422) {
		throw new ValidationError(
			await errorMessage(response, __('invalid invitation details', DOMAIN)),
		)
	}
	if (response.status === 429) {
		throw new RateLimitedError('too many attempts')
	}
	if (!response.ok) {
		throw new Error(`inviting failed with status ${response.status}`)
	}
	return invitationSchema.parse(await response.json())
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
 * Writes the role an account holds over REST.
 * @param id - The identifier of the account to update.
 * @param role - The role the account is to hold.
 */
async function restSetUserRole(id: string, role: string): Promise<void> {
	const response = await fetch(`/api/users/${id}/role`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ role }),
	})
	if (response.ok) {
		return
	}
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	const failed = await errorOf(response)
	if (failed.code === 'role_insufficient') {
		throw new RoleRefusedError(failed.message)
	}
	if (failed.code === 'self_role_refused') {
		throw new SelfRoleError(failed.message)
	}
	if (failed.code === 'last_privileged_refused') {
		throw new LastPrivilegedError(failed.message)
	}
	throw new Error(`updating role failed with status ${response.status}`)
}

/**
 * Returns the code and message a refused response carries.
 * @param response - The refused HTTP response.
 * @returns The error code, if any, and its message.
 */
async function errorOf(response: Response): Promise<{ code?: string, message: string }> {
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
 * Sets the role an account holds.
 * @param id - The identifier of the account to update.
 * @param role - The role the account is to hold.
 */
export async function setUserRole(id: string, role: string): Promise<void> {
	await resolveTransport('setUserRole', restSetUserRole)(id, role)
}

/**
 * Sends an invitation, answering the same shape whatever the address's
 * state.
 * @param input - The email, name, and optional role of the invited account.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
export async function invite(input: NewInvite): Promise<Invitation> {
	return resolveTransport('invite', restInvite)(input)
}
