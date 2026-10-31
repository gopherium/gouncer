// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { onTestFinished } from 'vitest'

/**
 * Loads the package stylesheet into the page until the test ends.
 * @returns The loaded sheet.
 */
export function withStylesheet(): CSSStyleSheet {
	const sheet = document.createElement('style')
	sheet.textContent = readFileSync(resolve('src/wpds/style.css'), 'utf8')
	document.head.append(sheet)
	onTestFinished(() => sheet.remove())
	return sheet.sheet as CSSStyleSheet
}
