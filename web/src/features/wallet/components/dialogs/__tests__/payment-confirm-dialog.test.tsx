/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { PaymentConfirmDialog } from '../payment-confirm-dialog'

function renderDialog(feeRate?: number) {
  render(
    <PaymentConfirmDialog
      open
      onOpenChange={vi.fn()}
      onConfirm={vi.fn()}
      topupAmount={100}
      paymentAmount={feeRate ? 105.5 : 100}
      paymentMethod={{ name: 'Stripe', type: 'stripe' }}
      calculating={false}
      processing={false}
      feeRate={feeRate}
    />
  )
}

it('shows the credit purchase fee and its amount when a fee is configured', () => {
  renderDialog(0.055)

  expect(screen.getByText('Credit purchase fee (5.5%)')).toBeVisible()
  // Money with its symbol, rounded to the cent: fee 105.5 - 105.5 / 1.055 = 5.5.
  expect(screen.getByText('$5.5')).toBeVisible()
  expect(screen.getByText('$105.5')).toBeVisible()
  expect(
    screen.getByText(/credit purchase fee is not refundable/i)
  ).toBeVisible()
})

it('shows no fee row when no fee is configured', () => {
  renderDialog()

  expect(screen.queryByText(/Credit purchase fee/)).not.toBeInTheDocument()
  expect(screen.getAllByText('$100')).toHaveLength(2) // top-up and charge
})

it('shows the SAR charge at the fixed rate when the gateway charges in riyals', () => {
  render(
    <PaymentConfirmDialog
      open
      onOpenChange={vi.fn()}
      onConfirm={vi.fn()}
      topupAmount={10}
      // $10 at 5.5%: 3957 halalas = 39.57 SAR = 10.552 USD (server value)
      paymentAmount={10.552}
      paymentMethod={{ name: 'Moyasar', type: 'moyasar' }}
      calculating={false}
      processing={false}
      feeRate={0.055}
      sarPerUsd={3.75}
    />
  )

  expect(
    screen.getByText('Charged in Saudi riyals: 39.57 SAR (1 USD = 3.75 SAR)')
  ).toBeVisible()
})

it('shows the fee of a $5 Moyasar top-up to the cent', () => {
  render(
    <PaymentConfirmDialog
      open
      onOpenChange={vi.fn()}
      onConfirm={vi.fn()}
      topupAmount={5}
      // $5 at 5.5%: 1979 halalas = 19.79 SAR = 5.277333 USD (server value)
      paymentAmount={5.277333}
      paymentMethod={{ name: 'Moyasar', type: 'moyasar' }}
      calculating={false}
      processing={false}
      feeRate={0.055}
      sarPerUsd={3.75}
    />
  )

  expect(screen.getByText('$0.28')).toBeVisible()
  expect(screen.getByText('$5.28')).toBeVisible()
  expect(screen.queryByText('0.2751')).not.toBeInTheDocument()
})

it('shows no SAR line for gateways that charge in USD', () => {
  renderDialog(0.055)

  expect(screen.queryByText(/Saudi riyals/)).not.toBeInTheDocument()
})
