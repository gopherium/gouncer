// SPDX-License-Identifier: Apache-2.0

import { __ } from '@wordpress/i18n'
import { Button, CheckboxControl, ControlWithError, Field, Input, Stack, Text } from '@wordpress/ui'
import { useId, useRef, useState } from 'react'
import type { ComponentProps, KeyboardEvent, ReactElement, ReactNode, RefObject } from 'react'

import { DOMAIN } from '../domain.js'
import { levelOf } from './strength.js'
import type { Tone } from './strength.js'
import { needsConfirmation, toneOf } from './usePasswordField.js'
import type { PasswordFieldState, Strength } from './usePasswordField.js'

/** Where a label sits as DataForm names it: beside its field as the installer's, or above it as a godmin form's. */
type LabelPosition = 'side' | 'top'

/** The message about the password, in the shape the WPDS ValidatedInputControl takes it. */
type CustomValidity = ComponentProps<typeof ControlWithError>['customValidity']

/** The classes of a row and its label, and the element its field root renders, none where the field stacks itself. */
interface RowLayout {
	/** The class of the row, the installer's two column grid. */
	row?: string
	/** The class of the label, the installer's th look. */
	label?: string
	/** The element the field root renders in place of the column a Field stacks its label and control in. */
	root?: ReactElement<Record<string, unknown>>
}

/** How a row lays out for each label position. */
const ROW_LAYOUTS: Record<LabelPosition, RowLayout> = {
	side: { row: 'gopherium-setup__row', label: 'gopherium-setup__label', root: <div /> },
	top: {},
}

/** The word the meter box holds for an empty password. */
const NO_WORD = '\u00a0'

/** The arrow drawn before the Caps Lock words. */
const CAPS_ARROW = (
	<span aria-hidden="true" className="gopherium-setup__caps-icon">
		<svg viewBox="0 0 24 26" fill="currentColor" stroke="currentColor" strokeWidth="0.5">
			<path d="M12 5L19 15H16V19H8V15H5L12 5Z" />
			<rect x="8" y="21" width="8" height="1.5" rx="0.75" />
		</svg>
	</span>
)

/**
 * Returns the word the meter box holds.
 * @param strength - What the meter knows of the password.
 * @returns Nothing before scoring, a no-break space for an empty field, or the level's word.
 */
function meterWord(strength: Strength): string {
	if (strength === undefined) {
		return ''
	}
	return strength === 'empty' ? NO_WORD : levelOf(strength).word
}

/**
 * Returns a class with its tone modifier when it has a tone.
 * @param block - The class without a tone.
 * @param tone - The tone to add, if any.
 * @returns The class names.
 */
function withTone(block: string, tone: Tone | undefined): string {
	return tone === undefined ? block : `${block} ${block}--${tone}`
}

/**
 * Returns the classes of the meter box, as WordPress marks its own.
 * @param strength - What the meter knows of the password.
 * @returns The box class with the empty mark for an empty field, or with the level's tone once scored.
 */
function meterClass(strength: Strength): string {
	const box = withTone('gopherium-setup__meter', toneOf(strength))
	return strength === 'empty' ? `${box} gopherium-setup__meter--empty` : box
}

/**
 * Reports whether a key types a character, the only keys Caps Lock is read from.
 * @param event - The key the person pressed.
 * @returns True for a printable key pressed without Control, Alt or Meta.
 */
function isPrintable(event: KeyboardEvent): boolean {
	return event.key.length === 1 && !event.ctrlKey && !event.altKey && !event.metaKey
}

/**
 * Renders the button that puts a new generated password in the field.
 * @param disabled - Whether the password stays as it is.
 * @param onGenerate - Called when the button is pressed.
 * @returns The button element.
 */
function GenerateButton({ disabled, onGenerate }: { disabled: boolean; onGenerate: () => void }) {
	return (
		<Button
			type="button"
			variant="outline"
			className="gopherium-setup__generate"
			disabled={disabled}
			onClick={onGenerate}
		>
			{__('Generate password', DOMAIN)}
		</Button>
	)
}

/**
 * Renders the button that shows or hides the password.
 * @param masked - Whether the password is hidden.
 * @param onToggle - Called when the button is pressed.
 * @returns The toggle element.
 */
function MaskToggle({ masked, onToggle }: { masked: boolean; onToggle: () => void }) {
	const show = __('Show', DOMAIN)
	const hide = __('Hide', DOMAIN)
	return (
		<Button
			type="button"
			variant="outline"
			className="gopherium-setup__toggle"
			data-other-word={masked ? hide : show}
			aria-label={masked ? __('Show password', DOMAIN) : __('Hide password', DOMAIN)}
			onClick={onToggle}
		>
			{masked ? show : hide}
		</Button>
	)
}

/**
 * Renders the box that shows the meter's word in its tone.
 * @param id - The id the password field is described by.
 * @param strength - What the meter knows of the password.
 * @returns The meter box element.
 */
function MeterBox({ id, strength }: { id: string; strength: Strength }) {
	return (
		<div id={id} aria-live="polite" className={meterClass(strength)}>
			{meterWord(strength)}
		</div>
	)
}

/**
 * Renders the line that warns while Caps Lock is on, empty while it is off.
 * @param on - Whether Caps Lock is on.
 * @returns The Caps Lock line element.
 */
function CapsLockLine({ on }: { on: boolean }) {
	return (
		<div aria-live="assertive" className="gopherium-setup__caps">
			{on ? (
				<>
					{CAPS_ARROW}
					<span className="gopherium-setup__caps-text">{__('Caps lock is on.', DOMAIN)}</span>
				</>
			) : null}
		</div>
	)
}

/**
 * Renders the label and the field column of the password row: the meter box, the Caps Lock line and the help line.
 * @param label - The label, which ControlWithError marks (Required) when the field is required.
 * @param required - Whether the field must hold a password, set by ControlWithError.
 * @param invalid - Whether the message about the password says it is invalid.
 * @param inputRef - Takes the password input, the element ControlWithError checks.
 * @param password - The password and its setters.
 * @param generateFirst - Whether the Generate password button comes before the field.
 * @param readOnly - Whether the password stays as it is.
 * @param help - The line under the field, if any.
 * @param layout - How the row lays out its label and field.
 * @returns The label and the field column.
 */
function PasswordCells({
	label,
	required,
	invalid,
	inputRef,
	password,
	generateFirst,
	readOnly,
	help,
	layout,
}: {
	label: ReactNode
	required?: boolean
	invalid: boolean
	inputRef: RefObject<HTMLInputElement | null>
	password: PasswordFieldState
	generateFirst: boolean
	readOnly: boolean
	help?: ReactNode
	layout: RowLayout
}) {
	const [capsLock, setCapsLock] = useState(false)
	const meterId = useId()
	const helpId = useId()
	return (
		<>
			<Field.Label className={layout.label}>{label}</Field.Label>
			<div>
				{generateFirst ? <GenerateButton disabled={readOnly} onGenerate={password.generate} /> : null}
				<div className="gopherium-setup__password">
					<div className="gopherium-setup__password-field">
						<Input
							ref={inputRef}
							type={password.masked ? 'password' : 'text'}
							className={withTone('gopherium-setup__input', toneOf(password.strength))}
							value={password.value}
							readOnly={readOnly}
							required={required}
							onChange={(event) => password.change(event.target.value)}
							onKeyDown={(event) => {
								if (isPrintable(event)) {
									setCapsLock(event.getModifierState('CapsLock'))
								}
							}}
							autoComplete="new-password"
							spellCheck={false}
							aria-invalid={invalid || undefined}
							aria-describedby={help === undefined ? meterId : `${meterId} ${helpId}`}
						/>
						<MeterBox id={meterId} strength={password.strength} />
						<CapsLockLine on={capsLock} />
					</div>
					<MaskToggle masked={password.masked} onToggle={() => password.mask(!password.masked)} />
				</div>
				{help === undefined ? null : <Text render={<p id={helpId} className="gopherium-setup__help" />}>{help}</Text>}
			</div>
		</>
	)
}

/**
 * Renders the row asking to confirm the use of a weak password.
 * @param checked - Whether the use is confirmed.
 * @param readOnly - Whether the choice stays as it is.
 * @param onCheckedChange - Called with the new choice.
 * @param layout - How the row lays out its label and checkbox.
 * @returns The row element.
 */
function ConfirmRow({
	checked,
	readOnly,
	onCheckedChange,
	layout,
}: {
	checked: boolean
	readOnly: boolean
	onCheckedChange: (checked: boolean) => void
	layout: RowLayout
}) {
	const label = <Field.VisualLabel className={layout.label}>{__('Confirm Password', DOMAIN)}</Field.VisualLabel>
	const checkbox = (
		<CheckboxControl
			className="gopherium-setup__checkbox"
			label={__('Confirm use of weak password', DOMAIN)}
			checked={checked}
			readOnly={readOnly}
			onCheckedChange={(next) => onCheckedChange(next)}
		/>
	)
	if (layout.row === undefined) {
		return (
			<Stack direction="column" gap="sm">
				{label}
				{checkbox}
			</Stack>
		)
	}
	return (
		<div className={layout.row}>
			{label}
			<div>{checkbox}</div>
		</div>
	)
}

/**
 * Renders the Password row with its meter and show or hide button, then the weak password row when one is needed.
 * @param password - The password and its setters, from usePasswordField.
 * @param generateFirst - Draws the Generate password button before the field, as WordPress's Add User does.
 * @param readOnly - Whether the password and its confirmation stay as they are, such as while the form is sent.
 * @param help - The line under the field, such as the installer's line asking to store the password.
 * @param labelPosition - Each label beside its field as the installer's, the default, or above it as a godmin form's.
 * @param required - Marks the label (Required) and holds the form back while the field is empty.
 * @param customValidity - The message under the field, an invalid one holding the form back.
 * @returns The password rows.
 */
export function PasswordField({
	password,
	generateFirst = false,
	readOnly = false,
	help,
	labelPosition = 'side',
	required,
	customValidity,
}: {
	password: PasswordFieldState
	generateFirst?: boolean
	readOnly?: boolean
	help?: ReactNode
	labelPosition?: LabelPosition
	required?: boolean
	customValidity?: CustomValidity
}) {
	const layout = ROW_LAYOUTS[labelPosition]
	const input = useRef<HTMLInputElement>(null)
	return (
		<>
			<ControlWithError
				required={required}
				customValidity={customValidity}
				getValidityTarget={() => input.current}
				render={<Field.Root className={layout.row} render={layout.root} />}
			>
				<PasswordCells
					label={__('Password', DOMAIN)}
					invalid={customValidity?.type === 'invalid'}
					inputRef={input}
					password={password}
					generateFirst={generateFirst}
					readOnly={readOnly}
					help={help}
					layout={layout}
				/>
			</ControlWithError>
			{needsConfirmation(password.strength) ? (
				<ConfirmRow checked={password.confirmed} readOnly={readOnly} onCheckedChange={password.confirm} layout={layout} />
			) : null}
		</>
	)
}
