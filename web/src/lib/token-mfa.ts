/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import axios from 'axios'

/**
 * TOKEN: admin and root accounts must enrol TOTP or a passkey. Until they
 * do, the API answers 403 with this code for everything except enrolment.
 */
export const MFA_ENROLLMENT_REQUIRED = 'MFA_ENROLLMENT_REQUIRED'

export function isMFAEnrollmentRequired(error: unknown): boolean {
  return (
    axios.isAxiosError(error) &&
    error.response?.status === 403 &&
    (error.response.data as { code?: string } | undefined)?.code ===
      MFA_ENROLLMENT_REQUIRED
  )
}

/** Send the admin to Security & Access, once, with the enrolment banner. */
export function redirectToMFAEnrollment(): void {
  if (typeof window === 'undefined' || window.location.pathname === '/security')
    return
  window.location.replace('/security?mfa=required')
}
