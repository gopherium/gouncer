// SPDX-License-Identifier: Apache-2.0

import { FormDialog, useToaster } from '@gopherium/godmin'
import type { DialogAnswer, Trigger } from '@gopherium/godmin'
import { useMutation } from '@tanstack/react-query'
import { __, sprintf } from '@wordpress/i18n'
import { InputControl } from '@wordpress/ui'
import { useEffect, useRef, useState } from 'react'

import type { Account } from '../../account.js'
import { resendInvite } from '../../admin/index.js'
import { DOMAIN } from '../../domain.js'
import { errorMessageOf } from './words.js'
import type { ErrorMessages } from './words.js'

/** The Resend invitation dialog the page holds open, on its account and the trigger it opened from. */
export interface OpenedResend {
	/** The pending account the dialog acts on. */
	account: Account
	/** The trigger the dialog opened from, which takes the focus back once the dialog is done. */
	trigger: Trigger
}

/** What Resend invitation takes from the page. */
interface ResendProps {
	/** The pending account the dialog acts on. */
	account: Account
	/** The words the app gives the error codes only it answers. */
	errorMessages: ErrorMessages
	/** Takes the dialog away, once its card has left. */
	onClose: () => void
}

/**
 * Renders the form sending a pending account a new invitation, then the activation link when no mail server sent it.
 * @param props - The account, the app's own error words and what takes the dialog away.
 * @returns The dialog, open until the write ends and open with the words of a refused resend.
 */
export function ResendDialog({ account, errorMessages, onClose }: ResendProps) {
	const toaster = useToaster()
	const resend = useMutation({ mutationFn: () => resendInvite(account.email) })
	const [link, setLink] = useState<string>()
	const linkField = useRef<HTMLInputElement>(null)
	useEffect(() => {
		if (link !== undefined) {
			linkField.current?.focus()
		}
	}, [link])
	const send = () =>
		resend.mutateAsync().then(
			(sent): DialogAnswer | undefined => {
				if (sent.delivered) {
					toaster.show(sprintf(__('Invitation sent to "%(name)s".', DOMAIN), { name: account.name }))
					return undefined
				}
				setLink(sent.activation_link)
				return { close: false }
			},
			(error: unknown): DialogAnswer => ({
				error: errorMessageOf(error, errorMessages, __('The invitation could not be sent.', DOMAIN)),
			}),
		)
	const question = __('Send a new invitation to "%(name)s" (%(email)s)? The link sent before stops working.', DOMAIN)
	const asking = link === undefined
	return (
		<FormDialog
			title={__('Resend invitation', DOMAIN)}
			description={
				asking
					? sprintf(question, { name: account.name, email: account.email })
					: __('No mail server is configured. Deliver the activation link by hand.', DOMAIN)
			}
			submitLabel={asking ? __('Resend invitation', DOMAIN) : __('Done', DOMAIN)}
			cancelLabel={asking ? __('Cancel', DOMAIN) : undefined}
			onSubmit={asking ? send : () => undefined}
			onClose={onClose}
		>
			{asking ? null : <InputControl ref={linkField} label={__('Activation link', DOMAIN)} readOnly value={link} />}
		</FormDialog>
	)
}
