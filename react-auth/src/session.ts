// SPDX-License-Identifier: Apache-2.0

import { hashKey, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { Query, QueryClient } from '@tanstack/react-query'

import { fetchSession, logout } from './api.js'
import type { User } from './api.js'

/**
 * sessionQueryKey is the react-query key the login session is cached under.
 */
export const sessionQueryKey = ['session'] as const

/** sessionQueryHash is the hash the session is filed under. */
const sessionQueryHash = hashKey(sessionQueryKey)

/**
 * Reports whether a cached read is the session.
 * @param query - The cached read.
 * @returns Whether the read is filed under the session key.
 */
export function isSessionQuery(query: Query): boolean {
	return query.queryHash === sessionQueryHash
}

/**
 * Drops every cached read but the session.
 * @param client - The query client holding the session.
 */
export function dropOtherQueries(client: QueryClient): void {
	client.removeQueries({ predicate: (query) => !isSessionQuery(query) })
}

/**
 * Loads the current session as a react-query result.
 * @returns The session query, whose data is the user or null.
 */
export function useSession() {
	return useQuery({
		queryKey: sessionQueryKey,
		queryFn: ({ signal }) => fetchSession(signal),
	})
}

/**
 * Ends the current session and drops every cached read of the signed-out account.
 * @returns The logout mutation.
 */
export function useLogout() {
	const queryClient = useQueryClient()
	return useMutation({
		mutationFn: logout,
		onSuccess: async () => {
			await queryClient.cancelQueries()
			queryClient.setQueryData(sessionQueryKey, null)
			dropOtherQueries(queryClient)
		},
	})
}

/**
 * Installs a freshly authenticated user as the cached session and drops what the last account left cached.
 * @param client - The query client holding the session.
 * @param user - The authenticated user to install.
 */
export async function adoptSession(client: QueryClient, user: User): Promise<void> {
	await client.cancelQueries()
	client.setQueryData(sessionQueryKey, user)
	dropOtherQueries(client)
}
