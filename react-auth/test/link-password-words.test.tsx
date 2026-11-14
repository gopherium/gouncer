// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { resetLocaleData } from '@wordpress/i18n'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { DOMAIN } from '../src/domain'
import { catalogFor } from '../src/languages'
import { HttpResponse, http, server } from '../src/testing'
import { ResetPasswordScreen, SetPasswordScreen } from '../src/wpds'

const tooShort = { error: 'password must be at least 12 characters', code: 'password_too_short', meta: { min: 12 } }

const tooLong = { error: 'password must be at most 1024 characters', code: 'password_too_long', meta: { max: 1024 } }

const pages: { name: string; path: string; page: () => ReactNode }[] = [
	{
		name: 'activation',
		path: '/api/auth/activate',
		page: () => <SetPasswordScreen brand="Testbed" token="t-123" onAccepted={vi.fn()} />,
	},
	{
		name: 'reset',
		path: '/api/auth/password-reset/confirm',
		page: () => <ResetPasswordScreen brand="Testbed" token="t-123" />,
	},
]

const locales = [
	{
		locale: 'es-ES',
		label: 'Contraseña',
		submit: 'Guardar contraseña',
		hint: 'Sugerencia: La contraseña debe ser de al menos 12 caracteres.',
		long: 'Esta contraseña es demasiado larga.',
	},
	{
		locale: 'fr-FR',
		label: 'Mot de passe',
		submit: 'Enregistrer le mot de passe',
		hint: 'Conseil : Le mot de passe devrait contenir au moins 12 caractères.',
		long: 'Ce mot de passe est trop long.',
	},
]

afterEach(() => {
	resetLocaleData(undefined, DOMAIN)
})

async function rejectedOn(
	{ path, page }: (typeof pages)[number],
	{ locale, label, submit }: (typeof locales)[number],
	body: Record<string, unknown>,
): Promise<HTMLElement> {
	resetLocaleData(await catalogFor(locale), DOMAIN)
	server.use(http.post(path, () => HttpResponse.json(body, { status: 422 })))
	const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
	render(<QueryClientProvider client={client}>{page()}</QueryClientProvider>)
	await userEvent.type(await screen.findByLabelText(label), 'short-but-typed')
	await userEvent.click(screen.getByRole('button', { name: submit }))
	return screen.findByRole('alert')
}

test.each(pages.flatMap((page) => locales.map((words) => ({ page, words, name: page.name, locale: words.locale }))))(
	'the $name page words a password too short in $locale with the server minimum',
	async ({ page, words }) => {
		const alert = await rejectedOn(page, words, tooShort)

		expect(alert).toHaveTextContent(words.hint)
	},
)

test.each(pages.flatMap((page) => locales.map((words) => ({ page, words, name: page.name, locale: words.locale }))))(
	'the $name page words a password too long in $locale',
	async ({ page, words }) => {
		const alert = await rejectedOn(page, words, tooLong)

		expect(alert.textContent).toBe(words.long)
	},
)
