/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { AxiosError, AxiosHeaders } from 'axios'
import { expect, it } from 'vitest'

import { isMFAEnrollmentRequired } from '../token-mfa'

function apiError(status: number, code: string) {
  const config = { headers: new AxiosHeaders() }
  return new AxiosError('failed', String(status), config, undefined, {
    status,
    statusText: '',
    headers: {},
    config,
    data: { success: false, code },
  })
}

it('recognizes only the admin MFA enrolment denial', () => {
  expect(
    isMFAEnrollmentRequired(apiError(403, 'MFA_ENROLLMENT_REQUIRED'))
  ).toBe(true)
  expect(
    isMFAEnrollmentRequired(apiError(403, 'AUTH_INSUFFICIENT_PRIVILEGE'))
  ).toBe(false)
  expect(
    isMFAEnrollmentRequired(apiError(401, 'MFA_ENROLLMENT_REQUIRED'))
  ).toBe(false)
  expect(isMFAEnrollmentRequired(new Error('x'))).toBe(false)
})
