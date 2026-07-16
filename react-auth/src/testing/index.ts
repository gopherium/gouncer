// SPDX-License-Identifier: Apache-2.0

import '@testing-library/jest-dom/vitest'
import type { QueryClient } from '@tanstack/react-query'
import { cleanup } from '@testing-library/react'
import { HttpResponse, http } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll } from 'vitest'

import type { User } from '../api'
import { sessionQueryKey } from '../session'

export { http, HttpResponse } from 'msw'

export const server = setupServer()

/**
 * Installs the msw server lifecycle and DOM cleanup as vitest hooks.
 */
export function installTestEnvironment() {
	beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
	afterEach(() => {
		cleanup()
		server.resetHandlers()
	})
	afterAll(() => server.close())
}

/**
 * defaultUser is the canned account tests sign in as.
 */
export const defaultUser: User = {
	id: '0198b2f0-0000-7000-8000-000000000001',
	email: 'grace@example.com',
	name: 'Grace Hopper',
}

/**
 * Seeds the cached session so components render signed in without a fetch.
 * @param client - The query client under test.
 * @param user - The signed-in user, or null for a signed-out state.
 */
export function seedSession(client: QueryClient, user: User | null = defaultUser) {
	client.setQueryData(sessionQueryKey, user)
}

/**
 * Answers the session endpoint with the given user.
 * @param user - The user the session belongs to.
 * @returns The msw handler.
 */
export function sessionOk(user: User = defaultUser) {
	return http.get('/api/auth/session', () => HttpResponse.json(user))
}

/**
 * Answers the session endpoint as signed out.
 * @returns The msw handler.
 */
export function sessionAnonymous() {
	return http.get('/api/auth/session', () =>
		HttpResponse.json({ error: 'no session' }, { status: 401 }),
	)
}

/**
 * Fails the session endpoint with a server error.
 * @returns The msw handler.
 */
export function sessionFailure() {
	return http.get('/api/auth/session', () =>
		HttpResponse.json({ error: 'internal error' }, { status: 500 }),
	)
}

/**
 * Accepts any login with the given user.
 * @param user - The user the login authenticates.
 * @returns The msw handler.
 */
export function loginOk(user: User = defaultUser) {
	return http.post('/api/auth/login', () => HttpResponse.json(user))
}

/**
 * Rejects logins as invalid credentials.
 * @returns The msw handler.
 */
export function loginInvalid() {
	return http.post('/api/auth/login', () =>
		HttpResponse.json({ error: 'invalid credentials' }, { status: 401 }),
	)
}

/**
 * Rejects logins as rate limited.
 * @returns The msw handler.
 */
export function loginRateLimited() {
	return http.post('/api/auth/login', () =>
		HttpResponse.json({ error: 'too many login attempts' }, { status: 429 }),
	)
}

/**
 * Fails logins with a server error.
 * @returns The msw handler.
 */
export function loginFailure() {
	return http.post('/api/auth/login', () =>
		HttpResponse.json({ error: 'internal error' }, { status: 500 }),
	)
}

/**
 * Accepts logouts.
 * @returns The msw handler.
 */
export function logoutOk() {
	return http.post('/api/auth/logout', () => new HttpResponse(null, { status: 204 }))
}

/**
 * Fails logouts with a server error.
 * @returns The msw handler.
 */
export function logoutFailure() {
	return http.post('/api/auth/logout', () =>
		HttpResponse.json({ error: 'internal error' }, { status: 500 }),
	)
}
