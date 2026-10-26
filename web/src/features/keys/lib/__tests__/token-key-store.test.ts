/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { expect, it } from 'vitest'

import { rememberTokenKey, withSkPrefix } from '../token-key-store'

it('keeps keys in the tkn_ and earlier thetoken_ formats as they are', () => {
  expect(withSkPrefix('tkn_abc123XYZ')).toBe('tkn_abc123XYZ')
  expect(withSkPrefix('tkn_abcd…wxyz')).toBe('tkn_abcd…wxyz')
  expect(withSkPrefix('thetoken_abcd…wxyz')).toBe('thetoken_abcd…wxyz')
})

it('gives older keys and their hints the sk- prefix once', () => {
  expect(withSkPrefix('abcd…wxyz')).toBe('sk-abcd…wxyz')
  expect(withSkPrefix('sk-abcdef')).toBe('sk-abcdef')
})

it('remembers a new key exactly as issued', () => {
  expect(rememberTokenKey(7, 'tkn_abc123XYZ')).toBe('tkn_abc123XYZ')
})
