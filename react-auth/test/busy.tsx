// SPDX-License-Identifier: Apache-2.0

import { render } from '@testing-library/react'
import { Button } from '@wordpress/ui'

/**
 * Returns the class tokens the design system adds to a loading button.
 * @returns The tokens a busy button carries and an idle one does not.
 */
export function busyClasses(): string[] {
	const { container, unmount } = render(
		<>
			<Button id="idle">idle</Button>
			<Button id="busy" loading>
				busy
			</Button>
		</>,
	)
	const idle = new Set((container.querySelector('#idle') as Element).classList)
	const busy = [...(container.querySelector('#busy') as Element).classList]
	unmount()
	return busy.filter((token) => !idle.has(token))
}
