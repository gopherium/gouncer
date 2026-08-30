// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs'
import { expect, test } from 'vitest'

import { catalogFor } from '../src/languages'

const pot = readFileSync('languages/gopherium-react-auth.pot', 'utf8')

/**
 * Returns every lookup key the template asks a catalogue to carry.
 * @returns The keys, context joined to the message the way the runtime joins them.
 */
function templateKeys(): string[] {
	const keys: string[] = []
	let context = ''
	for (const line of pot.split('\n')) {
		const asContext = /^msgctxt "(.*)"$/.exec(line)
		if (asContext) {
			context = asContext[1]
			continue
		}
		const asMessage = /^msgid "(.+)"$/.exec(line)
		if (asMessage) {
			keys.push(context === '' ? asMessage[1] : `${context}${asMessage[1]}`)
			context = ''
		}
	}
	return keys
}

test('the shipped catalogue translates every string the template asks for', async () => {
	const catalog = await catalogFor('es-ES')

	const missing = templateKeys().filter((key) => {
		const held = catalog?.[key]
		return !Array.isArray(held) || held[0] === undefined || held[0] === ''
	})

	expect(missing).toEqual([])
})

test('the shipped catalogue carries nothing the template dropped', async () => {
	const catalog = await catalogFor('es-ES')
	const wanted = new Set(templateKeys())

	const stale = Object.keys(catalog ?? {}).filter((key) => key !== '' && !wanted.has(key))

	expect(stale).toEqual([])
})
