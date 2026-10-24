/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { render, screen, waitFor } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import { api } from '@/lib/api'

import { Wallet } from '..'
import { RechargeFormCard } from '../components/recharge-form-card'
import type { TopupInfo } from '../types'

vi.mock('@/hooks/use-status', () => ({ useStatus: () => ({ status: {} }) }))
vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({ currency: {} }),
}))

const i18n = createInstance()

beforeEach(async () => {
  await i18n.use(initReactI18next).init({
    lng: 'en',
    fallbackLng: false,
    nsSeparator: false,
    resources: { en },
    interpolation: { escapeValue: false },
  })
})

function renderRechargeForm(topupInfo: TopupInfo) {
  render(
    <I18nextProvider i18n={i18n}>
      <RechargeFormCard
        topupInfo={topupInfo}
        presetAmounts={[{ value: 5 }, { value: 100 }]}
        selectedPreset={null}
        topupAmount={5}
        paymentAmount={5}
        priceRatio={1}
        calculating={false}
        paymentLoading={null}
        redemptionCode=''
        redeeming={false}
        onSelectPreset={vi.fn()}
        onTopupAmountChange={vi.fn()}
        onPaymentMethodSelect={vi.fn()}
        onRedemptionCodeChange={vi.fn()}
        onRedeem={vi.fn()}
      />
    </I18nextProvider>
  )
}

it('offers Moyasar when it is the only enabled gateway', () => {
  renderRechargeForm({
    enable_online_topup: false,
    enable_stripe_topup: false,
    enable_moyasar_topup: true,
    moyasar_min_topup: 5,
    pay_methods: [
      {
        name: 'Moyasar TEST MODE (administrators only, test cards)',
        type: 'moyasar',
        min_topup: 5,
      },
    ],
    min_topup: 1,
    stripe_min_topup: 1,
    amount_options: [],
    discount: {},
  })

  expect(
    screen.getByRole('button', { name: /Moyasar TEST MODE/ })
  ).toBeVisible()
  expect(
    screen.queryByText(/Online topup is not enabled/)
  ).not.toBeInTheDocument()
})

it('says online top-up is off when no gateway is enabled', () => {
  renderRechargeForm({
    enable_online_topup: false,
    enable_stripe_topup: false,
    enable_moyasar_topup: false,
    pay_methods: [],
    min_topup: 1,
    stripe_min_topup: 1,
    amount_options: [],
    discount: {},
  })

  expect(screen.getByText(/Online topup is not enabled/)).toBeVisible()
})

it('does not show the referral card in the wallet', async () => {
  // Each endpoint the wallet reads answers with its own empty shape.
  vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    let data: unknown = {}
    if (url.startsWith('/api/subscription')) data = []
    if (url === '/api/user/topup/info') data = { pay_methods: [] }
    if (url === '/api/user/aff') data = 'abcd'
    if (url === '/api/user/self') {
      data = { id: 1, quota: 0, used_quota: 0, aff_quota: 0, aff_count: 0 }
    }
    return { data: { success: true, data } }
  })
  render(
    <I18nextProvider i18n={i18n}>
      <Wallet />
    </I18nextProvider>
  )

  expect(await screen.findByText('Current Balance')).toBeVisible()
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith('/api/user/topup/info')
  )
  expect(screen.queryByText('Referral Program')).not.toBeInTheDocument()
})

it('prices Moyasar presets like the Moyasar charge, with the fee rounded up', () => {
  render(
    <I18nextProvider i18n={i18n}>
      <RechargeFormCard
        topupInfo={{
          enable_online_topup: false,
          enable_stripe_topup: false,
          enable_moyasar_topup: true,
          moyasar_min_topup: 5,
          topup_fee_rate: 0.055,
          pay_methods: [{ name: 'Moyasar', type: 'moyasar', min_topup: 5 }],
          min_topup: 1,
          stripe_min_topup: 1,
          amount_options: [],
          discount: {},
        }}
        presetAmounts={[{ value: 5 }, { value: 25 }, { value: 100 }]}
        selectedPreset={null}
        topupAmount={5}
        paymentAmount={5}
        priceRatio={1.055}
        calculating={false}
        paymentLoading={null}
        redemptionCode=''
        redeeming={false}
        onSelectPreset={vi.fn()}
        onTopupAmountChange={vi.fn()}
        onPaymentMethodSelect={vi.fn()}
        onRedemptionCodeChange={vi.fn()}
        onRedeem={vi.fn()}
      />
    </I18nextProvider>
  )

  // 19.79, 98.91 and 395.63 SAR at 3.75 SAR/USD
  expect(screen.getByRole('button', { name: /^5 / })).toHaveTextContent(
    'Pay 5.28'
  )
  expect(screen.getByRole('button', { name: /^25 / })).toHaveTextContent(
    'Pay 26.38'
  )
  expect(screen.getByRole('button', { name: /^100 / })).toHaveTextContent(
    'Pay 105.5'
  )
})

it('hides the redemption code box', () => {
  renderRechargeForm({
    enable_online_topup: false,
    enable_stripe_topup: false,
    enable_moyasar_topup: true,
    enable_redemption: true,
    pay_methods: [{ name: 'Moyasar', type: 'moyasar', min_topup: 5 }],
    min_topup: 1,
    stripe_min_topup: 1,
    amount_options: [],
    discount: {},
  })

  expect(screen.queryByLabelText('Have a Code?')).not.toBeInTheDocument()
  expect(
    screen.queryByPlaceholderText('Enter your redemption code')
  ).not.toBeInTheDocument()
})
