// SPDX-License-Identifier: Apache-2.0

import { useMutation } from '@tanstack/react-query'
import { Button, Card, InputControl, Stack, Text } from '@wordpress/ui'
import { useState } from 'react'

import { InvalidCredentialsError, RateLimitedError, login } from '../api'
import type { User } from '../api'

/**
 * Maps a login attempt error to the message shown to the user.
 * @param error - The error thrown by the login attempt.
 * @returns The message to display.
 */
function loginErrorMessage(error: unknown): string {
	if (error instanceof InvalidCredentialsError) {
		return 'Invalid email or password.'
	}
	if (error instanceof RateLimitedError) {
		return 'Too many attempts. Please wait a minute and try again.'
	}
	return 'Login failed, please try again.'
}

/**
 * Renders the login form and reports the authenticated user upward.
 * @param brand - The product name shown above the form.
 * @param onLogin - Called with the user after a successful login.
 * @returns The login screen element.
 */
export function LoginScreen({
	brand,
	onLogin,
}: {
	brand: string
	onLogin: (user: User) => void | Promise<void>
}) {
	const [email, setEmail] = useState('')
	const [password, setPassword] = useState('')
	const attempt = useMutation({
		mutationFn: () => login(email.trim(), password),
		onSuccess: (user) => onLogin(user),
	})

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
								label="Email"
								type="email"
								value={email}
								onChange={(event) => setEmail(event.target.value)}
							/>
							<InputControl
								label="Password"
								type="password"
								value={password}
								onChange={(event) => setPassword(event.target.value)}
							/>
							<Button
								type="submit"
								disabled={
									email.trim() === '' || password === '' || attempt.isPending
								}
							>
								Log in
							</Button>
							{attempt.isError ? (
								<Text role="alert">{loginErrorMessage(attempt.error)}</Text>
							) : null}
						</Stack>
					</form>
				</Card.Content>
			</Card.Root>
		</div>
	)
}
