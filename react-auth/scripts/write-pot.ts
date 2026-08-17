// SPDX-License-Identifier: Apache-2.0

import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

import { DOMAIN, packageRoot, pot } from './pot.ts'

const root = packageRoot()

mkdirSync(join(root, 'languages'), { recursive: true })
writeFileSync(join(root, 'languages', `${DOMAIN}.pot`), pot(root))
