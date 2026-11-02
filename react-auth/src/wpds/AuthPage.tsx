// SPDX-License-Identifier: Apache-2.0

import { Link } from '@wordpress/ui'
import { Fragment } from 'react'
import type { ReactNode } from 'react'

/** The separator WordPress puts between two login links. */
const LINK_SEPARATOR = ' | '

/** The rel every link carries, so an address holding a token never travels as a referrer. */
const LINK_REL = 'noopener noreferrer'

/** A link under a sign-in form, as data. */
export interface AuthLink {
	/** The words shown, already translated. */
	label: string
	/** The full address. */
	href: string
	/** The link's own rel, added to the one every link carries. */
	rel?: string
}

/**
 * Renders the centred column of a sign-in screen, its links last.
 * @param links - The links under the form, none when empty.
 * @param children - The card and anything under it.
 * @returns The page element.
 */
export function AuthPage({
	links = [],
	children,
}: {
	links?: readonly AuthLink[]
	children: ReactNode
}) {
	return (
		<div className="gopherium-login">
			<div className="gopherium-login__column">
				{children}
				{links.length > 0 ? (
					<div className="gopherium-login__links">
						{links.map((link, index) => (
							<Fragment key={link.href}>
								{index > 0 ? LINK_SEPARATOR : null}
								<Link href={link.href} rel={link.rel ? `${LINK_REL} ${link.rel}` : LINK_REL}>
									{link.label}
								</Link>
							</Fragment>
						))}
					</div>
				) : null}
			</div>
		</div>
	)
}
