// SPDX-License-Identifier: Apache-2.0

import { Button, Stack, Text } from '@wordpress/ui'

import { useLogout, useSession } from '../session.js'

/**
 * Renders the signed-in user's identity with a logout control, or nothing
 * without an active session.
 * @param className - Extra class names for the surrounding stack.
 * @returns The account panel element, or null when signed out.
 */
export function AccountPanel({ className }: { className?: string }) {
	const user = useSession().data
	const signOut = useLogout()

	if (!user) {
		return null
	}
	return (
		<Stack direction="column" gap="sm" className={className}>
			<Text>{user.name}</Text>
			<Button
				variant="outline"
				disabled={signOut.isPending}
				onClick={() => signOut.mutate()}
			>
				Log out
			</Button>
			{signOut.isError ? (
				<Text role="alert">Logout failed, please try again.</Text>
			) : null}
		</Stack>
	)
}
