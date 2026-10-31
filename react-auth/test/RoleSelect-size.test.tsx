// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join, resolve } from 'node:path'

import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { RoleSelect } from '../src/wpds/users'
import { withStylesheet } from './stylesheet'

const require = createRequire(resolve('package.json'))

const phone = 'max(var(--wpds-typography-font-size-md), 16px)'

const wide = '(min-width: 600px)'

function wideSizes(sheet: CSSStyleSheet, element: Element): string[] {
	return [...sheet.cssRules]
		.filter((rule): rule is CSSMediaRule => rule instanceof CSSMediaRule && rule.media.mediaText === wide)
		.flatMap((rule) => [...rule.cssRules])
		.filter((rule): rule is CSSStyleRule => rule instanceof CSSStyleRule && element.matches(rule.selectorText))
		.map((rule) => rule.style.fontSize)
}

test('draws the role menu text at the WPDS input phone size, at least 16px', () => {
	withStylesheet()
	render(<RoleSelect grantable={['admin', 'member']} value="member" onChange={() => {}} />)

	expect(getComputedStyle(screen.getByRole('combobox')).fontSize).toBe(phone)
})

test('brings the role menu text back to the WPDS body size from 600px wide, as the WPDS inputs do', () => {
	const sheet = withStylesheet()
	render(<RoleSelect grantable={['admin', 'member']} value="member" onChange={() => {}} />)

	expect(wideSizes(sheet, screen.getByRole('combobox'))).toEqual(['var(--wpds-typography-font-size-md)'])
})

test('follows the size and breakpoint the WPDS input itself uses', () => {
	const ui = dirname(require.resolve('@wordpress/ui/package.json'))
	const input = readFileSync(join(ui, 'src/form/primitives/input/style.module.css'), 'utf8')

	expect(input).toContain(`font-size: ${phone};`)
	expect(input).toContain(`@media ${wide}`)
})
