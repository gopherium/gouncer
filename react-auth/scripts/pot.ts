// SPDX-License-Identifier: Apache-2.0

import { GettextExtractor, JsExtractors } from 'gettext-extractor'
import { po, type GetTextTranslation } from 'gettext-parser'
import { join } from 'node:path'

/** The text domain every string this package owns is named under. */
export const DOMAIN = 'gopherium-react-auth'

/** The globs holding every string the package ships, read from the package root. */
const SOURCES = ['src/**/*.{ts,tsx}']

/** The paths inside those globs that ship no string. */
const IGNORED = [
	'**/node_modules/**',
	'**/*.d.ts',
	'**/test/**',
	'**/testing/**',
	'**/*.test.ts',
	'**/*.test.tsx',
	'**/scripts/**',
]

/** The comment positions the extractor leaves out of the template. */
const NO_COMMENTS = { otherLineLeading: false, sameLineLeading: false, sameLineTrailing: false }

/** The header the generated template carries, in this order. */
const HEADERS = {
	'Project-Id-Version': DOMAIN,
	'MIME-Version': '1.0',
	'Content-Type': 'text/plain; charset=UTF-8',
	'Content-Transfer-Encoding': '8bit',
	'Plural-Forms': 'nplurals=2; plural=(n != 1);',
	'X-Domain': DOMAIN,
}

/** A message one of the four gettext calls declared. */
export interface Found {
	text: string
	textPlural: string | null
	context: string | null
}

/**
 * Returns the package root the source globs resolve against.
 * @returns The absolute path of the package root.
 */
export function packageRoot(): string {
	return join(import.meta.dirname, '..')
}

/**
 * Returns the key an entry is ordered by.
 * @param entry - The entry to key.
 * @returns The context and message joined.
 */
function keyOf(entry: GetTextTranslation): string {
	return `${entry.msgctxt ?? ''} ${entry.msgid}`
}

/**
 * Returns the ordering placing entries by context then message, by code unit.
 * @param left - The entry on the left.
 * @param right - The entry on the right.
 * @returns A negative number when the left entry sorts first.
 */
function byKey(left: GetTextTranslation, right: GetTextTranslation): number {
	return Number(keyOf(left) > keyOf(right)) - Number(keyOf(left) < keyOf(right))
}

/**
 * Returns every translatable message the given sources declare.
 * @param root - The package root the globs resolve against.
 * @param sources - The globs to read, the package sources by default.
 * @returns The messages the extractor found.
 */
export function messages(root: string, sources: string[] = SOURCES): Found[] {
	const extractor = new GettextExtractor()
	const parser = extractor.createJsParser([
		JsExtractors.callExpression(['__', 'i18n.__'], {
			arguments: { text: 0 },
			comments: NO_COMMENTS,
		}),
		JsExtractors.callExpression(['_x', 'i18n._x'], {
			arguments: { text: 0, context: 1 },
			comments: NO_COMMENTS,
		}),
		JsExtractors.callExpression(['_n', 'i18n._n'], {
			arguments: { text: 0, textPlural: 1 },
			comments: NO_COMMENTS,
		}),
		JsExtractors.callExpression(['_nx', 'i18n._nx'], {
			arguments: { text: 0, textPlural: 1, context: 3 },
			comments: NO_COMMENTS,
		}),
	])
	for (const pattern of sources) {
		parser.parseFilesGlob(pattern, { cwd: root, ignore: IGNORED, absolute: true })
	}
	return extractor.getMessages() as Found[]
}

/**
 * Returns the catalogue template built from the given sources.
 * @param root - The package root the globs resolve against.
 * @param sources - The globs to read, the package sources by default.
 * @returns The template as UTF-8 bytes.
 */
export function pot(root: string, sources: string[] = SOURCES): Buffer {
	const translations: Record<string, Record<string, GetTextTranslation>> = {}
	for (const message of messages(root, sources)) {
		const context = message.context ?? ''
		translations[context] ??= {}
		translations[context][message.text] = {
			msgctxt: message.context ?? undefined,
			msgid: message.text,
			msgid_plural: message.textPlural ?? undefined,
			msgstr: message.textPlural ? ['', ''] : [''],
		}
	}
	return po.compile(
		{ charset: 'UTF-8', headers: HEADERS, translations },
		{ sort: byKey, foldLength: 0, eol: '\n' },
	)
}
