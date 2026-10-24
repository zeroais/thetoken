/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { expect, it } from 'vitest'

import { getApiKeyFormDefaultValues } from '../api-key-form'

it('starts a new API key in the Default routing group', () => {
  expect(getApiKeyFormDefaultValues(false).group).toBe('default')
})

it('keeps the auto group when the site defaults keys to it', () => {
  expect(getApiKeyFormDefaultValues(true).group).toBe('auto')
})
