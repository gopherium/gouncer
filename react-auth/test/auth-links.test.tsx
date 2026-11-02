// SPDX-License-Identifier: Apache-2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Link } from '@wordpress/ui'
import type { ReactElement } from 'react'
import { expect, test, vi } from 'vitest'

import {
	HttpResponse,
	activateInvalidToken,
	defaultUser,
	http,
	loginInvalid,
	resetInvalidToken,
	resetOk,
	resetRequestOk,
	resetRequestRateLimited,
	server,
} from '../src/testing'
import {
	LoginScreen,
	RequestResetScreen,
	ResetPasswordScreen,
	SetPasswordScreen,
} from '../src/wpds'
import type { AuthLink } from '../src/wpds'
import { withStylesheet } from './stylesheet'

const terms: AuthLink = {
	label: 'Terms of service',
	href: 'https://example.com/terms',
	rel: 'terms-of-service',
}

const privacy: AuthLink = {
	label: 'Privacy policy',
	href: 'https://example.com/privacy',
	rel: 'privacy-policy',
}

const unnamed: AuthLink = { label: 'Imprint', href: 'https://example.com/imprint' }

/** Each screen, given its links. */
const screens: [string, (links?: readonly AuthLink[]) => ReactElement][] = [
	['LoginScreen', (links) => <LoginScreen brand="Testbed" onLogin={vi.fn()} links={links} />],
	['RequestResetScreen', (links) => <RequestResetScreen brand="Testbed" links={links} />],
	['ResetPasswordScreen', (links) => <ResetPasswordScreen brand="Testbed" token="t-1" links={links} />],
	[
		'SetPasswordScreen',
		(links) => <SetPasswordScreen brand="Testbed" token="t-1" onAccepted={vi.fn()} links={links} />,
	],
]

function renderScreen(element: ReactElement) {
	const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
	return render(<QueryClientProvider client={client}>{element}</QueryClientProvider>)
}

function linksBlock() {
	return document.querySelector('.gopherium-login__links')
}

function expectLinksBelowTheForm() {
	const block = linksBlock()
	const column = document.querySelector('.gopherium-login__column')

	expect(block?.parentElement).toBe(column)
	expect(column?.lastElementChild).toBe(block)
	expect(block?.closest('.gopherium-login__card')).toBeNull()
}

async function sendEmail() {
	await userEvent.type(screen.getByLabelText('Email'), 'maria@example.com')
	await userEvent.click(screen.getByRole('button', { name: 'Send reset link' }))
}

async function setPassword() {
	await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Set password' }))
}

test.each(screens)('%s draws the links in order with their addresses', (_, screenWith) => {
	renderScreen(screenWith([terms, privacy]))

	const drawn = screen.getAllByRole('link')

	expect(drawn.map((link) => link.textContent)).toEqual(['Terms of service', 'Privacy policy'])
	expect(drawn.map((link) => link.getAttribute('href'))).toEqual([terms.href, privacy.href])
})

test.each(screens)('%s adds the given rel to the one every link carries', (_, screenWith) => {
	renderScreen(screenWith([terms, unnamed]))

	expect(screen.getByRole('link', { name: 'Terms of service' })).toHaveAttribute(
		'rel',
		'noopener noreferrer terms-of-service',
	)
	expect(screen.getByRole('link', { name: 'Imprint' })).toHaveAttribute('rel', 'noopener noreferrer')
})

test.each(screens)('%s stays in the same tab, as WordPress does', (_, screenWith) => {
	renderScreen(screenWith([privacy]))

	expect(screen.getByRole('link', { name: 'Privacy policy' })).not.toHaveAttribute('target')
})

test.each(screens)('%s puts the links last in the column, outside the card', (_, screenWith) => {
	renderScreen(screenWith([privacy]))

	expectLinksBelowTheForm()
})

test.each(screens)('%s splits two links with the WordPress separator', (_, screenWith) => {
	renderScreen(screenWith([terms, privacy]))

	expect(linksBlock()).toHaveTextContent('Terms of service | Privacy policy')
})

test.each(screens)('%s draws no separator for one link', (_, screenWith) => {
	renderScreen(screenWith([privacy]))

	expect(linksBlock()?.textContent).toBe('Privacy policy')
})

test.each(screens)('%s draws no links block with no links', (_, screenWith) => {
	const { unmount } = renderScreen(screenWith())
	expect(linksBlock()).toBeNull()
	expect(document.querySelector('.gopherium-login')?.children).toHaveLength(1)
	unmount()

	renderScreen(screenWith([]))

	expect(linksBlock()).toBeNull()
})

test('keeps two links to one address apart', () => {
	const complaints = vi.spyOn(console, 'error')
	const address = 'https://example.com/legal'
	renderScreen(
		<LoginScreen
			brand="Testbed"
			onLogin={vi.fn()}
			links={[
				{ label: 'Terms of service', href: address },
				{ label: 'Privacy policy', href: address },
			]}
		/>,
	)

	expect(screen.getAllByRole('link').map((link) => link.textContent)).toEqual([
		'Terms of service',
		'Privacy policy',
	])
	expect(complaints).not.toHaveBeenCalled()
})

test('keeps the links under the lost password way and after a refused sign in', async () => {
	server.use(loginInvalid())
	renderScreen(
		<LoginScreen brand="Testbed" onLogin={vi.fn()} onForgotPassword={vi.fn()} links={[privacy]} />,
	)

	expect(linksBlock()?.previousElementSibling).toHaveClass('gopherium-login__nav')

	await userEvent.type(screen.getByLabelText('Email'), 'maria@example.com')
	await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))

	await screen.findByRole('alert')
	expectLinksBelowTheForm()
})

test.each([
	['once it is sent', resetRequestOk, 'If that address has an account, a reset link is on its way.'],
	['when it hits the limit', resetRequestRateLimited, 'Too many attempts. Please wait a minute and try again.'],
] as const)('keeps the links on the reset request %s', async (_, answer, words) => {
	server.use(answer())
	renderScreen(<RequestResetScreen brand="Testbed" links={[privacy]} />)

	await sendEmail()

	await screen.findByText(words)
	expectLinksBelowTheForm()
})

test.each([
	['once the password is set', resetOk, 'Your password is set. Sign in with it.'],
	['on a dead link', resetInvalidToken, 'This link is no longer valid. Ask for a new one.'],
] as const)('keeps the links on the reset screen %s', async (_, answer, words) => {
	server.use(answer())
	renderScreen(<ResetPasswordScreen brand="Testbed" token="t-1" links={[privacy]} />)

	await setPassword()

	await screen.findByText(words)
	expectLinksBelowTheForm()
})

test('keeps the links on the set password screen after a dead link', async () => {
	server.use(activateInvalidToken())
	renderScreen(
		<SetPasswordScreen brand="Testbed" token="t-1" onAccepted={vi.fn()} links={[privacy]} />,
	)

	await setPassword()

	await screen.findByRole('alert')
	expectLinksBelowTheForm()
})

test('keeps the links on the set password screen when the hand-off fails', async () => {
	server.use(http.post('/api/auth/activate', () => HttpResponse.json(defaultUser)))
	renderScreen(
		<SetPasswordScreen
			brand="Testbed"
			token="t-1"
			onAccepted={() => Promise.reject(new Error('the consumer could not navigate'))}
			links={[privacy]}
		/>,
	)

	await setPassword()

	await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong.'))
	expectLinksBelowTheForm()
})

test('draws the links as WordPress draws its privacy policy link', () => {
	const sheet = withStylesheet()
	const rule = [...sheet.cssRules].find(
		(candidate): candidate is CSSStyleRule =>
			candidate instanceof CSSStyleRule && candidate.selectorText === '.gopherium-login__links',
	)

	expect(rule?.style.margin).toBe('3em 0px 2em')
	expect(rule?.style.fontSize).toBe('var(--wpds-typography-font-size-md)')
	expect(rule?.style.lineHeight).toBe('1.4')
	expect(rule?.style.textAlign).toBe('center')
})

function defaultLinkClasses() {
	const { container, unmount } = render(<Link href="/">link</Link>)
	const tokens = [...(container.querySelector('a') as Element).classList]
	unmount()
	return tokens
}

test('draws each link at the default WPDS tone, as a WordPress page link', () => {
	const tokens = defaultLinkClasses()
	expect(tokens.length).toBeGreaterThan(0)
	renderScreen(<LoginScreen brand="Testbed" onLogin={vi.fn()} links={[privacy]} />)

	expect([...screen.getByRole('link', { name: 'Privacy policy' }).classList]).toEqual(tokens)
})
