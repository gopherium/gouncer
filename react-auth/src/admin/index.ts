// SPDX-License-Identifier: Apache-2.0

import { __ } from '@wordpress/i18n'
import { z } from 'zod'

import { accountSchema } from '../account.js'
import type { Account as User } from '../account.js'
import { RateLimitedError, UnauthorizedError, ValidationError, refusalOf } from '../api.js'
import { DOMAIN } from '../domain.js'
import { resolveTransport } from '../transport.js'
import {
	EmailTakenError,
	LastPrivilegedError,
	RoleChangedError,
	RoleNotGrantableError,
	RoleRefusedError,
	RoleUnknownError,
	SelfDisableError,
	SelfRoleError,
	UserNotEditableError,
	UserNotFoundError,
} from './errors.js'

export const usersQueryKey = ['users'] as const

export type { Account as User } from '../account.js'

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
	z.object({ delivered: z.literal(false), activation_link: z.string().trim().min(1) }),
])

export type Invitation = z.infer<typeof invitationSchema>

export { ValidationError } from '../api.js'
export {
	EmailTakenError,
	LastPrivilegedError,
	RoleChangedError,
	RoleNotGrantableError,
	RoleRefusedError,
	RoleUnknownError,
	SelfDisableError,
	SelfRoleError,
	UserNotEditableError,
	UserNotFoundError,
} from './errors.js'
export { fieldError } from './fieldError.js'
export type { FieldError, UserField } from './fieldError.js'

type Refused = Awaited<ReturnType<typeof refusalOf>>

type ErrorClasses = ReadonlyMap<string, new (message: string) => Error>

const roleErrors: ErrorClasses = new Map<string, new (message: string) => Error>([
	['role_unknown', RoleUnknownError],
	['user_not_editable', UserNotEditableError],
	['role_not_grantable', RoleNotGrantableError],
])

const accountErrors: ErrorClasses = new Map<string, new (message: string) => Error>([
	...roleErrors,
	['role_changed', RoleChangedError],
	['user_not_found', UserNotFoundError],
	['last_privileged_refused', LastPrivilegedError],
	['role_insufficient', RoleRefusedError],
])

const disableErrors: ErrorClasses = new Map<string, new (message: string) => Error>([
	...accountErrors,
	['self_disable_refused', SelfDisableError],
])

const roleChangeErrors: ErrorClasses = new Map<string, new (message: string) => Error>([
	...accountErrors,
	['self_role_refused', SelfRoleError],
])

type StatusErrors = ReadonlyMap<number, () => Error>

const creationStatusErrors: StatusErrors = new Map([[409, () => new EmailTakenError('email already in use')]])

const invitationStatusErrors: StatusErrors = new Map([[429, () => new RateLimitedError('too many attempts')]])

interface AccountPost {
	path: string
	body: object
	write: string
	fallback: string
	errorsByStatus: StatusErrors
}

type InvitationRoute = Pick<AccountPost, 'path' | 'write'>

interface FailedWrite {
	write: string
	status: number
}

const inviteRoute: InvitationRoute = { path: '/api/users/invite', write: 'inviting' }

const resendRoute: InvitationRoute = { path: '/api/users/invite/resend', write: 'resending invitation' }

/**
 * Returns the typed error a refused response's code names.
 * @param refused - The code and message the response carries.
 * @param errors - The error class each known code maps to.
 * @returns The typed error, or undefined for any other code.
 */
function typedError(refused: Refused, errors: ErrorClasses = roleErrors): Error | undefined {
	const typed = errors.get(refused.code ?? '')
	return typed && new typed(refused.message)
}

/**
 * Returns the plain error naming a refused write's status, with the server's code when it sent one.
 * @param refused - The code and message the response carries.
 * @param failed - The name the error gives the write and the status the server answered.
 * @returns The error.
 */
function plainError(refused: Refused, failed: FailedWrite): Error {
	const error = new Error(`${failed.write} failed with status ${failed.status}`)
	return refused.code === undefined ? error : Object.assign(error, { code: refused.code })
}

/**
 * Returns the error a refused account write throws.
 * @param response - The refused HTTP response.
 * @param post - The write, with the message for an unreadable body and its name for any other status.
 * @returns The error to throw.
 */
async function writeError(response: Response, post: AccountPost): Promise<Error> {
	const refused = await refusalOf(response, post.fallback)
	const typed = typedError(refused)
	if (typed) {
		return typed
	}
	if (response.status === 422) {
		return new ValidationError(refused.message, refused)
	}
	return plainError(refused, { write: post.write, status: response.status })
}

/**
 * Posts an account write over REST and returns the body it answers.
 * @param post - The route, the fields it reads and the error each known status throws.
 * @returns The answered body, still to be parsed.
 */
async function postAccountWrite(post: AccountPost): Promise<unknown> {
	const response = await fetch(post.path, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(post.body),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	const known = post.errorsByStatus.get(response.status)
	if (known) {
		throw known()
	}
	if (!response.ok) {
		throw await writeError(response, post)
	}
	return response.json()
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
	return z.array(accountSchema).parse(await response.json())
}

/**
 * Creates an account over REST.
 * @param input - The email, name, and password of the new account.
 * @returns The created user.
 */
async function restCreateUser(input: NewUser): Promise<User> {
	const created = await postAccountWrite({
		path: '/api/users',
		body: input,
		write: 'creating user',
		fallback: __('invalid user details', DOMAIN),
		errorsByStatus: creationStatusErrors,
	})
	return accountSchema.parse(created)
}

/**
 * Posts an invitation write over REST and answers its delivery report.
 * @param route - The invitation route and the name its errors give the write.
 * @param body - The fields the route reads.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
async function postInvitation(route: InvitationRoute, body: object): Promise<Invitation> {
	const report = await postAccountWrite({
		...route,
		body,
		fallback: __('invalid invitation details', DOMAIN),
		errorsByStatus: invitationStatusErrors,
	})
	return invitationSchema.parse(report)
}

/**
 * Sends an invitation over REST, answering the same shape whatever the
 * address's state.
 * @param input - The email, name, and optional role of the invited account.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
async function restInvite(input: NewInvite): Promise<Invitation> {
	return postInvitation(inviteRoute, input)
}

/**
 * Sends a new invitation to a pending account over REST, ending the link sent before.
 * @param email - The address of the invited account.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
async function restResendInvite(email: string): Promise<Invitation> {
	return postInvitation(resendRoute, { email })
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
		const refused = await refusalOf(response, __('the account could not be updated', DOMAIN))
		const failed = { write: 'updating user', status: response.status }
		throw typedError(refused, disableErrors) ?? plainError(refused, failed)
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
	const refused = await refusalOf(response, __('the account could not be updated', DOMAIN))
	const failed = { write: 'updating role', status: response.status }
	throw typedError(refused, roleChangeErrors) ?? plainError(refused, failed)
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
 * state, and holding every transport to the invitation contract.
 * @param input - The email, name, and optional role of the invited account.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
export async function invite(input: NewInvite): Promise<Invitation> {
	return invitationSchema.parse(await resolveTransport('invite', restInvite)(input))
}

/**
 * Sends a new invitation to a pending account, ending the link sent before,
 * and holds every transport to the invitation contract.
 * @param email - The address of the invited account.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
export async function resendInvite(email: string): Promise<Invitation> {
	return invitationSchema.parse(await resolveTransport('resendInvite', restResendInvite)(email))
}
