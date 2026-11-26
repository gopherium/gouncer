// SPDX-License-Identifier: Apache-2.0

import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import type { DefaultOptions, QueryCacheNotifyEvent } from '@tanstack/react-query'

import { UnauthorizedError } from './api.js'
import type { User } from './api.js'
import { dropOtherQueries, isSessionQuery, sessionQueryKey } from './session.js'

/**
 * Builds a query client that drops the session on an UnauthorizedError and every other read on an account change.
 * @param defaultOptions - Query and mutation defaults, mainly for tests.
 * @returns The configured query client.
 */
export function createAuthQueryClient(defaultOptions?: DefaultOptions): QueryClient {
	const queryClient: QueryClient = new QueryClient({
		defaultOptions,
		queryCache: new QueryCache({ onError: dropExpiredSession }),
		mutationCache: new MutationCache({ onError: dropExpiredSession }),
	})
	/** lastAccountId is the id of the last account the session answered, kept while it answers nobody. */
	let lastAccountId: string | undefined
	queryClient.getQueryCache().subscribe(dropOnAccountChange)

	/**
	 * Clears the cached session when the backend reports it gone.
	 * @param error - The failure reported by a query or mutation.
	 */
	function dropExpiredSession(error: unknown) {
		if (error instanceof UnauthorizedError) {
			queryClient.setQueryData(sessionQueryKey, null)
		}
	}

	/**
	 * Drops every other cached read when the session answers a different account than the last one.
	 * @param event - The change the query cache reports.
	 */
	function dropOnAccountChange(event: QueryCacheNotifyEvent) {
		if (event.type !== 'updated' || event.action.type !== 'success' || !isSessionQuery(event.query)) {
			return
		}
		const account = event.query.state.data as User | null
		if (account === null) {
			return
		}
		if (lastAccountId !== undefined && account.id !== lastAccountId) {
			dropOtherQueries(queryClient)
		}
		lastAccountId = account.id
	}

	return queryClient
}
