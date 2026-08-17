// SPDX-License-Identifier: Apache-2.0

import { expect, test } from 'vitest'

import { catalogFor } from '../src/languages'

test('answers the catalogue a locale ships with', async () => {
	const held = await catalogFor('es-ES')

	expect(held).toBeDefined()
	expect(held?.['Log out']).toEqual(['Cerrar sesión'])
})

test('answers nothing for the language the sources are written in', async () => {
	expect(await catalogFor('en-US')).toBeUndefined()
})

test('answers nothing for a language the package ships no catalogue for', async () => {
	expect(await catalogFor('not-a-locale')).toBeUndefined()
})

test('carries the plural rule the runtime reads', async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['']).toMatchObject({ lang: 'es-ES' })
})
