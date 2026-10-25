/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'

import { KeySpendingLimitNote } from '../token-key-spending-limit'

it('explains how a spending limit caps a key and shortens replies', () => {
  render(<KeySpendingLimitNote limited />)

  expect(screen.getByText(/most this key can spend in total/)).toBeVisible()
  expect(screen.getByText(/reply is shortened to fit/)).toBeVisible()
})

it('says an unlimited key spends up to the wallet balance', () => {
  render(<KeySpendingLimitNote limited={false} />)

  expect(screen.getByText(/up to your wallet balance/)).toBeVisible()
})
