// SPDX-License-Identifier: Apache-2.0

import { hashKey, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'

import { fetchSession, logout } from './api.js'
import type { User } from './api.js'

/**
 * sessionQueryKey is the react-query key the login session is cached under.
 */
export const sessionQueryKey = ['session'] as const

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
 * Ends the current session and drops all cached data belonging to the
 * signed-out user.
 * @returns The logout mutation.
 */
export function useLogout() {
	const queryClient = useQueryClient()
	return useMutation({
		mutationFn: logout,
		onSuccess: async () => {
			await queryClient.cancelQueries()
			queryClient.setQueryData(sessionQueryKey, null)
			queryClient.removeQueries({
				predicate: (query) => query.queryHash !== hashKey(sessionQueryKey),
			})
		},
	})
}

/**
 * Installs a freshly authenticated user as the cached session, cancelling
 * any in-flight session fetch so it cannot clobber the seed.
 * @param client - The query client holding the session.
 * @param user - The authenticated user to install.
 */
export async function adoptSession(client: QueryClient, user: User): Promise<void> {
	await client.cancelQueries({ queryKey: sessionQueryKey })
	client.setQueryData(sessionQueryKey, user)
}
