import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'

import { expect, test } from 'vitest'

const require = createRequire(resolve('package.json'))

/**
 * Returns the design token names the given stylesheet reads.
 * @param css - The stylesheet source to scan.
 * @returns The distinct `--wpds-` token names referenced by `var()`.
 */
function tokensRead(css: string): string[] {
	return [...new Set([...css.matchAll(/var\(\s*(--wpds-[a-z0-9-]+)/g)].map((m) => m[1]))]
}

/**
 * Returns the design token names the given stylesheet declares.
 * @param css - The stylesheet source to scan.
 * @returns The distinct `--wpds-` token names assigned a value.
 */
function tokensDeclared(css: string): Set<string> {
	return new Set([...css.matchAll(/(--wpds-[a-z0-9-]+)\s*:/g)].map((m) => m[1]))
}

test('every design token the wpds stylesheet reads is declared by @wordpress/theme', () => {
	const ours = readFileSync(resolve('src/wpds/style.css'), 'utf8')
	const theme = readFileSync(require.resolve('@wordpress/theme/design-tokens.css'), 'utf8')

	const declared = tokensDeclared(theme)
	const missing = tokensRead(ours).filter((token) => !declared.has(token))

	expect(tokensRead(ours).length).toBeGreaterThan(0)
	expect(missing).toEqual([])
})
