// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs'
import { po } from 'gettext-parser'
import { expect, test } from 'vitest'

import { catalogFor } from '../src/languages'

const oursFuzzy = [
	'Unlock %s',
	'To ensure this site is securely set up by its owner, a setup code has been written to a file on the server.',
	'Please copy the setup code from the file and paste it below.',
	'Setup code',
	'Your setup code appears to be invalid. Please copy it again from the setup code file.',
	'Your setup code has expired. A new one is waiting in the setup code file.',
	'The setup code file cannot be used. Please check it and try again.',
	'You appear to have already set up this site.',
	'The site is starting. Please wait a moment and try again.',
	'Too many attempts. Please try again later.',
	'Sorry, that is not a valid email address. Email addresses look like <code>username@example.com</code>.',
	'Please provide a name.',
	'Hint: The password should be at least %d character long. To make it stronger, use upper and lower case letters, ' +
		'numbers, and symbols like ! " ? $ %% ^ & ).',
	'This name is too long.',
	'This password is too long.',
	'The account could not be created, please try again.',
]

const oursFuzzyInTheList: [context: string, msgid: string][] = [
	['', '"%(name)s" disabled.'],
	['', '"%(name)s" enabled.'],
	['', '%s user could not be disabled.'],
	['', '%s user could not be enabled.'],
	['', '%s user disabled.'],
	['', '%s user enabled.'],
	['', 'Change role'],
	['', 'Change role to…'],
	['', 'Change role…'],
	['', 'Changed roles.'],
	['', 'Choose the role of %(name)s.'],
	['', 'Choose the role of %s user.'],
	['', 'Created'],
	['', 'Invitation sent to "%(name)s".'],
	['', 'Manage who can sign in and what they can do.'],
	['', 'No users yet'],
	['', 'Resend invitation'],
	['', 'Resend invitation…'],
	['', 'Role'],
	['', 'Role changed for "%(name)s".'],
	['', 'Search users…'],
	['', 'Send a new invitation to "%(name)s" (%(email)s)? The link sent before stops working.'],
	['', 'Somebody must be able to manage users.'],
	['', 'Sorry, you are not allowed to give users that role.'],
	['', 'The role could not be changed.'],
	['', 'The role of %s user could not be changed.'],
	['', 'The role of that account just changed. Reload the page and try again.'],
	['', 'The user could not be disabled.'],
	['', 'The user could not be enabled.'],
	['', 'You cannot change your own role.'],
	['', 'You cannot disable your own account.'],
	['role', 'No role'],
	['user status', 'Invited'],
]

const wordpressReviewedInTheList: [context: string, msgid: string][] = [
	['', 'Add User'],
	['', 'Avatar'],
	['', 'Cancel'],
	['', 'Email'],
	['', 'Generate password'],
	['', 'Invalid user ID.'],
	['', 'Name'],
	['', 'Sorry, you are not allowed to create users.'],
	['', 'Sorry, you are not allowed to edit this user.'],
	['', 'Sorry, you are not allowed to edit users.'],
	['', 'Status'],
	['', 'The email address is not correct.'],
	['', 'This email is already registered. Please choose another one.'],
	['no user roles', 'None'],
	['user status', 'Active'],
	['user status', 'Disabled'],
]

const frenchWordpressReviewed: [context: string, msgid: string][] = [
	['', 'Add User'],
	['', 'Avatar'],
	['', 'Cancel'],
	['', 'Caps lock is on.'],
	['', 'Change role to…'],
	['', 'Changed roles.'],
	['', 'Confirm Password'],
	['', 'Confirm use of weak password'],
	['', 'Continue'],
	['', 'Created'],
	['', 'Disable'],
	['', 'Done'],
	['', 'Double-check your email address before continuing.'],
	['', 'Email'],
	['', 'Enable'],
	['', 'Generate password'],
	['', 'Hide'],
	['', 'Hide password'],
	['', 'Important:'],
	['', 'Invalid user ID.'],
	['', 'Loading…'],
	['', 'Lost your password?'],
	['', 'Name'],
	['', 'Password'],
	['', 'Role'],
	['', 'Show'],
	['', 'Show password'],
	['', 'Sorry, that is not a valid email address. Email addresses look like <code>username@example.com</code>.'],
	['', 'Sorry, you are not allowed to create users.'],
	['', 'Sorry, you are not allowed to edit this user.'],
	['', 'Sorry, you are not allowed to edit users.'],
	['', 'Sorry, you are not allowed to give users that role.'],
	['', 'Status'],
	['', 'Try again'],
	['', 'You will need this password to log\xa0in. Please store it in a secure location.'],
	['', 'Your Email'],
	['admin section', 'Users'],
	['field label', 'Email'],
	['field label', 'Name'],
	['no user roles', 'None'],
	['password strength', 'Medium'],
	['password strength', 'Password strength unknown'],
	['password strength', 'Strong'],
	['password strength', 'Very weak'],
	['password strength', 'Weak'],
	['role', 'No role'],
	['user status', 'Active'],
	['user status', 'Disabled'],
]

const wordpressReviewed: [context: string, msgid: string][] = [
	['', 'Continue'],
	['', 'Your Email'],
	['', 'Double-check your email address before continuing.'],
	['', 'Important:'],
	['', 'You will need this password to log\u00a0in. Please store it in a secure location.'],
	['', 'Hide'],
	['', 'Show'],
	['', 'Hide password'],
	['', 'Show password'],
	['', 'Caps lock is on.'],
	['password strength', 'Password strength unknown'],
	['password strength', 'Very weak'],
	['password strength', 'Weak'],
	['password strength', 'Medium'],
	['password strength', 'Strong'],
	['', 'Confirm Password'],
	['', 'Confirm use of weak password'],
]

function flagsOf(msgid: string, context = '', locale = 'es-ES'): string[] | undefined {
	const entry = po.parse(readFileSync(`languages/${locale}.po`, 'utf8')).translations[context]?.[msgid]
	return entry && (entry.comments?.flag ?? '').split(',').map((flag) => flag.trim())
}

function frenchEntries(): [context: string, msgid: string, fuzzy: boolean][] {
	const translations = po.parse(readFileSync('languages/fr-FR.po', 'utf8')).translations
	return Object.entries(translations).flatMap(([context, held]) =>
		Object.values(held)
			.filter((entry) => entry.msgid !== '')
			.map((entry): [string, string, boolean] => [context, entry.msgid, entry.comments?.flag === 'fuzzy']),
	)
}

test('answers the catalogue a locale ships with', async () => {
	const held = await catalogFor('es-ES')

	expect(held).toBeDefined()
	expect(held?.['Log out']).toEqual(['Cerrar sesión'])
})

test('carries the Spanish WordPress uses for a lost password', async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['Lost your password?']).toEqual(['¿Has olvidado tu contraseña?'])
})

test("carries WordPress's Spanish and French for an Add User the reader may no longer send", async () => {
	const refused = 'Sorry, you are not allowed to create users.'

	expect((await catalogFor('es-ES'))?.[refused]).toEqual(['Lo siento, no tienes permisos para crear usuarios.'])
	expect((await catalogFor('fr-FR'))?.[refused]).toEqual([
		'Désolé, vous n’avez pas l’autorisation de créer des comptes.',
	])
})

test("carries WordPress's Spanish for Continue", async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['Continue']).toEqual(['Continuar'])
})

test("carries WordPress's Spanish for the account step", async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['Your Email']).toEqual(['Tu correo electrónico'])
	expect(held?.['Important:']).toEqual(['Importante:'])
	expect(held?.['Hide password']).toEqual(['Ocultar la contraseña'])
	expect(held?.['Caps lock is on.']).toEqual(['El bloqueo de mayúsculas está activado.'])
})

test("carries WordPress's Spanish for the meter, Medio and never Mediano", async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['password strength\u0004Medium']).toEqual(['Medio'])
	expect(held?.['Confirm use of weak password']).toEqual(['Confirma el uso de una contraseña débil.'])
})

test("ships WordPress's own words reviewed and ours fuzzy", () => {
	for (const [context, msgid] of wordpressReviewed) {
		expect(flagsOf(msgid, context), msgid).toBeDefined()
		expect(flagsOf(msgid, context), msgid).not.toContain('fuzzy')
	}
	for (const msgid of oursFuzzy) {
		expect(flagsOf(msgid), msgid).toContain('fuzzy')
	}
})

test("ships the list's WordPress words reviewed and its own words fuzzy in Spanish", () => {
	for (const [context, msgid] of wordpressReviewedInTheList) {
		expect(flagsOf(msgid, context), msgid).toBeDefined()
		expect(flagsOf(msgid, context), msgid).not.toContain('fuzzy')
	}
	for (const [context, msgid] of oursFuzzyInTheList) {
		expect(flagsOf(msgid, context), msgid).toContain('fuzzy')
	}
})

test("ships WordPress's French reviewed and every other French word fuzzy", () => {
	const reviewed = new Set(frenchWordpressReviewed.map(([context, msgid]) => `${context}\u0004${msgid}`))
	const entries = frenchEntries()

	const misflagged = entries
		.filter(([context, msgid, fuzzy]) => fuzzy === reviewed.has(`${context}\u0004${msgid}`))
		.map(([context, msgid]) => `${context}\u0004${msgid}`)

	expect(entries.length).toBeGreaterThan(frenchWordpressReviewed.length)
	expect(misflagged).toEqual([])
})

test('answers the French catalogue with the language it is for', async () => {
	const held = await catalogFor('fr-FR')

	expect(held?.['']).toMatchObject({ lang: 'fr-FR' })
})

test('keeps Rol in Spanish where WordPress says Perfil', async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['Role']).toEqual(['Rol'])
	expect(held?.['Change role to…']).toEqual(['Cambiar el rol a…'])
	expect(held?.['Changed roles.']).toEqual(['Roles cambiados.'])
})

test('takes Comptes in French as WordPress names users', async () => {
	const held = await catalogFor('fr-FR')

	expect(held?.['admin section\u0004Users']).toEqual(['Comptes'])
	expect(held?.['Add User']).toEqual(['Ajouter un compte'])
	expect(held?.['No users yet']).toEqual(['Aucun compte pour l’instant'])
})

test('reads the users list failure the way both admins already say it in Spanish', async () => {
	const held = await catalogFor('es-ES')

	expect(held?.['Users could not be loaded.']).toEqual(['No se han podido cargar los usuarios.'])
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
