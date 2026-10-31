// SPDX-License-Identifier: Apache-2.0

import { readFileSync, readdirSync } from 'node:fs'
import { join, resolve } from 'node:path'

import { expect, test } from 'vitest'

import { RoleNotGrantableError } from '../src/admin'
import * as wpds from '../src/wpds'
import { errorMessageOf, usersListDefaults } from '../src/wpds/users'

const manifest = JSON.parse(readFileSync(resolve('package.json'), 'utf8')) as {
	exports: Record<string, unknown>
	peerDependencies: Record<string, string>
	peerDependenciesMeta: Record<string, { optional?: boolean }>
	devDependencies: Record<string, string>
}

const listPeers: [name: string, window: string][] = [
	['@gopherium/godmin', '>=0.28.0 <0.30.0'],
	['@gopherium/gottext', '>=0.6.0 <0.8.0'],
	['@wordpress/components', '>=42.0.0 <43.0.0'],
	['@wordpress/dataviews', '>=20.0.0 <21.0.0'],
	['@wordpress/date', '>=5.57.0 <6.0.0'],
]

function sourceFiles(directory: string): string[] {
	return readdirSync(directory, { withFileTypes: true }).flatMap((entry) =>
		entry.isDirectory() ? sourceFiles(join(directory, entry.name)) : [join(directory, entry.name)],
	)
}

test('hands the users list out on its own entry, so a sign in only app never resolves the list peers', () => {
	expect(manifest.exports['./wpds/users']).toEqual({
		types: './dist/wpds/users/index.d.ts',
		default: './dist/wpds/users/index.js',
	})
	expect(wpds).not.toHaveProperty('UsersPage')
	expect(wpds).not.toHaveProperty('usersListDefaults')
})

test('drops the old users table and the invite only new user screen, which no app mounts', () => {
	expect(wpds).not.toHaveProperty('UsersScreen')
	expect(wpds).not.toHaveProperty('NewUserScreen')
})

test('drops the rules of the old users table from the stylesheet', () => {
	expect(readFileSync(resolve('src/wpds/style.css'), 'utf8')).not.toContain('.gopherium-users')
})

test.each(listPeers)('declares %s as an optional peer on the window %s', (name, window) => {
	expect(manifest.peerDependencies[name]).toBe(window)
	expect(manifest.peerDependenciesMeta[name]).toEqual({ optional: true })
	expect(manifest.devDependencies[name]).toMatch(/^\d+\.\d+\.\d+$/)
})

test('tests the list against godmin 0.29.0, the newest minor its window takes', () => {
	expect(manifest.devDependencies['@gopherium/godmin']).toBe('0.29.0')
})

test('keeps every source file free of a router, so the app holds the view', () => {
	const routed = sourceFiles(resolve('src')).filter((file) =>
		readFileSync(file, 'utf8').includes('@tanstack/react-router'),
	)

	expect(routed).toEqual([])
})

test('opens on Status and Role beside the name, newest first, as WordPress lists users with no Created column', () => {
	expect(usersListDefaults()).toEqual({
		fields: ['status', 'role'],
		phoneFields: ['status', 'role'],
		titleField: 'name',
		descriptionField: 'email',
		mediaField: 'avatar',
		sort: { field: 'created', direction: 'desc' },
		perPage: undefined,
	})
})

test('opens on the page size and the data columns the app names, the columns after Role and off the phone row', () => {
	const defaults = usersListDefaults({ perPage: 20, columns: ['posts'] })

	expect(defaults.fields).toEqual(['status', 'role', 'posts'])
	expect(defaults.phoneFields).toEqual(['status', 'role'])
	expect(defaults.perPage).toBe(20)
})

test("hands out the words of a failed account write, so an app's own Add User reads them as the list does", () => {
	const refused = new RoleNotGrantableError('Sorry, you are not allowed to give users that role.')

	expect(errorMessageOf(refused, {}, 'The user could not be created.')).toBe(
		'Sorry, you are not allowed to give users that role.',
	)
	expect(errorMessageOf(new Error('500'), {}, 'The user could not be created.')).toBe('The user could not be created.')
})

test('hands out a new view each time, so an app changing one never changes the next', () => {
	const first = usersListDefaults()
	first.fields.push('posts')

	expect(usersListDefaults().fields).toEqual(['status', 'role'])
})
