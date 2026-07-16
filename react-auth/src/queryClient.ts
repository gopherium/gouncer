// SPDX-License-Identifier: Apache-2.0

import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import type { DefaultOptions } from '@tanstack/react-query'

import { UnauthorizedError } from './api'
import { sessionQueryKey } from './session'

/**
 * Builds a query client that drops the cached session whenever a query
 * or mutation fails with an UnauthorizedError so the login screen takes over.
 * @param defaultOptions - Query and mutation defaults, mainly for tests.
 * @returns The configured query client.
 */
export function createAuthQueryClient(defaultOptions?: DefaultOptions): QueryClient {
	const queryClient: QueryClient = new QueryClient({
		defaultOptions,
		queryCache: new QueryCache({ onError: dropExpiredSession }),
		mutationCache: new MutationCache({ onError: dropExpiredSession }),
	})

	/**
	 * Clears the cached session when the backend reports it gone.
	 * @param error - The failure reported by a query or mutation.
	 */
	function dropExpiredSession(error: unknown) {
		if (error instanceof UnauthorizedError) {
			queryClient.setQueryData(sessionQueryKey, null)
		}
	}

	return queryClient
}
