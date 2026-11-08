// SPDX-License-Identifier: Apache-2.0

import { mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, isAbsolute, join, relative, resolve } from 'node:path'

import { build } from 'vite'
import type { Rolldown } from 'vite'
import { expect, onTestFinished, test } from 'vitest'

const manifest = resolve('package.json')

const usersEntry = resolve('src/wpds/users/index.ts')

const stylesheet = resolve('src/wpds/style.css')

const published = join(
	'package',
	(JSON.parse(readFileSync(manifest, 'utf8')) as { exports: Record<string, string> }).exports['./wpds/style.css'],
)

async function bundle(source: string, files: Record<string, string> = {}) {
	const workspace = realpathSync(mkdtempSync(join(tmpdir(), 'react-auth-bundle-')))
	onTestFinished(() => rmSync(workspace, { recursive: true, force: true }))
	for (const [path, content] of Object.entries(files)) {
		mkdirSync(dirname(join(workspace, path)), { recursive: true })
		writeFileSync(join(workspace, path), content)
	}
	const entry = join(workspace, 'entry.js')
	writeFileSync(entry, source)
	const resolved = new Map<string, Rolldown.ResolvedId>()
	const output = (await build({
		configFile: false,
		root: workspace,
		logLevel: 'silent',
		publicDir: false,
		plugins: [
			{
				name: 'record-resolved',
				enforce: 'pre',
				async resolveId(id, importer, options) {
					const found = await this.resolve(id, importer, options)
					if (found !== null) {
						resolved.set(found.id, found)
					}
					return found
				},
			},
		],
		build: {
			write: false,
			minify: false,
			rolldownOptions: {
				input: entry,
				preserveEntrySignatures: 'exports-only',
				external: (id) => !isAbsolute(id) && /^[\w@]/.test(id),
			},
		},
	})) as Rolldown.RolldownOutput
	const chunk = output.output.find((file) => file.type === 'chunk') as Rolldown.OutputChunk
	return { chunk, resolved, workspace }
}

test('bundles none of the users list for an entry taking only RoleSelect and the words of a failed write', async () => {
	const { chunk, resolved } = await bundle(
		`export { RoleSelect, addUserErrorMessageOf, errorMessageOf } from ${JSON.stringify(usersEntry)}`,
	)
	const taken = chunk.moduleIds.map((id) => relative(resolve(), id))

	expect(chunk.code).toContain('gopherium-role-select')
	expect(chunk.code).toContain('Sorry, you are not allowed to edit this user.')
	expect(chunk.code).toContain('Sorry, you are not allowed to create users.')
	expect(taken).not.toContain('src/wpds/users/UsersPage.tsx')
	expect(chunk.imports).not.toContain('@wordpress/dataviews')
	expect(resolved.get(usersEntry)).toMatchObject({ packageJsonPath: manifest, moduleSideEffects: false })
})

test('keeps a bare import of the stylesheet at its published path as a side effect', async () => {
	const { resolved, workspace } = await bundle(`import ${JSON.stringify(`./${published}`)}`, {
		'package/package.json': readFileSync(manifest, 'utf8'),
		[published]: readFileSync(stylesheet, 'utf8'),
	})
	const copy = resolved.get(join(workspace, published))

	expect(copy?.packageJsonPath).toBe(join(workspace, 'package/package.json'))
	expect(copy?.moduleSideEffects).not.toBe(false)
})
