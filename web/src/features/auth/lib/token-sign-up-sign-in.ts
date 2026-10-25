/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { login } from '../api'
import type { LoginPayload } from '../types'

/**
 * TOKEN: signs a newly registered account in, so people land in the console
 * instead of the sign-in form. The sign-up response carries the new session
 * (the server signs the account in, since a separate login would need a
 * second one-time Turnstile token); without one (an older server), it logs in
 * through the standard login endpoint (same rate limits, password encryption
 * and session handling as the sign-in page). Returns false when that cannot
 * complete; the caller then sends the person to the sign-in page as before.
 * Errors are not shown here: the account exists, and signing in by hand still
 * works.
 */
export async function signInAfterSignUp(
  registered: unknown,
  credentials: LoginPayload,
  handleLoginResult: (result: unknown) => Promise<boolean>
): Promise<boolean> {
  try {
    if (carriesSession(registered)) {
      return await handleLoginResult(registered)
    }
    const res = await login(credentials)
    if (!res.success) return false
    return await handleLoginResult(res.data)
  } catch {
    return false
  }
}

function carriesSession(data: unknown): boolean {
  return (
    typeof data === 'object' &&
    data !== null &&
    typeof (data as { access_token?: unknown }).access_token === 'string'
  )
}
