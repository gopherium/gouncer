// SPDX-License-Identifier: Apache-2.0

import {
	ErrorNotice,
	ListEmpty,
	ListNoResults,
	Page,
	focusedTrigger,
	openOnTap,
	useBulkRun,
	useDialogRun,
	useFocusBack,
	useListRun,
	viewNarrows,
} from '@gopherium/godmin'
import type { ListView, Trigger } from '@gopherium/godmin'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { UseQueryResult } from '@tanstack/react-query'
import { DataViews, filterSortAndPaginate } from '@wordpress/dataviews'
import type { Action, View } from '@wordpress/dataviews'
import { __, _x } from '@wordpress/i18n'
import { people } from '@wordpress/icons'
import { ButtonLink } from '@wordpress/ui'
import { useCallback, useMemo, useState } from 'react'
import type { ReactElement, ReactNode } from 'react'

import type { Account } from '../../account.js'
import { fetchUsers, usersQueryKey } from '../../admin/index.js'
import type { User } from '../../api.js'
import { DOMAIN } from '../../domain.js'
import { useSession } from '../../session.js'
import { mayChangeRole, useUsersActions } from './actions.js'
import { OpenRoleDialog } from './ChangeRoleDialog.js'
import type { OpenedRole } from './ChangeRoleDialog.js'
import { usersFields } from './fields.js'
import type { UsersColumn } from './fields.js'
import { ResendDialog } from './ResendDialog.js'
import type { OpenedResend } from './ResendDialog.js'
import type { ErrorMessages, RoleLabels } from './words.js'

/** What an app hands the users page: the view it holds, its settings and its own words, all as data. */
export interface UsersPageProps {
	/** The view godmin's useListView holds in the address. */
	list: ListView<View>
	/** The page sizes the list offers, none while the settings load. */
	pageSizes?: number[]
	/** Whether the settings naming the page size could not be read, every account then showing on one page. */
	sizesFailed?: boolean
	/** Whether the site shows avatars, true when absent. */
	showAvatars?: boolean
	/** The labels the app gives its own roles. */
	roleLabels?: RoleLabels
	/** The words the app gives the error codes only it answers. */
	errorMessages?: ErrorMessages
	/** The link element the Add User words go in, no header button when absent. */
	addLink?: ReactElement<Record<string, unknown>>
	/** The tabs drawn under the header. */
	tabs?: ReactNode
	/** The level of the page title, 2 on a screen beside the sidebar. */
	headingLevel?: 1 | 2
	/** The columns the app adds as data, after Role. */
	columns?: readonly UsersColumn[]
}

/** What the list takes beside what the app hands the page. */
interface ListProps extends UsersPageProps {
	/** The account query. */
	users: UseQueryResult<Account[]>
	/** The row and bulk actions. */
	actions: Action<Account>[]
	/** The signed-in account, none while nobody is signed in. */
	reader: User | null
	/** Opens Change role on the accounts, given the trigger it opened from. */
	openRole: (accounts: Account[], trigger: Trigger) => void
	/** Whether a write reloads the list. */
	refreshing: boolean
}

/** The accounts the list holds before the first read answers. */
const NO_ACCOUNTS: Account[] = []

/** The columns of a list the app adds none to. */
const NO_COLUMNS: readonly UsersColumn[] = []

/** The labels of a list whose app names no role. */
const NO_LABELS: RoleLabels = {}

/** The words of a list whose app answers no error code of its own. */
const NO_MESSAGES: ErrorMessages = {}

/** The roles a reader may give while the session names none. */
const NO_ROLES: readonly string[] = []

/**
 * Returns the words of the header button adding an account.
 * @returns The words in the reader's language.
 */
function addUserLabel(): string {
	return __('Add User', DOMAIN)
}

/**
 * Renders what the list shows when no account is on it.
 * @param props - Whether a search or a filter narrowed the list, and the link element the Add User words go in.
 * @returns The empty state, offering the link when nothing narrowed the list.
 */
function UsersEmpty({ narrowed, addLink }: Pick<UsersPageProps, 'addLink'> & { narrowed: boolean }) {
	if (narrowed) {
		return <ListNoResults />
	}
	return (
		<ListEmpty
			icon={people}
			title={__('No users yet', DOMAIN)}
			actions={
				addLink && (
					<ButtonLink variant="solid" render={addLink}>
						{addUserLabel()}
					</ButtonLink>
				)
			}
		/>
	)
}

/**
 * Renders the accounts as a list to search, filter, sort and page, with the actions the reader may take.
 * @param props - The account query, the actions, the reader, what opens Change role and what the app hands the page.
 * @returns The list, opening Change role from a name or a phone tap on an account the reader may change.
 */
function UsersList({ users, actions, reader, openRole, refreshing, ...props }: ListProps) {
	const { list } = props
	const accounts = users.data ?? NO_ACCOUNTS
	const showAvatars = props.showAvatars ?? true
	const roleLabels = props.roleLabels ?? NO_LABELS
	const columns = props.columns ?? NO_COLUMNS
	const fields = useMemo(
		() => usersFields(accounts, { roleLabels, showAvatars, columns, phone: list.phone }),
		[accounts, roleLabels, showAvatars, columns, list.phone],
	)
	const shown = filterSortAndPaginate(accounts, list.view, fields)
	const sizing = list.view.perPage === undefined && props.sizesFailed !== true
	const changeable = (account: Account) => mayChangeRole(reader, account)
	const tapped = (id: string) => {
		const account = accounts.find((held) => held.id === id) as Account
		if (changeable(account)) {
			openRole([account], 'row')
		}
	}
	return (
		<DataViews<Account>
			data={shown.data}
			paginationInfo={shown.paginationInfo}
			fields={fields}
			view={list.view}
			onChangeView={list.onChangeView}
			defaultLayouts={list.defaultLayouts}
			selection={list.selection}
			onChangeSelection={openOnTap(list, tapped)}
			isItemClickable={changeable}
			onClickItem={(account) => openRole([account], focusedTrigger())}
			actions={actions}
			isLoading={users.isPending || sizing || refreshing}
			searchLabel={__('Search users…', DOMAIN)}
			config={props.pageSizes === undefined ? undefined : { perPageSizes: props.pageSizes }}
			empty={<UsersEmpty narrowed={viewNarrows(list.view)} addLink={props.addLink} />}
		/>
	)
}

/**
 * Renders the Users page: the header, the accounts as a list and what a reader may do with them.
 * @param props - The view the app holds, its settings and its own words.
 * @returns The page.
 */
export function UsersPage(props: UsersPageProps) {
	const users = useQuery({ queryKey: usersQueryKey, queryFn: ({ signal }) => fetchUsers(signal) })
	const reader = useSession().data ?? null
	const client = useQueryClient()
	const { run, region, refreshing } = useListRun('users', props.list.onChangeSelection)
	const reload = useCallback(() => client.invalidateQueries({ queryKey: usersQueryKey }), [client])
	const switches = useBulkRun<Account>(run, reload)
	const write = useDialogRun<Account>(run, reload)
	const focusBack = useFocusBack(users.isFetching, region)
	const [opened, setOpened] = useState<OpenedRole>()
	const openRole = useCallback((accounts: Account[], trigger: Trigger) => setOpened({ accounts, trigger }), [])
	const [resending, setResending] = useState<OpenedResend>()
	const openResend = useCallback((account: Account, trigger: Trigger) => setResending({ account, trigger }), [])
	const errorMessages = props.errorMessages ?? NO_MESSAGES
	const roleLabels = props.roleLabels ?? NO_LABELS
	const actions = useUsersActions({ reader, switches, errorMessages, openRole, openResend })
	const grantable = reader?.grantable ?? NO_ROLES
	const title = _x('Users', 'admin section', DOMAIN)
	return (
		<Page
			title={title}
			headingLevel={props.headingLevel}
			subtitle={__('Manage who can sign in and what they can do.', DOMAIN)}
			actions={
				props.addLink === undefined ? undefined : (
					<ButtonLink variant="solid" size="compact" render={props.addLink}>
						{addUserLabel()}
					</ButtonLink>
				)
			}
			tabs={props.tabs}
			list
			listLabel={title}
			listRef={region}
		>
			{users.isError ? (
				<ErrorNotice>{__('Users could not be loaded.', DOMAIN)}</ErrorNotice>
			) : (
				<UsersList
					users={users}
					actions={actions}
					reader={reader}
					openRole={openRole}
					refreshing={refreshing}
					{...props}
				/>
			)}
			<OpenRoleDialog
				opened={opened}
				tools={{ grantable, roleLabels, errorMessages, write }}
				onClose={() => setOpened(undefined)}
				onClearSelection={() => props.list.onChangeSelection([])}
				onBack={focusBack}
			/>
			{resending === undefined ? null : (
				<ResendDialog
					account={resending.account}
					errorMessages={errorMessages}
					onClose={() => {
						setResending(undefined)
						focusBack(resending.account.id, resending.trigger)
					}}
				/>
			)}
		</Page>
	)
}
