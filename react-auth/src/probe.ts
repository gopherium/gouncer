// SPDX-License-Identifier: Apache-2.0

import { resolveTransport } from './transport.js'

/**
 * Probes the session over REST.
 * @param signal - Aborts the in-flight request.
 * @returns True when the session is revoked.
 */
async function restIsSessionRevoked(signal?: AbortSignal): Promise<boolean> {
	const response = await fetch('/api/auth/session', {
		credentials: 'include',
		signal,
	})
	return response.status === 401
}

/**
 * Reports whether the backend considers the session gone, distinguishing
 * a revoked login from a transient outage after a stream failure.
 * @param signal - Aborts the in-flight request.
 * @returns True when the session is revoked, false when the backend still
 * accepts it.
 */
export async function isSessionRevoked(signal?: AbortSignal): Promise<boolean> {
	return resolveTransport('isSessionRevoked', restIsSessionRevoked)(signal)
}
