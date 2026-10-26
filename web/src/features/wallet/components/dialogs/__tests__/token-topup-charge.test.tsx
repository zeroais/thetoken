/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'

import { TopupChargeDetails } from '../token-topup-charge'

it('itemises an order as charged, in SAR with the USD equivalent', () => {
  render(
    <TopupChargeDetails
      creditUSD={5}
      charge={{
        currency: 'SAR',
        mode: 'live',
        base_minor: 1875,
        fee_minor: 104,
        total_minor: 1979,
      }}
    />
  )

  expect(screen.getByText('$5')).toBeVisible()
  expect(screen.getByText('SAR 1.04 ($0.28)')).toBeVisible()
  expect(screen.getByText('SAR 19.79 ($5.28)')).toBeVisible()
})

it('shows the credit, and no bare payment number, for an order without a recorded charge', () => {
  render(<TopupChargeDetails creditUSD={100} />)

  expect(screen.getByText('$100')).toBeVisible()
  expect(
    screen.getByText('Charge details were not recorded for this order.')
  ).toBeVisible()
  expect(screen.queryByText('100')).not.toBeInTheDocument()
})
