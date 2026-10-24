/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { describe, expect, it } from 'vitest'

import { resources } from '../config'
import en from '../locales/en.json'
import tokenEn from '../locales/token-en.json'
import zh from '../locales/zh.json'

describe('TOKEN English wording', () => {
  it('overrides only strings that still exist upstream, so an upstream rename is caught', () => {
    const upstream = en.translation as Record<string, string>
    const stale = Object.keys(tokenEn).filter((key) => !(key in upstream))
    expect(stale).toEqual([])
  })

  it('registers credit, spending limit and routing group wording for English', () => {
    const english = resources.en.translation as Record<string, string>
    expect(english['Unlimited Quota']).toBe('No spending limit')
    expect(english['Remaining quota']).toBe('Remaining credit')
    expect(english['Group Ratio']).toBe('Group price multiplier')
    expect(english['Quota ({{currency}})']).toBe(
      'Spending limit ({{currency}})'
    )
  })

  it('describes billing on the home page as one price per model plus the published fee', () => {
    const english = resources.en.translation as Record<string, string>
    expect(english['Pay-as-you-go with real-time usage monitoring']).toBe(
      'One price per model, whichever provider answers. A 5.5% credit purchase fee, shown at checkout.'
    )
  })

  it('describes TOKEN on the home page, not a self-hosted New API gateway', () => {
    const english = resources.en.translation as Record<string, string>
    const home = [
      'Access a vast selection of models via a standard, unified API protocol. Power AI applications, manage digital assets, and connect the Future.',
      'Supports one-click configuration and perfectly adapts to NewAPI multi-protocol configuration.',
      'Community driven, self-hosted, and extensible',
      'Add your API keys, set up channels and configure access permissions',
      'Deploy your own gateway and start routing requests through your configured upstream services.',
    ].map((key) => english[key])
    for (const text of home) {
      expect(text).not.toMatch(/self-hosted|NewAPI|your own gateway|channels/)
    }
  })

  it('points customers to support when online top-up is off', () => {
    const english = resources.en.translation as Record<string, string>
    expect(
      english[
        'Online topup is not enabled. Please use redemption code or contact administrator.'
      ]
    ).toBe(
      'Top-ups are not available right now. Please contact support@thetoken.dev.'
    )
  })

  it('keeps every other English string and other languages unchanged', () => {
    const english = resources.en.translation as Record<string, string>
    const upstream = en.translation as Record<string, string>
    expect(Object.keys(english)).toHaveLength(Object.keys(upstream).length)
    expect(english['Copy']).toBe(upstream['Copy'])
    expect(resources.zhCN).toBe(zh)
  })
})
