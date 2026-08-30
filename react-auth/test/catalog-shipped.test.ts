// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs'
import { po } from 'gettext-parser'
import { expect, test } from 'vitest'

import { catalogFor } from '../src/languages'

/** The byte Jed joins a context to its message with. */
const CONTEXT_SEPARATOR = String.fromCharCode(4)

/**
 * Returns every lookup key a template asks a catalogue to carry.
 * @param source - The template as POT text.
 * @returns The keys, read the way the runtime keys a message.
 */
function keysOf(source: string): string[] {
	const keys: string[] = []
	for (const [context, held] of Object.entries(po.parse(source).translations)) {
		for (const msgid of Object.keys(held)) {
			if (msgid !== '') {
				keys.push(context === '' ? msgid : context + CONTEXT_SEPARATOR + msgid)
			}
		}
	}
	return keys
}

const templateKeys = keysOf(readFileSync('languages/gopherium-react-auth.pot', 'utf8'))

test('a message carrying a quote is keyed the way the runtime keys it', () => {
	const source = 'msgid ""\nmsgstr ""\n\nmsgid "Type \\"delete\\" to confirm"\nmsgstr ""\n'

	expect(keysOf(source)).toEqual(['Type "delete" to confirm'])
})

test('the shipped catalogue translates every string the template asks for', async () => {
	const catalog = await catalogFor('es-ES')

	const missing = templateKeys.filter((key) => {
		const held = catalog?.[key]
		return !Array.isArray(held) || held[0] === undefined || held[0] === ''
	})

	expect(missing).toEqual([])
})

test('the shipped catalogue carries nothing the template dropped', async () => {
	const catalog = await catalogFor('es-ES')
	const wanted = new Set(templateKeys)

	const stale = Object.keys(catalog ?? {}).filter((key) => key !== '' && !wanted.has(key))

	expect(stale).toEqual([])
})
