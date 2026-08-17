// SPDX-License-Identifier: Apache-2.0

import { _x } from '@wordpress/i18n'
import type { ComponentProps, ReactElement } from 'react'

import { DOMAIN } from '../domain.js'

const usersIcon = (
	<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" fill="currentColor">
		<path
			d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z"
		/>
	</svg>
)

/**
 * usersNavItem is the ready-made navigation entry for the user
 * administration screen.
 */
export const usersNavItem: {
	label: string
	to: string
	icon: ReactElement<ComponentProps<'svg'>>
} = {
	get label() {
		return _x('Users', 'admin section', DOMAIN)
	},
	to: '/users',
	icon: usersIcon,
}
