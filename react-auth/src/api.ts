// SPDX-License-Identifier: Apache-2.0

import { __ } from '@wordpress/i18n'
import { z } from 'zod'

import { DOMAIN } from './domain.js'
import { resolveTransport } from './transport.js'

const userSchema = z.object({
	id: z.string(),
	email: z.string(),
	name: z.string(),
	role: z.string().default(''),
})

export type User = z.infer<typeof userSchema>

/**
 * InvalidCredentialsError is thrown when the backend rejects a login.
 */
export class InvalidCredentialsError extends Error {}

/**
 * UnauthorizedError is thrown when the backend rejects a request because
 * the session is missing, expired, or revoked.
 */
export class UnauthorizedError extends Error {}

/**
 * RateLimitedError is thrown when the backend rejects a login for too many
 * attempts.
 */
export class RateLimitedError extends Error {}

/**
 * InvalidTokenError is thrown when an invitation or reset link is spent,
 * expired, or unknown.
 */
export class InvalidTokenError extends Error {}

/**
 * ValidationError is thrown when the backend rejects submitted details;
 * its message is the backend's human-readable explanation.
 */
export class ValidationError extends Error {}

const refusalSchema = z.object({ error: z.string(), code: z.string().optional() })

/**
 * Reads the backend's refusal from a failed response body.
 * @param response - The failed HTTP response.
 * @param fallback - The message to use when the body is unreadable.
 * @returns The refusal message and optional code.
 */
async function refusalOf(
	response: Response,
	fallback: string,
): Promise<{ message: string; code?: string }> {
	try {
		const held = refusalSchema.parse(await response.json())
		return { message: held.error, code: held.code }
	} catch {
		return { message: fallback }
	}
}

/**
 * Maps a token route refusal onto the error class contract.
 * @param response - The failed HTTP response.
 * @returns The error to throw.
 */
async function tokenRefusal(response: Response): Promise<Error> {
	if (response.status === 429) {
		return new RateLimitedError('too many attempts')
	}
	if (response.status === 422) {
		const refusal = await refusalOf(response, __('invalid password details', DOMAIN))
		if (refusal.code === 'token_invalid') {
			return new InvalidTokenError('the link is no longer valid')
		}
		return new ValidationError(refusal.message)
	}
	return new Error(`the request failed with status ${response.status}`)
}

/**
 * Loads the session over REST.
 * @param signal - Aborts the in-flight request.
 * @returns The current user, or null when unauthenticated.
 */
async function restFetchSession(signal?: AbortSignal): Promise<User | null> {
	const response = await fetch('/api/auth/session', { signal })
	if (response.status === 401) {
		return null
	}
	if (!response.ok) {
		throw new Error(`loading session failed with status ${response.status}`)
	}
	return userSchema.parse(await response.json())
}

/**
 * Logs in over REST.
 * @param email - The account email address.
 * @param password - The account password.
 * @returns The authenticated user.
 */
async function restLogin(email: string, password: string): Promise<User> {
	const response = await fetch('/api/auth/login', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email, password }),
	})
	if (response.status === 401) {
		throw new InvalidCredentialsError('invalid credentials')
	}
	if (response.status === 429) {
		throw new RateLimitedError('too many login attempts')
	}
	if (!response.ok) {
		throw new Error(`login failed with status ${response.status}`)
	}
	return userSchema.parse(await response.json())
}

/**
 * Accepts an invitation over REST, setting the password and starting the
 * session.
 * @param token - The invitation link's secret.
 * @param password - The password the person chose.
 * @returns The signed-in user.
 */
async function restAcceptInvite(token: string, password: string): Promise<User> {
	const response = await fetch('/api/auth/activate', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ token, password }),
	})
	if (!response.ok) {
		throw await tokenRefusal(response)
	}
	return userSchema.parse(await response.json())
}

/**
 * Asks for a password reset link over REST.
 * @param email - The address the link is mailed to.
 */
async function restRequestPasswordReset(email: string): Promise<void> {
	const response = await fetch('/api/auth/password-reset', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email }),
	})
	if (response.status === 429) {
		throw new RateLimitedError('too many attempts')
	}
	if (!response.ok) {
		throw new Error(`the request failed with status ${response.status}`)
	}
}

/**
 * Replaces the password over REST using a reset link's secret.
 * @param token - The reset link's secret.
 * @param password - The password the person chose.
 */
async function restResetPassword(token: string, password: string): Promise<void> {
	const response = await fetch('/api/auth/password-reset/confirm', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ token, password }),
	})
	if (!response.ok) {
		throw await tokenRefusal(response)
	}
}

/**
 * Ends the session over REST.
 */
async function restLogout(): Promise<void> {
	const response = await fetch('/api/auth/logout', { method: 'POST' })
	if (!response.ok) {
		throw new Error(`logout failed with status ${response.status}`)
	}
}

/**
 * Returns the logged-in user, or null when no session is active.
 * @param signal - Aborts the in-flight request.
 * @returns The current user, or null when unauthenticated.
 */
export async function fetchSession(signal?: AbortSignal): Promise<User | null> {
	return resolveTransport('fetchSession', restFetchSession)(signal)
}

/**
 * Logs in with the given credentials and returns the user.
 * @param email - The account email address.
 * @param password - The account password.
 * @returns The authenticated user.
 */
export async function login(email: string, password: string): Promise<User> {
	return resolveTransport('login', restLogin)(email, password)
}

/**
 * Ends the current session.
 */
export async function logout(): Promise<void> {
	await resolveTransport('logout', restLogout)()
}

/**
 * Accepts an invitation, setting the password and starting the session.
 * @param token - The invitation link's secret.
 * @param password - The password the person chose.
 * @returns The signed-in user.
 */
export async function acceptInvite(token: string, password: string): Promise<User> {
	return resolveTransport('acceptInvite', restAcceptInvite)(token, password)
}

/**
 * Asks for a password reset link, answering nothing about the address.
 * @param email - The address the link is mailed to.
 */
export async function requestPasswordReset(email: string): Promise<void> {
	await resolveTransport('requestPasswordReset', restRequestPasswordReset)(email)
}

/**
 * Replaces the password using a reset link's secret.
 * @param token - The reset link's secret.
 * @param password - The password the person chose.
 */
export async function resetPassword(token: string, password: string): Promise<void> {
	await resolveTransport('resetPassword', restResetPassword)(token, password)
}
