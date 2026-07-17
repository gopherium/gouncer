// SPDX-License-Identifier: Apache-2.0

export {
	InvalidCredentialsError,
	RateLimitedError,
	UnauthorizedError,
	fetchSession,
	login,
	logout,
} from './api.js'
export type { User } from './api.js'
export { AuthGate } from './AuthGate.js'
export { isSessionRevoked } from './probe.js'
export { createAuthQueryClient } from './queryClient.js'
export { sessionQueryKey, useLogout, useSession } from './session.js'
