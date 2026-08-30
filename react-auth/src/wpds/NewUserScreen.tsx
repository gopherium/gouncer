// SPDX-License-Identifier: Apache-2.0

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { __, _x } from '@wordpress/i18n'
import { Button, InputControl, Stack, Text } from '@wordpress/ui'
import { useState } from 'react'

import { ValidationError, invite, usersQueryKey } from '../admin/index.js'
import { DOMAIN } from '../domain.js'
import { Outcome } from './Outcome.js'

/**
 * Maps an invitation failure to the message shown under the form,
 * surfacing the backend's explanation for rejected input.
 * @param error - The error thrown by the invite mutation.
 * @returns The human-readable failure message.
 */
function inviteErrorMessage(error: Error): string {
	if (error instanceof ValidationError) {
		return error.message
	}
	return __('The invitation could not be sent.', DOMAIN)
}

/**
 * Renders the new user form, sending an invitation and reporting success
 * upward, or showing the activation link when no mail server delivered it.
 * @param onCreated - Called after the invitation is handled.
 * @returns The new user screen element.
 */
export function NewUserScreen({
	onCreated,
}: {
	onCreated?: () => void | Promise<void>
}) {
	const queryClient = useQueryClient()
	const [email, setEmail] = useState('')
	const [name, setName] = useState('')
	const [activationLink, setActivationLink] = useState<string | null>(null)
	const create = useMutation({
		mutationFn: () => invite({ email: email.trim(), name: name.trim() }),
		onSuccess: async (invitation) => {
			await queryClient.invalidateQueries({ queryKey: usersQueryKey })
			if (invitation.delivered) {
				await onCreated?.()
				return
			}
			setActivationLink(invitation.activation_link)
		},
	})

	if (activationLink !== null) {
		return (
			<Stack direction="column" gap="lg">
				<Text variant="heading-lg" render={<h1 />}>
					{_x('New user', 'page heading', DOMAIN)}
				</Text>
				<Outcome>
					<Text>
						{__('No mail server is configured. Deliver the activation link by hand.', DOMAIN)}
					</Text>
				</Outcome>
				<InputControl
					label={_x('Activation link', 'field label', DOMAIN)}
					readOnly
					value={activationLink}
				/>
				<Button type="button" onClick={() => onCreated?.()}>
					{__('Done', DOMAIN)}
				</Button>
			</Stack>
		)
	}
	return (
		<Stack direction="column" gap="lg">
			<Text variant="heading-lg" render={<h1 />}>
				{_x('New user', 'page heading', DOMAIN)}
			</Text>
			<form
				onSubmit={(event) => {
					event.preventDefault()
					create.mutate()
				}}
			>
				<Stack direction="column" gap="md">
					<InputControl
						label={_x('Email', 'field label', DOMAIN)}
						type="email"
						autoComplete="off"
						value={email}
						onChange={(event) => setEmail(event.target.value)}
					/>
					<InputControl
						label={_x('Name', 'field label', DOMAIN)}
						autoComplete="off"
						value={name}
						onChange={(event) => setName(event.target.value)}
					/>
					<Button
						type="submit"
						disabled={email.trim() === '' || name.trim() === '' || create.isPending}
					>
						{__('Send invitation', DOMAIN)}
					</Button>
					{create.isError ? (
						<Text role="alert">{inviteErrorMessage(create.error)}</Text>
					) : null}
				</Stack>
			</form>
		</Stack>
	)
}
