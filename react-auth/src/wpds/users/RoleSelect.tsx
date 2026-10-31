// SPDX-License-Identifier: Apache-2.0

import { __ } from '@wordpress/i18n'
import { SelectControl } from '@wordpress/ui'

import { DOMAIN } from '../../domain.js'
import { roleLabelOf } from './words.js'
import type { RoleLabels } from './words.js'

/** One option of the role menu, the role as its value, none for the empty first option. */
interface SelectItem {
	value: string | null
	label: string
}

/** What the role menu takes, all data. */
export interface RoleSelectProps {
	/** The roles the reader may give, the one carrying most capabilities first, as the session lists them. */
	grantable: readonly string[]
	/** The role chosen, empty for none. */
	value: string
	/** Takes the role the reader picks, empty for the empty first option. */
	onChange: (role: string) => void
	/** The labels the app gives its own roles. */
	roleLabels?: RoleLabels
	/** The words of an empty first option, such as WordPress's Change role to…, no such option when absent. */
	placeholder?: string
}

/**
 * Renders the role menu, the roles the reader may give with the fewest capabilities first, as WordPress lists them.
 * @param props - The roles the reader may give, the role chosen, what takes a pick, the labels and the empty option.
 * @returns The select.
 */
export function RoleSelect({ grantable, value, onChange, roleLabels, placeholder }: RoleSelectProps) {
	const roles: SelectItem[] = [...grantable]
		.reverse()
		.map((role) => ({ value: role, label: roleLabelOf(role, roleLabels) }))
	const items = placeholder === undefined ? roles : [{ value: null, label: placeholder }, ...roles]
	return (
		<SelectControl
			className="gopherium-role-select"
			label={__('Role', DOMAIN)}
			items={items}
			value={items.find((item) => (item.value ?? '') === value) ?? null}
			placeholder={placeholder}
			onValueChange={(item) => onChange(item?.value ?? '')}
		/>
	)
}
