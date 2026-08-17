// SPDX-License-Identifier: Apache-2.0

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { __, _x, sprintf } from '@wordpress/i18n'
import { Badge, Button, Stack, Text, VisuallyHidden } from '@wordpress/ui'
import type { ReactElement } from 'react'

import { fetchUsers, setUserDisabled, usersQueryKey } from '../admin/index.js'
import type { User } from '../admin/index.js'
import { DOMAIN } from '../domain.js'
import { useSession } from '../session.js'

/**
 * Renders one user row with status and a disable toggle, hidden for the
 * signed-in account.
 * @param user - The user the row describes.
 * @param isSelf - Whether the row is the signed-in account.
 * @returns The table row element.
 */
function UserRow({ user, isSelf }: { user: User; isSelf: boolean }) {
	const queryClient = useQueryClient()
	const toggle = useMutation({
		mutationFn: () => setUserDisabled(user.id, !user.disabled),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: usersQueryKey }),
	})

	return (
		<tr>
			<td>{user.name}</td>
			<td>{user.email}</td>
			<td>
				<Badge intent={user.disabled ? 'draft' : 'stable'}>
					{user.disabled ? __('Disabled', DOMAIN) : __('Active', DOMAIN)}
				</Badge>
			</td>
			<td>
				{isSelf ? null : (
					<Stack direction="column" gap="xs">
						<Button
							variant="outline"
							aria-label={
								user.disabled
									? sprintf(__('Enable %s', DOMAIN), user.name)
									: sprintf(__('Disable %s', DOMAIN), user.name)
							}
							disabled={toggle.isPending}
							onClick={() => toggle.mutate()}
						>
							{user.disabled ? __('Enable', DOMAIN) : __('Disable', DOMAIN)}
						</Button>
						{toggle.isError ? <Text role="alert">{__('Update failed.', DOMAIN)}</Text> : null}
					</Stack>
				)}
			</td>
		</tr>
	)
}

/**
 * Renders the user administration screen: the account list with status and
 * disable controls, plus an optional control to create a new user.
 * @param newUserRender - The navigation element the New user button renders
 * as, typically the app router's link to its create form.
 * @returns The users screen, or a loading or error message.
 */
export function UsersScreen({
	newUserRender,
}: {
	newUserRender?: ReactElement<Record<string, unknown>>
}) {
	const currentUserId = useSession().data?.id
	const users = useQuery({
		queryKey: usersQueryKey,
		queryFn: ({ signal }) => fetchUsers(signal),
	})

	if (users.isPending) {
		return <Text role="status">{__('Loading users…', DOMAIN)}</Text>
	}
	if (users.isError) {
		return <Text role="alert">{__('Users could not be loaded.', DOMAIN)}</Text>
	}
	return (
		<Stack direction="column" gap="lg">
			<Stack direction="row" align="center" gap="md">
				<Text variant="heading-lg" render={<h1 />}>
					{_x('Users', 'page heading', DOMAIN)}
				</Text>
				{newUserRender ? (
					<Button render={newUserRender}>{_x('New user', 'button', DOMAIN)}</Button>
				) : null}
			</Stack>
			<table className="gopherium-users">
				<thead>
					<tr>
						<th scope="col">{_x('Name', 'column', DOMAIN)}</th>
						<th scope="col">{_x('Email', 'column', DOMAIN)}</th>
						<th scope="col">{_x('Status', 'column', DOMAIN)}</th>
						<th scope="col">
							<VisuallyHidden>{__('Actions', DOMAIN)}</VisuallyHidden>
						</th>
					</tr>
				</thead>
				<tbody>
					{users.data.map((user) => (
						<UserRow
							key={user.id}
							user={user}
							isSelf={user.id === currentUserId}
						/>
					))}
				</tbody>
			</table>
		</Stack>
	)
}
