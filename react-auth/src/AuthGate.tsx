// SPDX-License-Identifier: Apache-2.0

import { useQueryClient } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import type { User } from './api'
import { sessionQueryKey, useSession } from './session'

/**
 * Guards its children behind a login session, rendering the given login
 * screen until the user is authenticated.
 * @param children - The application to reveal once a session is active.
 * @param loginScreen - Renders the login UI, wired to the given login handler.
 * @param loading - Shown while the session resolves.
 * @param error - Shown when the session cannot be loaded.
 * @returns The children, the login screen, or a status element.
 */
export function AuthGate({
	children,
	loginScreen,
	loading = <p role="status">Loading…</p>,
	error = <p role="alert">Something went wrong.</p>,
}: {
	children: ReactNode
	loginScreen: (onLogin: (user: User) => void | Promise<void>) => ReactNode
	loading?: ReactNode
	error?: ReactNode
}) {
	const queryClient = useQueryClient()
	const session = useSession()

	if (session.data === undefined) {
		if (session.isError) {
			return <>{error}</>
		}
		return <>{loading}</>
	}
	if (session.data === null) {
		return (
			<>
				{loginScreen(async (user) => {
					await queryClient.cancelQueries({ queryKey: sessionQueryKey })
					queryClient.setQueryData(sessionQueryKey, user)
				})}
			</>
		)
	}
	return <>{children}</>
}
