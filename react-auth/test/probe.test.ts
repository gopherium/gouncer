// SPDX-License-Identifier: Apache-2.0

import { expect, test } from 'vitest'

import { isSessionRevoked } from '../src/probe'
import { HttpResponse, http, server, sessionAnonymous, sessionOk } from '../src/testing'

test('reports a revoked session on 401', async () => {
	server.use(sessionAnonymous())

	await expect(isSessionRevoked()).resolves.toBe(true)
})

test('reports a live session as not revoked', async () => {
	server.use(sessionOk())

	await expect(isSessionRevoked()).resolves.toBe(false)
})

test('treats a server failure as not revoked', async () => {
	server.use(
		http.get('/api/auth/session', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)

	await expect(isSessionRevoked()).resolves.toBe(false)
})

test('rejects when the endpoint is unreachable', async () => {
	server.use(http.get('/api/auth/session', () => HttpResponse.error()))

	await expect(isSessionRevoked()).rejects.toThrow()
})
