// SPDX-License-Identifier: Apache-2.0

export {
	InvalidCredentialsError,
	RateLimitedError,
	UnauthorizedError,
	fetchSession,
	login,
	logout,
} from './api'
export type { User } from './api'
export { AuthGate } from './AuthGate'
export { isSessionRevoked } from './probe'
export { createAuthQueryClient } from './queryClient'
export { sessionQueryKey, useLogout, useSession } from './session'
