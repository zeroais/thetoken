/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import * as walletApi from '../../../api'
import { MoyasarResyncButton } from '../token-moyasar-resync'

function signInAs(role: number) {
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { ...(state.auth.user ?? {}), role } as never,
    },
  }))
}

afterEach(() => {
  vi.restoreAllMocks()
  signInAs(0)
})

it('shows nothing to administrators who are not root', () => {
  signInAs(10)
  const { container } = render(
    <MoyasarResyncButton tradeNo='moyasar_inv-1' onDone={vi.fn()} />
  )

  expect(container).toBeEmptyDOMElement()
})

it('shows nothing for orders of other gateways', () => {
  signInAs(100)
  const { container } = render(
    <MoyasarResyncButton tradeNo='stripe_cs_1' onDone={vi.fn()} />
  )

  expect(container).toBeEmptyDOMElement()
})

it('re-syncs a Moyasar order after confirmation and refreshes the list', async () => {
  signInAs(100)
  const resync = vi
    .spyOn(walletApi, 'resyncMoyasarOrder')
    .mockResolvedValue({ success: true, message: 'balance $304.25 -> $299.25' })
  const onDone = vi.fn()
  render(<MoyasarResyncButton tradeNo='moyasar_inv-1' onDone={onDone} />)

  fireEvent.click(screen.getByRole('button', { name: 'Re-sync from Moyasar' }))
  expect(resync).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Re-sync' }))

  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(resync).toHaveBeenCalledWith('moyasar_inv-1')
})
