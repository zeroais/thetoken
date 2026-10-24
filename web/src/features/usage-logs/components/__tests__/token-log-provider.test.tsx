/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'

import { TokenLogProvider } from '../token-log-provider'

it('names the provider that served the request', () => {
  render(
    <TokenLogProvider other={JSON.stringify({ provider: 'Provider A' })} />
  )

  expect(screen.getByText('via Provider A')).toBeVisible()
})

it('renders nothing when the log has no provider', () => {
  const { container } = render(
    <TokenLogProvider other={JSON.stringify({ model_ratio: 1 })} />
  )

  expect(container).toBeEmptyDOMElement()
})

it('renders nothing for an empty or malformed other field', () => {
  for (const other of ['', 'not json']) {
    const { container } = render(<TokenLogProvider other={other} />)
    expect(container).toBeEmptyDOMElement()
  }
})
