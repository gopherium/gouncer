// SPDX-License-Identifier: Apache-2.0

import type { ListDefaults } from '@gopherium/godmin'

/** What an app names for the view the users list opens on. */
export interface UsersListOptions {
	/** How many accounts a page holds, every account while it is unknown. */
	perPage?: number
	/** The ids of the data columns the app passes, shown after Role in the table and never in the phone row. */
	columns?: readonly string[]
}

/**
 * Returns the view the users list opens on, as data for godmin's useListView.
 * @param options - The page size and the data columns the app passes.
 * @returns The fields, the title, the line under it, the avatar, the order and the page size.
 */
export function usersListDefaults({ perPage, columns = [] }: UsersListOptions = {}): ListDefaults {
	return {
		fields: ['status', 'role', ...columns],
		phoneFields: ['status', 'role'],
		titleField: 'name',
		descriptionField: 'email',
		mediaField: 'avatar',
		sort: { field: 'created', direction: 'desc' },
		perPage,
	}
}
