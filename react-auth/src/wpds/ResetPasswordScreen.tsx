// SPDX-License-Identifier: Apache-2.0

import { useMutation } from '@tanstack/react-query'
import { __ } from '@wordpress/i18n'
import { Button, Card, InputControl, Stack, Text } from '@wordpress/ui'
import { useState } from 'react'

import {
	InvalidTokenError,
	RateLimitedError,
	ValidationError,
	resetPassword,
} from '../api.js'
import { DOMAIN } from '../domain.js'
import { Outcome } from './Outcome.js'

/**
 * Maps a reset failure to the message shown to the user.
 * @param error - The error thrown by the reset attempt.
 * @returns The message to display.
 */
function resetErrorMessage(error: Error): string {
	if (error instanceof InvalidTokenError) {
		return __('This link is no longer valid. Ask for a new one.', DOMAIN)
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
 * Renders the password reset form, replacing it with the outcome once the
 * password is set.
 * @param brand - The product name shown above the form.
 * @param token - The reset link's secret.
 * @param onDone - Called when the user heads to the login afterward.
 * @returns The reset password screen element.
 */
export function ResetPasswordScreen({
	brand,
	token,
	onDone,
}: {
	brand: string
	token: string
	onDone?: () => void
}) {
	const [password, setPassword] = useState('')
	const attempt = useMutation({
		mutationFn: () => resetPassword(token, password),
	})

	return (
		<div className="gopherium-login">
			<Card.Root className="gopherium-login__card">
				<Card.Content>
					<Stack direction="column" gap="lg">
						<Text variant="heading-lg" render={<h1 />}>
							{brand}
						</Text>
						{attempt.isSuccess ? (
							<Stack direction="column" gap="md">
								<Outcome>
									<Text>{__('Your password is set. Sign in with it.', DOMAIN)}</Text>
								</Outcome>
								{onDone ? (
									<Button type="button" onClick={onDone}>
										{__('Go to login', DOMAIN)}
									</Button>
								) : null}
							</Stack>
						) : (
							<form
								onSubmit={(event) => {
									event.preventDefault()
									attempt.mutate()
								}}
							>
								<Stack direction="column" gap="md">
									<InputControl
										label={__('Password', DOMAIN)}
										type="password"
										autoComplete="new-password"
										value={password}
										onChange={(event) => setPassword(event.target.value)}
									/>
									<Button
										type="submit"
										disabled={password === '' || attempt.isPending}
										loading={attempt.isPending}
									>
										{__('Set password', DOMAIN)}
									</Button>
									{attempt.isError ? (
										<Text role="alert">{resetErrorMessage(attempt.error)}</Text>
									) : null}
								</Stack>
							</form>
						)}
					</Stack>
				</Card.Content>
			</Card.Root>
		</div>
	)
}
