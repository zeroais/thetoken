/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { login } from '../api'
import type { LoginPayload } from '../types'

/**
 * TOKEN: signs a newly registered account in through the standard login
 * endpoint (same rate limits, password encryption and session handling as the
 * sign-in page), so people land in the console instead of the sign-in form.
 * Returns false when that login cannot complete (for example a one-time
 * Turnstile token was spent on the registration); the caller then sends the
 * person to the sign-in page as before. Errors are not shown here: the account
 * exists, and signing in by hand still works.
 */
export async function signInAfterSignUp(
  credentials: LoginPayload,
  handleLoginResult: (result: unknown) => Promise<boolean>
): Promise<boolean> {
  try {
    const res = await login(credentials)
    if (!res.success) return false
    return await handleLoginResult(res.data)
  } catch {
    return false
  }
}
