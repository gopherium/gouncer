// SPDX-License-Identifier: Apache-2.0

import { installTestEnvironment } from '../src/testing'

/** keptFile is the File class a library holds when it loads before the install. */
export const { File: keptFile } = globalThis

installTestEnvironment()
