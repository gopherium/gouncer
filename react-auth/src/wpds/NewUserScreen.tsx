// SPDX-License-Identifier: Apache-2.0

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { __, _x } from '@wordpress/i18n'
import { Button, InputControl, Stack, Text } from '@wordpress/ui'
import { useState } from 'react'

import { ValidationError, invite, usersQueryKey } from '../admin/index.js'
import { DOMAIN } from '../domain.js'
import { Outcome } from './Outcome.js'

/**
 * Maps the screen's failures to the message shown beside the form,
 * surfacing the backend's explanation for rejected input.
 * @param invited - The error the invitation raised, if any.
 * @param handedOff - The error handing the invitation onward raised, if any.
 * @returns The message to display, or null when nothing failed.
 */
function failureMessage(invited: Error | null, handedOff: Error | null): string | null {
	if (invited instanceof ValidationError) {
		return invited.message
	}
	if (invited) {
		return __('The invitation could not be sent.', DOMAIN)
	}
	if (handedOff) {
		return __('Something went wrong.', DOMAIN)
	}
	return null
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
	const finish = useMutation({
		mutationFn: async () => {
			await onCreated?.()
		},
	})
	const create = useMutation({
		mutationFn: () => invite({ email: email.trim(), name: name.trim() }),
		onSuccess: async (invitation) => {
			await queryClient.invalidateQueries({ queryKey: usersQueryKey })
			if (invitation.delivered) {
				finish.mutate()
				return
			}
			setActivationLink(invitation.activation_link)
		},
	})
	const failure = failureMessage(create.error, finish.error)

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
				<Button
					type="button"
					disabled={finish.isPending}
					loading={finish.isPending}
					onClick={() => finish.mutate()}
				>
					{__('Done', DOMAIN)}
				</Button>
				{failure ? <Text role="alert">{failure}</Text> : null}
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
					{failure ? <Text role="alert">{failure}</Text> : null}
				</Stack>
			</form>
		</Stack>
	)
}
