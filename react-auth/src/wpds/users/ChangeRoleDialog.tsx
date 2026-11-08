// SPDX-License-Identifier: Apache-2.0

import { FormDialog } from '@gopherium/godmin'
import type { DialogRun, Trigger } from '@gopherium/godmin'
import { __ } from '@wordpress/i18n'
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
	/** Runs the role write over the accounts, reads the list again and answers the dialog. */
	write: DialogRun<Account>
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
	/** Takes the dialog away, once its card has left. */
	onClose: () => void
	/** Runs once the write ended and the dialog closes. */
	onSent: () => void
}

/**
 * Returns the role the dialog opens on, the one account's role the reader may give.
 * @param accounts - The accounts the dialog acts on.
 * @param grantable - The roles the reader may give.
 * @returns The role, empty for several accounts or for a role the reader may not give.
 */
function startingRole(accounts: readonly Account[], grantable: readonly string[]): string {
	const [first] = accounts
	return accounts.length === 1 && grantable.includes(first.role) ? first.role : ''
}

/**
 * Renders Change role on one account or several, open until its write ends, and open after one account was refused.
 * @param props - The accounts, what the dialog needs from the page and what closes and sends it.
 * @returns The dialog.
 */
function ChangeRoleDialog({ accounts, tools, onClose, onSent }: DialogProps) {
	const [start] = useState(() => startingRole(accounts, tools.grantable))
	const [chosen, setChosen] = useState(start)
	const send = () =>
		tools.write(accounts, (account) => setUserRole(account.id, chosen), roleWords(tools.errorMessages)).then((answer) => {
			if (answer === undefined) {
				onSent()
			}
			return answer
		})
	return (
		<FormDialog
			title={__('Change role', DOMAIN)}
			description={roleLineOf(accounts)}
			submitLabel={__('Change role', DOMAIN)}
			cancelLabel={__('Cancel', DOMAIN)}
			canSubmit={chosen !== ''}
			onSubmit={send}
			onClose={onClose}
		>
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
 * @returns The dialog, giving the focus back to its trigger once it closed.
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
	const close = () => {
		onClose()
		onBack(opened.accounts[0].id, opened.trigger)
	}
	const sent = () => {
		if (opened.trigger === 'bulk') {
			onClearSelection()
		}
	}
	return <ChangeRoleDialog accounts={opened.accounts} tools={tools} onClose={close} onSent={sent} />
}
