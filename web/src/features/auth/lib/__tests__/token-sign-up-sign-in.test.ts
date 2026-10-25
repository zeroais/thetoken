/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { afterEach, expect, it, vi } from 'vitest'

import * as authApi from '../../api'
import { signInAfterSignUp } from '../token-sign-up-sign-in'

const credentials = {
  username: 'journeytest1',
  password: 'correct horse battery staple',
  passwordEncryptionEnabled: false,
}

afterEach(() => {
  vi.restoreAllMocks()
})

it('uses the session returned by the sign-up without a second login', async () => {
  const registered = { access_token: 'access', user: { id: 7 } }
  const login = vi.spyOn(authApi, 'login')
  const handleLoginResult = vi.fn().mockResolvedValue(true)

  await expect(
    signInAfterSignUp(registered, credentials, handleLoginResult)
  ).resolves.toBe(true)
  expect(handleLoginResult).toHaveBeenCalledWith(registered)
  expect(login).not.toHaveBeenCalled()
})

it('signs the new account in with the registered credentials', async () => {
  const session = { id: 7 }
  const login = vi
    .spyOn(authApi, 'login')
    .mockResolvedValue({ success: true, message: '', data: session as never })
  const handleLoginResult = vi.fn().mockResolvedValue(true)

  await expect(
    signInAfterSignUp(undefined, credentials, handleLoginResult)
  ).resolves.toBe(true)
  expect(login).toHaveBeenCalledWith(credentials)
  expect(handleLoginResult).toHaveBeenCalledWith(session)
})

it('falls back to the sign-in page when the login is refused', async () => {
  vi.spyOn(authApi, 'login').mockResolvedValue({
    success: false,
    message: 'Turnstile token is invalid',
  })
  const handleLoginResult = vi.fn()

  await expect(
    signInAfterSignUp(undefined, credentials, handleLoginResult)
  ).resolves.toBe(false)
  expect(handleLoginResult).not.toHaveBeenCalled()
})

it('falls back to the sign-in page when the login request fails', async () => {
  vi.spyOn(authApi, 'login').mockRejectedValue(new Error('network'))

  await expect(
    signInAfterSignUp(undefined, credentials, vi.fn())
  ).resolves.toBe(false)
})

it('falls back when the login needs a further step such as 2FA', async () => {
  vi.spyOn(authApi, 'login').mockResolvedValue({
    success: true,
    message: '',
    data: { require_2fa: true } as never,
  })

  await expect(
    signInAfterSignUp(undefined, credentials, vi.fn().mockResolvedValue(false))
  ).resolves.toBe(false)
})
