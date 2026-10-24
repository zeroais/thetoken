/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { describe, expect, it } from 'vitest'

import {
  TOKEN_BRAND,
  tokenApiOrigin,
  tokenSampleModel,
  tokenVersionLabel,
} from '../token-brand'

describe('tokenApiOrigin', () => {
  it('maps the app console host to the api host of the same domain', () => {
    expect(tokenApiOrigin('https://app.thetoken.dev')).toBe(
      'https://api.thetoken.dev'
    )
  })

  it('keeps any other origin, such as local development, unchanged', () => {
    expect(tokenApiOrigin('http://localhost:3000')).toBe(
      'http://localhost:3000'
    )
    expect(tokenApiOrigin('https://thetoken.dev')).toBe('https://thetoken.dev')
  })
})

describe('tokenSampleModel', () => {
  it('uses the TOKEN sample model before the model list is known or when it is empty', () => {
    expect(tokenSampleModel(undefined)).toBe(TOKEN_BRAND.sampleChatModel)
    expect(tokenSampleModel([])).toBe(TOKEN_BRAND.sampleChatModel)
  })

  it('prefers the TOKEN sample model when the user may use it', () => {
    expect(tokenSampleModel(['baai/bge-m3', TOKEN_BRAND.sampleChatModel])).toBe(
      TOKEN_BRAND.sampleChatModel
    )
  })

  it("falls back to the user's first model when the sample model is unavailable", () => {
    expect(tokenSampleModel(['zai-org/glm-5.3', 'baai/bge-m3'])).toBe(
      'zai-org/glm-5.3'
    )
  })
})

describe('tokenVersionLabel', () => {
  it('shows the TOKEN release and short commit instead of the New API rc tag', () => {
    expect(tokenVersionLabel('v1.0.0-rc.42+token.v1.0.0-88eac830449d')).toBe(
      'v1.0.0 (88eac83)'
    )
  })

  it('keeps a version without a TOKEN stamp, such as local development', () => {
    expect(tokenVersionLabel('v0.0.0')).toBe('v0.0.0')
    expect(tokenVersionLabel('v1.0.0-rc.42+token.local')).toBe('local')
  })
})
