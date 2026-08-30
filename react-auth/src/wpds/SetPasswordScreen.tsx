// SPDX-License-Identifier: Apache-2.0

import { useMutation } from '@tanstack/react-query'
import { __ } from '@wordpress/i18n'
import { Button, Card, InputControl, Stack, Text } from '@wordpress/ui'
import { useState } from 'react'

import {
	InvalidTokenError,
	RateLimitedError,
	ValidationError,
	acceptInvite,
} from '../api.js'
import type { User } from '../api.js'
import { DOMAIN } from '../domain.js'

/**
 * Maps the screen's failures to the message shown to the user.
 * @param attempted - The error the activation raised, if any.
 * @param handedOff - The error handing the user onward raised, if any.
 * @returns The message to display, or null when nothing failed.
 */
function setPasswordErrorMessage(attempted: Error | null, handedOff: Error | null): string | null {
	if (attempted instanceof InvalidTokenError) {
		return __('This link is no longer valid. Ask the person who invited you for a new one.', DOMAIN)
	}
	if (attempted instanceof ValidationError) {
		return attempted.message
	}
	if (attempted instanceof RateLimitedError) {
		return __('Too many attempts. Please wait a minute and try again.', DOMAIN)
	}
	if (attempted) {
		return __('The password could not be set, please try again.', DOMAIN)
	}
	if (handedOff) {
		return __('Something went wrong.', DOMAIN)
	}
	return null
}

/**
 * Renders the invitation acceptance form and reports the signed-in user
 * upward.
 * @param brand - The product name shown above the form.
 * @param token - The invitation link's secret.
 * @param onAccepted - Called with the user after a successful activation.
 * @returns The set password screen element.
 */
export function SetPasswordScreen({
	brand,
	token,
	onAccepted,
}: {
	brand: string
	token: string
	onAccepted: (user: User) => void | Promise<void>
}) {
	const [password, setPassword] = useState('')
	const finish = useMutation({
		mutationFn: async (user: User) => {
			await onAccepted(user)
		},
	})
	const attempt = useMutation({
		mutationFn: () => acceptInvite(token, password),
		onSuccess: (user) => finish.mutate(user),
	})
	const failure = setPasswordErrorMessage(attempt.error, finish.error)

	return (
		<div className="gopherium-login">
			<Card.Root className="gopherium-login__card">
				<Card.Content>
					<form
						onSubmit={(event) => {
							event.preventDefault()
							attempt.mutate()
						}}
					>
						<Stack direction="column" gap="lg">
							<Text variant="heading-lg" render={<h1 />}>
								{brand}
							</Text>
							<InputControl
								label={__('Password', DOMAIN)}
								type="password"
								autoComplete="new-password"
								value={password}
								onChange={(event) => setPassword(event.target.value)}
							/>
							<Button
								type="submit"
								disabled={
									password === '' ||
									attempt.isPending ||
									attempt.isSuccess ||
									finish.isPending
								}
								loading={attempt.isPending || finish.isPending}
							>
								{__('Set password', DOMAIN)}
							</Button>
							{failure ? <Text role="alert">{failure}</Text> : null}
						</Stack>
					</form>
				</Card.Content>
			</Card.Root>
		</div>
	)
}
