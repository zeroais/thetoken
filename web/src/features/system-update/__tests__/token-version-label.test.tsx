/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { SystemUpdateAction } from '../system-update-action'

vi.mock('../use-system-update', () => ({
  useSystemUpdate: () => ({
    currentVersion: 'v1.0.0-rc.42+token.v1.0.0-1ea44a349653',
    shouldNotify: false,
    checking: false,
    snapshot: null,
    release: null,
  }),
}))

afterEach(() => {
  useAuthStore.getState().auth.reset()
})

it("shows administrators TOKEN's release in the header, not New API's rc tag", () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })

  render(<SystemUpdateAction presentation='version' />)

  expect(screen.getByText('v1.0.0 (1ea44a3)')).toBeInTheDocument()
  expect(screen.queryByText(/rc\.42/)).not.toBeInTheDocument()
})
