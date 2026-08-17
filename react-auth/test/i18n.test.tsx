// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { setLocaleData } from '@wordpress/i18n'
import { expect, test, vi } from 'vitest'

import { DOMAIN } from '../src/domain'
import { seedSession } from '../src/testing'
import { AccountPanel, LoginScreen, usersNavItem } from '../src/wpds'

setLocaleData(
	{
		'admin section\u0004Users': ['Usuarios'],
		'field label\u0004Email': ['Correo'],
		Password: ['Contrasena'],
		'Log in': ['Entrar'],
		'Log out': ['Salir'],
	},
	DOMAIN,
)

/**
 * Renders a tree needing a query client.
 * @param tree - The element to render.
 */
function renderWith(tree: React.ReactNode) {
	const client = new QueryClient({
		defaultOptions: {
			queries: { retry: false, staleTime: Infinity },
			mutations: { retry: false },
		},
	})
	seedSession(client)
	render(<QueryClientProvider client={client}>{tree}</QueryClientProvider>)
}

test('names the brick after itself and not after any product', () => {
	expect(DOMAIN).toBe('gopherium-react-auth')
})

test('the users nav label follows the catalogue the consumer loaded', () => {
	expect(usersNavItem.label).toBe('Usuarios')
})

test('the login screen follows the catalogue the consumer loaded', async () => {
	renderWith(<LoginScreen brand="Testbed" onLogin={vi.fn()} />)

	expect(await screen.findByLabelText('Correo')).toBeInTheDocument()
	expect(screen.getByLabelText('Contrasena')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Entrar' })).toBeInTheDocument()
})

test('leaves the brand the consumer supplied untranslated', async () => {
	renderWith(<LoginScreen brand="Testbed" onLogin={vi.fn()} />)

	expect(await screen.findByText('Testbed')).toBeInTheDocument()
})

test('the account panel follows the catalogue the consumer loaded', async () => {
	renderWith(<AccountPanel className="testbed-account" />)

	expect(await screen.findByRole('button', { name: 'Salir' })).toBeInTheDocument()
})
