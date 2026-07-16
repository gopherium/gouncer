// SPDX-License-Identifier: Apache-2.0

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Badge, Button, Stack, Text, VisuallyHidden } from '@wordpress/ui'
import type { ReactElement } from 'react'

import { fetchUsers, setUserDisabled, usersQueryKey } from '../admin'
import type { User } from '../admin'
import { useSession } from '../session'

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
					{user.disabled ? 'Disabled' : 'Active'}
				</Badge>
			</td>
			<td>
				{isSelf ? null : (
					<Stack direction="column" gap="xs">
						<Button
							variant="outline"
							aria-label={`${user.disabled ? 'Enable' : 'Disable'} ${user.name}`}
							disabled={toggle.isPending}
							onClick={() => toggle.mutate()}
						>
							{user.disabled ? 'Enable' : 'Disable'}
						</Button>
						{toggle.isError ? <Text role="alert">Update failed.</Text> : null}
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
		return <Text role="status">Loading users…</Text>
	}
	if (users.isError) {
		return <Text role="alert">Users could not be loaded.</Text>
	}
	return (
		<Stack direction="column" gap="lg">
			<Stack direction="row" align="center" gap="md">
				<Text variant="heading-lg" render={<h1 />}>
					Users
				</Text>
				{newUserRender ? <Button render={newUserRender}>New user</Button> : null}
			</Stack>
			<table className="gopherium-users">
				<thead>
					<tr>
						<th scope="col">Name</th>
						<th scope="col">Email</th>
						<th scope="col">Status</th>
						<th scope="col">
							<VisuallyHidden>Actions</VisuallyHidden>
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
