// SPDX-License-Identifier: Apache-2.0

import type { User as Account, NewUser } from './admin/index.js'
import type { User } from './api.js'

/**
 * AuthTransport carries every backend call of the package, so a consumer
 * can replace the REST transport with its own while keeping the screens,
 * hooks, and error contract. Implementations must throw the same error
 * classes the REST transport throws.
 */
export interface AuthTransport {
	fetchSession(signal?: AbortSignal): Promise<User | null>
	login(email: string, password: string): Promise<User>
	logout(): Promise<void>
	isSessionRevoked(signal?: AbortSignal): Promise<boolean>
	fetchUsers(signal?: AbortSignal): Promise<Account[]>
	createUser(input: NewUser): Promise<Account>
	setUserDisabled(id: string, disabled: boolean): Promise<void>
	setUserRank(id: string, rank: string): Promise<void>
}

let overrides: Partial<AuthTransport> = {}

/**
 * Replaces backend calls with the given implementations, keeping the REST
 * transport for every operation not named.
 * @param transport - The operations to override.
 */
export function configureAuthTransport(transport: Partial<AuthTransport>): void {
	overrides = { ...overrides, ...transport }
}

/**
 * Restores the REST transport for every operation.
 */
export function resetAuthTransport(): void {
	overrides = {}
}

/**
 * Returns the configured implementation of one operation, or the fallback.
 * @param name - The operation to resolve.
 * @param fallback - The REST implementation used when not overridden.
 * @returns The active implementation.
 */
export function resolveTransport<K extends keyof AuthTransport>(
	name: K,
	fallback: AuthTransport[K],
): AuthTransport[K] {
	return (overrides[name] as AuthTransport[K] | undefined) ?? fallback
}
