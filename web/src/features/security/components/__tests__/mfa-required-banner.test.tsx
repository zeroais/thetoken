/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'

import { MFARequiredBanner } from '../mfa-required-banner'

async function renderAt(url: string) {
  const router = createRouter({
    routeTree: createRootRoute({ component: MFARequiredBanner }),
    history: createMemoryHistory({ initialEntries: [url] }),
  })
  render(<RouterProvider router={router} />)
  await router.load()
}

it('explains the requirement when the console redirected an admin to enrol', async () => {
  await renderAt('/security?mfa=required')
  expect(
    await screen.findByText('Two-factor authentication required')
  ).toBeVisible()
})

it('shows nothing on a normal visit to the security page', async () => {
  await renderAt('/security')
  expect(
    screen.queryByText('Two-factor authentication required')
  ).not.toBeInTheDocument()
})
