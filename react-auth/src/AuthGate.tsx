// SPDX-License-Identifier: Apache-2.0

import { useQueryClient } from '@tanstack/react-query'
import { __ } from '@wordpress/i18n'
import { Fragment } from 'react'
import type { ReactNode } from 'react'

import type { User } from './api.js'
import { DOMAIN } from './domain.js'
import { adoptSession, useSession } from './session.js'

/**
 * Guards its children behind a login session and mounts them again for each signed-in account.
 * @param children - The application to reveal once a session is active.
 * @param loginScreen - Renders the login UI, wired to the given login handler.
 * @param loading - Shown while the session resolves.
 * @param error - Shown when the session cannot be loaded.
 * @returns The children under the account's key, the login screen, or a status element.
 */
export function AuthGate({
	children,
	loginScreen,
	loading = <p role="status">{__('Loading…', DOMAIN)}</p>,
	error = <p role="alert">{__('Something went wrong.', DOMAIN)}</p>,
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
				{loginScreen((user) => adoptSession(queryClient, user))}
			</>
		)
	}
	return <Fragment key={session.data.id}>{children}</Fragment>
}
