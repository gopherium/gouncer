// SPDX-License-Identifier: Apache-2.0

import { bulkNotes, markLatest, runEach, useToaster } from '@gopherium/godmin'
import type { ToasterHandle, Trigger, UnsavedText } from '@gopherium/godmin'
import { FormDialog } from '@gopherium/godmin/dialogs'
import { __ } from '@wordpress/i18n'
import { Text } from '@wordpress/ui'
import { useState } from 'react'

import type { Account } from '../../account.js'
import { setUserRole } from '../../admin/index.js'
import { DOMAIN } from '../../domain.js'
import { RoleSelect } from './RoleSelect.js'
import { roleLineOf, roleWords } from './words.js'
import type { ErrorMessages, RoleLabels } from './words.js'

/** What Change role needs from the page. */
export interface RoleTools {
	/** The roles the reader may give, the one carrying most capabilities first. */
	grantable: readonly string[]
	/** The labels the app gives its own roles. */
	roleLabels: RoleLabels
	/** The words the app gives the error codes only it answers. */
	errorMessages: ErrorMessages
	/** The roles failed changes picked, by the accounts they were for, kept for the next open. */
	unsaved: UnsavedText
	/** Reads the list again. */
	reload: () => Promise<unknown>
	/** Marks the list reloading and returns what ends the mark. */
	onRefresh: () => () => void
}

/** The Change role dialog the page holds open, on the accounts it acts on and the trigger it opened from. */
export interface OpenedRole {
	/** The accounts the dialog acts on. */
	accounts: Account[]
	/** The trigger the dialog opened from, which takes the focus back once the dialog is done. */
	trigger: Trigger
}

/** What the dialog takes beside the accounts. */
interface DialogProps {
	/** The accounts the dialog acts on. */
	accounts: Account[]
	/** What the dialog needs from the page. */
	tools: RoleTools
	/** Takes the dialog away. */
	onClose: () => void
	/** Runs once the dialog sends, before it closes. */
	onSent: () => void
	/** Puts a focus the list lost while it read the accounts again back on the trigger the dialog opened from. */
	onSettled: () => void
}

/**
 * Returns the key the pick of a failed change is kept under, the ids of the accounts it was for.
 * @param accounts - The accounts the dialog acts on.
 * @returns The key.
 */
function keyOf(accounts: readonly Account[]): string {
	return accounts.map((account) => account.id).join(' ')
}

/**
 * Returns the role the dialog opens on: the one a failed change kept, else the one account's role the reader may give.
 * @param accounts - The accounts the dialog acts on.
 * @param tools - What the dialog needs from the page.
 * @returns The role, empty for several accounts or for a role the reader may not give.
 */
function startingRole(accounts: readonly Account[], tools: RoleTools): string {
	const kept = tools.unsaved.get(keyOf(accounts))
	if (kept !== undefined) {
		return kept
	}
	const [first] = accounts
	return accounts.length === 1 && tools.grantable.includes(first.role) ? first.role : ''
}

/**
 * Writes the role to each account, shows one toast and reads the list again, keeping the pick when a write failed.
 * @param accounts - The accounts the dialog acts on.
 * @param role - The role picked.
 * @param tools - What the dialog needs from the page.
 * @param toaster - Shows the toast.
 * @returns What settles once the list is read again.
 */
async function changeRoles(accounts: Account[], role: string, tools: RoleTools, toaster: ToasterHandle): Promise<void> {
	const key = keyOf(accounts)
	tools.unsaved.delete(key)
	const stillLatest = markLatest(tools.unsaved, key)
	const outcome = await runEach(accounts, (account) => setUserRole(account.id, role))
	const notes = bulkNotes(accounts, outcome, roleWords(tools.errorMessages))
	toaster.show(notes.notice ?? (notes.toast as string))
	if (outcome.failures.length > 0 && stillLatest()) {
		tools.unsaved.set(key, role)
	}
	const ended = outcome.done > 0 ? tools.onRefresh() : undefined
	await tools.reload().finally(ended)
}

/**
 * Renders Change role, closed the moment it is sent as a WordPress row edit closes, on one account or several.
 * @param props - The accounts, what the dialog needs from the page and what closes, sends and settles it.
 * @returns The dialog.
 */
function ChangeRoleDialog({ accounts, tools, onClose, onSent, onSettled }: DialogProps) {
	const toaster = useToaster()
	const [start] = useState(() => startingRole(accounts, tools))
	const [chosen, setChosen] = useState(start)
	const send = () => {
		onSent()
		onClose()
		void changeRoles(accounts, chosen, tools, toaster).then(onSettled)
	}
	return (
		<FormDialog
			title={__('Change role', DOMAIN)}
			size="medium"
			kind="edit"
			submitLabel={__('Change role', DOMAIN)}
			cancelLabel={__('Cancel', DOMAIN)}
			canSubmit={chosen !== ''}
			onSubmit={send}
			onClose={onClose}
		>
			<Text>{roleLineOf(accounts)}</Text>
			<RoleSelect
				grantable={tools.grantable}
				value={chosen}
				onChange={setChosen}
				roleLabels={tools.roleLabels}
				placeholder={start === '' ? __('Change role to…', DOMAIN) : undefined}
			/>
		</FormDialog>
	)
}

/**
 * Renders the Change role dialog the page holds open, none while it holds none.
 * @param props - The dialog, what it needs, what takes it away, what clears the selection and the focus back.
 * @returns The dialog, giving the focus back to its trigger once it closed and once its writes read the list again.
 */
export function OpenRoleDialog({
	opened,
	tools,
	onClose,
	onClearSelection,
	onBack,
}: {
	opened?: OpenedRole
	tools: RoleTools
	onClose: () => void
	onClearSelection: () => void
	onBack: (id: string, trigger: Trigger) => void
}) {
	if (opened === undefined) {
		return null
	}
	const back = () => onBack(opened.accounts[0].id, opened.trigger)
	const close = () => {
		onClose()
		back()
	}
	const sent = () => {
		if (opened.trigger === 'bulk') {
			onClearSelection()
		}
	}
	return <ChangeRoleDialog accounts={opened.accounts} tools={tools} onClose={close} onSent={sent} onSettled={back} />
}
