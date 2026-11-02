// SPDX-License-Identifier: Apache-2.0

import { useMutation } from '@tanstack/react-query'
import { __, _x } from '@wordpress/i18n'
import { Button, Card, InputControl, Stack, Text } from '@wordpress/ui'
import { useState } from 'react'

import { RateLimitedError, requestPasswordReset } from '../api.js'
import { DOMAIN } from '../domain.js'
import { AuthPage } from './AuthPage.js'
import type { AuthLink } from './AuthPage.js'
import { Outcome } from './Outcome.js'

/**
 * Maps a reset request failure to the message shown to the user.
 * @param error - The error thrown by the request attempt.
 * @returns The message to display.
 */
function requestErrorMessage(error: Error): string {
	if (error instanceof RateLimitedError) {
		return __('Too many attempts. Please wait a minute and try again.', DOMAIN)
	}
	return __('The request failed, please try again.', DOMAIN)
}

/**
 * Renders the forgotten password form, answering the same sentence
 * whatever the address.
 * @param brand - The product name shown above the form.
 * @param onBack - Called when the user heads back to the login.
 * @param links - The links under the form, none when left out.
 * @returns The request reset screen element.
 */
export function RequestResetScreen({
	brand,
	onBack,
	links,
}: {
	brand: string
	onBack?: () => void
	links?: readonly AuthLink[]
}) {
	const [email, setEmail] = useState('')
	const attempt = useMutation({
		mutationFn: () => requestPasswordReset(email.trim()),
	})

	return (
		<AuthPage links={links}>
			<Card.Root className="gopherium-login__card">
				<Card.Content>
					<Stack direction="column" gap="lg">
						<Text variant="heading-lg" render={<h1 />}>
							{brand}
						</Text>
						{attempt.isSuccess ? (
							<Outcome>
								<Text>
									{__('If that address has an account, a reset link is on its way.', DOMAIN)}
								</Text>
							</Outcome>
						) : (
							<form
								onSubmit={(event) => {
									event.preventDefault()
									attempt.mutate()
								}}
							>
								<Stack direction="column" gap="md">
									<InputControl
										label={_x('Email', 'field label', DOMAIN)}
										type="email"
										autoComplete="email"
										value={email}
										onChange={(event) => setEmail(event.target.value)}
									/>
									<Button
										type="submit"
										disabled={email.trim() === '' || attempt.isPending}
										loading={attempt.isPending}
									>
										{__('Send reset link', DOMAIN)}
									</Button>
									{attempt.isError ? (
										<Text role="alert">{requestErrorMessage(attempt.error)}</Text>
									) : null}
								</Stack>
							</form>
						)}
						{onBack ? (
							<Button type="button" variant="outline" onClick={onBack}>
								{__('Back to login', DOMAIN)}
							</Button>
						) : null}
					</Stack>
				</Card.Content>
			</Card.Root>
		</AuthPage>
	)
}
