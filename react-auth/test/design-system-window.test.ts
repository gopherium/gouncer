import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { expect, test } from 'vitest'

const manifest = JSON.parse(readFileSync(resolve('package.json'), 'utf8')) as {
	peerDependencies: Record<string, string>
	devDependencies: Record<string, string>
}

const numeric = { numeric: true }

const designSystemPeers = ['@wordpress/i18n', '@wordpress/icons', '@wordpress/theme', '@wordpress/ui']

test('declares the design system window this build is tested against', () => {
	expect(manifest.peerDependencies['@wordpress/ui']).toBe('>=0.25.0 <0.26.0')
	expect(manifest.peerDependencies['@wordpress/theme']).toBe('>=3.0.0 <4.0.0')
	expect(manifest.peerDependencies['@wordpress/i18n']).toBe('>=7.0.0 <8.0.0')
	expect(manifest.peerDependencies['@wordpress/icons']).toBe('>=19.0.0 <20.0.0')
})

test.each(designSystemPeers)('pins %s inside its peer window', (name) => {
	const range = /^>=(\S+) <(\S+)$/.exec(manifest.peerDependencies[name])
	const pin = manifest.devDependencies[name]

	expect(range, `${name} has no longhand peer range`).not.toBeNull()
	expect(pin, `${name} is not pinned exactly`).toMatch(/^\d+\.\d+\.\d+$/)
	expect(pin.localeCompare(range?.[1] ?? '', 'en', numeric), `${pin} is below ${range?.[1]}`).toBeGreaterThanOrEqual(0)
	expect(pin.localeCompare(range?.[2] ?? '', 'en', numeric), `${pin} reaches ${range?.[2]}`).toBeLessThan(0)
})
