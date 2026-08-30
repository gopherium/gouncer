// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'

/**
 * Announces an in-screen outcome, taking focus so it is read aloud.
 * @param children - The outcome to announce.
 * @returns The focused status element.
 */
export function Outcome({ children }: { children: ReactNode }) {
	const held = useRef<HTMLDivElement>(null)
	useEffect(() => {
		held.current?.focus()
	}, [])
	return (
		<div role="status" tabIndex={-1} ref={held}>
			{children}
		</div>
	)
}
