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
 * Maps an activation failure to the message shown to the user.
 * @param error - The error thrown by the activation attempt.
 * @returns The message to display.
 */
function setPasswordErrorMessage(error: Error): string {
	if (error instanceof InvalidTokenError) {
		return __('This link is no longer valid. Ask the person who invited you for a new one.', DOMAIN)
	}
	if (error instanceof ValidationError) {
		return error.message
	}
	if (error instanceof RateLimitedError) {
		return __('Too many attempts. Please wait a minute and try again.', DOMAIN)
	}
	return __('The password could not be set, please try again.', DOMAIN)
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
	const [activated, setActivated] = useState<User | null>(null)
	const [handoffFailed, setHandoffFailed] = useState(false)
	const finish = useMutation({
		mutationFn: async (user: User) => {
			await onAccepted(user)
		},
		onError: () => setHandoffFailed(true),
		onSuccess: () => setHandoffFailed(false),
	})
	const attempt = useMutation({
		mutationFn: () => acceptInvite(token, password),
		onSuccess: (user) => {
			setActivated(user)
			finish.mutate(user)
		},
	})

	if (activated !== null && handoffFailed) {
		return (
			<div className="gopherium-login">
				<Card.Root className="gopherium-login__card">
					<Card.Content>
						<Stack direction="column" gap="lg">
							<Text variant="heading-lg" render={<h1 />}>
								{brand}
							</Text>
							<Text role="alert">{__('Something went wrong.', DOMAIN)}</Text>
							<Button
								type="button"
								disabled={finish.isPending}
								loading={finish.isPending}
								onClick={() => finish.mutate(activated)}
							>
								{__('Try again', DOMAIN)}
							</Button>
						</Stack>
					</Card.Content>
				</Card.Root>
			</div>
		)
	}
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
							{attempt.isError ? (
								<Text role="alert">{setPasswordErrorMessage(attempt.error)}</Text>
							) : null}
						</Stack>
					</form>
				</Card.Content>
			</Card.Root>
		</div>
	)
}
