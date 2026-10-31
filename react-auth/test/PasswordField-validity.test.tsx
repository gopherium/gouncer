// SPDX-License-Identifier: Apache-2.0

import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ValidatedInputControl } from '@wordpress/ui'
import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { PasswordField, usePasswordField } from '../src/wpds'
import type { Meter } from '../src/wpds/strength'
import { withStylesheet } from './stylesheet'

const meter = vi.hoisted(() => ({ load: vi.fn() }))

vi.mock('../src/wpds/strength', async (importOriginal) => ({
	...(await importOriginal<typeof import('../src/wpds/strength')>()),
	loadMeter: meter.load,
}))

beforeEach(() => {
	meter.load.mockReset()
	meter.load.mockReturnValue(new Promise(() => {}))
})

const empty = 'Please enter a password.'

const refused = 'Please choose a different password.'

type Validity = { type: 'validating' | 'valid' | 'invalid'; message: string } | undefined

interface FormProps {
	required?: boolean
	customValidity?: Validity
	help?: ReactNode
	labelPosition?: 'side' | 'top'
	onSend?: () => void
	withEmail?: boolean
}

function Form({ required, customValidity, help, labelPosition = 'top', onSend = () => {}, withEmail }: FormProps) {
	const password = usePasswordField({ length: 24, userInputs: [] })
	return (
		<form
			onSubmit={(event) => {
				event.preventDefault()
				onSend()
			}}
		>
			{withEmail ? <ValidatedInputControl label="Email" value="" customValidity={customValidity} /> : null}
			<PasswordField
				password={password}
				generateFirst
				labelPosition={labelPosition}
				help={help}
				required={required}
				customValidity={customValidity}
			/>
			<input aria-label="Elsewhere" />
			<button type="submit">Send</button>
		</form>
	)
}

function SentForm() {
	const password = usePasswordField({ length: 24, userInputs: [] })
	const [message, setMessage] = useState<string>()
	const [reported, setReported] = useState<boolean>()
	const sent = useRef<HTMLFormElement>(null)
	useEffect(() => {
		if (message !== undefined) {
			setReported(sent.current?.reportValidity())
		}
	}, [message])
	return (
		<form
			ref={sent}
			onSubmit={(event) => {
				event.preventDefault()
				setMessage(refused)
			}}
		>
			<PasswordField
				password={password}
				generateFirst
				labelPosition="top"
				customValidity={message === undefined ? undefined : { type: 'invalid', message }}
			/>
			<output>{String(reported)}</output>
			<button type="submit">Send</button>
		</form>
	)
}

function passwordField() {
	return screen.getByLabelText(/^Password/) as HTMLInputElement
}

function form() {
	return passwordField().form as HTMLFormElement
}

function describedBy(field: Element): HTMLElement[] {
	const ids = field.getAttribute('aria-describedby')?.split(' ') ?? []
	return ids.map((id) => document.getElementById(id) as HTMLElement)
}

function expectInOrder(...nodes: Element[]) {
	for (let index = 1; index < nodes.length; index += 1) {
		const following = nodes[index - 1].compareDocumentPosition(nodes[index]) & Node.DOCUMENT_POSITION_FOLLOWING
		expect(following, `node ${index} follows node ${index - 1}`).not.toBe(0)
	}
}

async function leave(field: HTMLElement) {
	await userEvent.click(field)
	await userEvent.click(screen.getByLabelText('Elsewhere'))
}

function invalidAbout(): Validity {
	return { type: 'invalid', message: empty }
}

function borderRulesOn(sheet: CSSStyleSheet, element: Element): string[] {
	return [...sheet.cssRules]
		.filter((rule): rule is CSSStyleRule => rule instanceof CSSStyleRule && rule.cssText.includes('border-color'))
		.filter((rule) => element.matches(rule.selectorText))
		.map((rule) => rule.selectorText)
}

test('marks the label (Required) and the field required with required, as ValidatedInputControl does', () => {
	render(<Form required />)

	expect(screen.getByLabelText('Password (Required)')).toBe(passwordField())
	expect(passwordField()).toBeRequired()
})

test('keeps the label and the field as they were without required', () => {
	render(<Form />)

	expect(screen.getByLabelText('Password')).not.toBeRequired()
})

test('shows an invalid customValidity under the field once it is left, as the WPDS fields draw theirs', async () => {
	render(<Form withEmail customValidity={invalidAbout()} />)
	expect(screen.queryAllByText(empty)).toEqual([])

	await leave(passwordField())
	await leave(screen.getByLabelText('Email'))

	const [email, password] = screen.getAllByText(empty)
	expect(password.className).toBe(email.className)
	expect(password.tagName).toBe('P')
	expect(password.querySelector('svg')).not.toBeNull()
	expect(passwordField()).toHaveAttribute('data-validity-visible')
})

test('keeps the message hidden while the focus moves inside the row, to Hide', async () => {
	render(<Form customValidity={invalidAbout()} />)

	await userEvent.click(passwordField())
	await userEvent.click(screen.getByRole('button', { name: 'Hide password' }))

	expect(screen.queryByText(empty)).toBeNull()
})

test('sets aria-invalid while customValidity is invalid, as WordPress marks its validated controls', () => {
	const { rerender } = render(<Form customValidity={invalidAbout()} />)
	expect(passwordField()).toHaveAttribute('aria-invalid', 'true')

	rerender(<Form customValidity={{ type: 'valid', message: 'Looks good.' }} />)
	expect(passwordField()).not.toHaveAttribute('aria-invalid')

	rerender(<Form />)
	expect(passwordField()).not.toHaveAttribute('aria-invalid')
})

test('describes the field with the meter, the help, then the message once it shows', async () => {
	render(<Form help="Store it somewhere safe." customValidity={invalidAbout()} />)

	await leave(passwordField())

	const [box, help, message] = describedBy(passwordField())
	expect(box).toHaveAttribute('aria-live', 'polite')
	expect(help).toHaveTextContent('Store it somewhere safe.')
	expect(message).toHaveTextContent(empty)
	expect(passwordField()).toHaveAccessibleDescription(expect.stringContaining(empty))
})

test('shows the message once an invalid event fires on the field', async () => {
	render(<Form customValidity={invalidAbout()} />)

	await act(async () => fireEvent.invalid(passwordField()))

	expect(describedBy(passwordField()).at(-1)).toHaveTextContent(empty)
})

test("shows the message once the form's reportValidity() runs", async () => {
	render(<Form customValidity={invalidAbout()} />)

	let reported = true
	await act(async () => {
		reported = form().reportValidity()
	})

	expect(reported).toBe(false)
	expect(screen.getByText(empty)).toBeInTheDocument()
})

test('shows a message set after a send once reportValidity() runs in an effect after that render', async () => {
	render(<SentForm />)

	await userEvent.click(screen.getByRole('button', { name: 'Send' }))

	expect(screen.getByRole('status')).toHaveTextContent('false')
	expect(describedBy(passwordField()).at(-1)).toHaveTextContent(refused)
})

test('holds the form back while the field is invalid, then lets it go once the message is cleared', async () => {
	const onSend = vi.fn()
	const { rerender } = render(<Form customValidity={invalidAbout()} onSend={onSend} />)

	await userEvent.click(screen.getByRole('button', { name: 'Send' }))

	expect(onSend).not.toHaveBeenCalled()
	expect(screen.getByText(empty)).toBeInTheDocument()

	rerender(<Form onSend={onSend} />)
	await userEvent.click(screen.getByRole('button', { name: 'Send' }))

	expect(onSend).toHaveBeenCalledOnce()
	expect(screen.queryByText(empty)).toBeNull()
})

test("holds the form back while a required field is empty, with the browser's own words under it", async () => {
	const onSend = vi.fn()
	render(<Form required onSend={onSend} />)
	expect(form().checkValidity()).toBe(true)

	fireEvent.change(passwordField(), { target: { value: '' } })
	await leave(passwordField())

	expect(form().checkValidity()).toBe(false)
	expect(describedBy(passwordField()).at(-1)).toHaveTextContent(passwordField().validationMessage)
	await userEvent.click(screen.getByRole('button', { name: 'Send' }))
	expect(onSend).not.toHaveBeenCalled()
})

test('puts the message under the meter and the Caps Lock line, never between the field and its meter', async () => {
	render(<Form help="Store it somewhere safe." customValidity={invalidAbout()} />)

	await leave(passwordField())

	const [box, help, message] = describedBy(passwordField())
	const caps = document.querySelector('.gopherium-setup__caps') as HTMLElement
	expectInOrder(passwordField(), box, caps, screen.getByRole('button', { name: 'Hide password' }), help, message)
})

test("puts the message under the meter at ValidatedInputControl's gap, with labelPosition top", async () => {
	withStylesheet()
	let land: (loaded: Meter) => void = () => {}
	meter.load.mockReturnValue(
		new Promise<Meter>((resolve) => {
			land = resolve
		}),
	)
	render(<Form withEmail customValidity={invalidAbout()} />)
	await act(async () => land(() => 4))

	await leave(passwordField())
	await leave(screen.getByLabelText('Email'))

	const [email, password] = screen.getAllByText(empty)
	const box = describedBy(passwordField())[0]
	expect((password.parentElement as HTMLElement).style.gap).toBe((email.parentElement as HTMLElement).style.gap)
	expect(box).toHaveClass('gopherium-setup__meter--strong')
	expect(getComputedStyle(box).marginBottom).toBe('0px')
})

test("puts the message in the field column of the installer's row, with labelPosition side", async () => {
	withStylesheet()
	render(<Form labelPosition="side" customValidity={invalidAbout()} />)

	await leave(passwordField())

	const message = screen.getByText(empty)
	const row = screen.getByText('Password').parentElement as HTMLElement
	expect(row).toHaveClass('gopherium-setup__row')
	expect(message.parentElement).toBe(row)
	expect(getComputedStyle(message).gridColumn).toBe('-2 / -1')
	expect(getComputedStyle(row).rowGap).toBe('4px')
})

test('leaves the field border to the WPDS error colour over the meter tone once the message shows', async () => {
	const sheet = withStylesheet()
	let land: (loaded: Meter) => void = () => {}
	meter.load.mockReturnValue(
		new Promise<Meter>((resolve) => {
			land = resolve
		}),
	)
	render(<Form customValidity={invalidAbout()} />)
	await act(async () => land(() => 4))
	const layout = passwordField().parentElement as HTMLElement
	expect(layout).toHaveClass('gopherium-setup__input--strong')
	expect(borderRulesOn(sheet, layout)).toEqual([expect.stringContaining('.gopherium-setup__input--strong')])

	await leave(passwordField())

	expect(passwordField()).toHaveAttribute('data-validity-visible')
	expect(passwordField()).toBeInvalid()
	expect(borderRulesOn(sheet, layout)).toEqual([])
})
