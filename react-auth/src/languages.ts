// SPDX-License-Identifier: Apache-2.0

/** A compiled catalogue, keyed by message with the metadata entry first. */
export type Catalog = Record<string, string[] | Record<string, string>>

/** The catalogue this package ships for each language it has one for. */
const SHIPPED: Record<string, () => Promise<{ default: Catalog }>> = {
	'es-ES': () => import('./languages/es-ES.json', { with: { type: 'json' } }),
}

/**
 * Returns the catalogue this package ships for a language.
 * @param locale - The language the reader settled on.
 * @returns The catalogue, or nothing when the package ships none for it.
 */
export async function catalogFor(locale: string): Promise<Catalog | undefined> {
	const load = SHIPPED[locale]
	return load === undefined ? undefined : (await load()).default
}
