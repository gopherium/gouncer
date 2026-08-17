// SPDX-License-Identifier: Apache-2.0

import { mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

import { compileCatalog, serializeCatalog } from './jed.ts'
import { packageRoot } from './pot.ts'

const root = packageRoot()
const sources = join(root, 'languages')
const built = join(root, 'src', 'languages')

mkdirSync(built, { recursive: true })
for (const file of readdirSync(sources)) {
	if (!file.endsWith('.po')) {
		continue
	}
	const locale = file.slice(0, -'.po'.length)
	const compiled = compileCatalog(readFileSync(join(sources, file), 'utf8'))
	writeFileSync(join(built, `${locale}.json`), serializeCatalog(compiled))
}
