// Authentication API surface: the session query plus the sign-in/sign-up/reset
// commands. Kept apart from queries.ts and commands.ts because the session is
// not just another resource — it is what decides whether the rest of the app
// can be shown at all.
import { queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiFetch, isApiError } from './client'
import type {
  ForgotPasswordRequest,
  MessageResponse,
  ResetPasswordRequest,
  Session,
  SignInRequest,
  SignUpRequest,
} from './types'

export const authKeys = {
  session: () => ['auth', 'session'] as const,
}

/**
 * The current session, or null when signed out.
 *
 * A 401 is a legitimate answer here, not an error: it is how the API says
 * "nobody is signed in". Mapping it to null keeps every consumer off the error
 * path for the ordinary anonymous case.
 */
export const sessionQuery = () =>
  queryOptions({
    queryKey: authKeys.session(),
    queryFn: async (): Promise<Session | null> => {
      try {
        return await apiFetch<Session>('/auth/session')
      } catch (err) {
        if (isApiError(err) && err.isUnauthorized) return null
        throw err
      }
    },
    // The session is read on every guarded route; refetching it on each mount
    // would put a request in front of every navigation.
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

export function useSession() {
  return useQuery(sessionQuery())
}

/**
 * Sign in with an email and password.
 *
 * The response carries the session, so it seeds the cache directly rather than
 * invalidating and forcing a second round trip before the UI can settle.
 */
export function useSignIn() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: SignInRequest) =>
      apiFetch<Session>('/auth/signin', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: (session) => {
      qc.setQueryData(authKeys.session(), session)
    },
  })
}

/**
 * Register an account. Returns a message, not a session: the address has to be
 * verified before the account can sign in.
 */
export function useSignUp() {
  return useMutation({
    mutationFn: (body: SignUpRequest) =>
      apiFetch<MessageResponse>('/auth/signup', { method: 'POST', body: JSON.stringify(body) }),
  })
}

/**
 * End the session and drop every cached query.
 *
 * Clearing the whole cache matters: playlists and covers are another user's data
 * the moment this resolves, and leaving them in memory would show them to
 * whoever signs in next on a shared machine.
 */
export function useSignOut() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiFetch<void>('/auth/signout', { method: 'POST' }),
    onSettled: () => {
      qc.setQueryData(authKeys.session(), null)
      qc.clear()
    },
  })
}

/** Confirm an email address with the token from the emailed link. */
export function useVerifyEmail() {
  return useMutation({
    mutationFn: (token: string) =>
      apiFetch<MessageResponse>('/auth/verify-email', { method: 'POST', body: JSON.stringify({ token }) }),
  })
}

/**
 * Request a password reset link. Always succeeds from the caller's point of
 * view — the API answers identically whether or not the address is registered,
 * so this deliberately cannot be used to test whether an account exists.
 */
export function useForgotPassword() {
  return useMutation({
    mutationFn: (body: ForgotPasswordRequest) =>
      apiFetch<MessageResponse>('/auth/password/forgot', { method: 'POST', body: JSON.stringify(body) }),
  })
}

/**
 * Set a new password from a reset token. The API revokes every session as part
 * of this, so the cache is cleared: whatever it holds belongs to a session that
 * no longer exists.
 */
export function useResetPassword() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: ResetPasswordRequest) =>
      apiFetch<MessageResponse>('/auth/password/reset', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.setQueryData(authKeys.session(), null)
      qc.clear()
    },
  })
}

/**
 * Begin Sign in with Google.
 *
 * This is a full-page navigation rather than a fetch, on purpose: the flow is a
 * top-level redirect to Google and back, and the cookies carrying `state`,
 * `nonce` and the PKCE verifier only travel on a navigation of this kind.
 */
export function startGoogleSignIn(returnTo?: string) {
  const params = returnTo ? `?return=${encodeURIComponent(returnTo)}` : ''
  window.location.href = `/api/v1/auth/federated/google/start${params}`
}
