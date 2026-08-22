// SPDX-License-Identifier: Apache-2.0

import { z } from 'zod'

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
