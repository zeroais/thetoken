/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import type { UserProfile } from '@/features/profile/types'
import { statusQueryOptions } from '@/lib/status-query'

import { AccountBindings } from '../account-bindings'

afterEach(() => cleanup())

function renderBindings(email: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // TOKEN enables email binding only; every OAuth provider is off.
  const status: Record<string, unknown> = {
    github_oauth: false,
    discord_oauth: false,
    oidc_enabled: false,
    telegram_oauth: false,
    linuxdo_oauth: false,
    wechat_login: false,
  }
  client.setQueryData(statusQueryOptions.queryKey, status)
  render(
    <QueryClientProvider client={client}>
      <AccountBindings
        profile={{ id: 1, username: 'eid.ops', email } as UserProfile}
        onUpdate={() => {}}
      />
    </QueryClientProvider>
  )
  return screen.findByRole('list', { name: 'Account Bindings' })
}

describe('account bindings', () => {
  it('shows the email label, bound state and the whole address for a bound email', async () => {
    const list = await renderBindings('contact@zeroais.com')
    const card = within(list).getByRole('listitem')

    expect(within(card).getByText('Email')).toBeVisible()
    expect(within(card).getByText('Bound')).toBeVisible()
    const address = within(card).getByText('contact@zeroais.com')
    expect(address).toHaveAttribute('title', 'contact@zeroais.com')
    expect(within(card).getByRole('button', { name: 'Change' })).toBeEnabled()
  })

  it('lets a single binding card span the row instead of a third of it', async () => {
    const list = await renderBindings('contact@zeroais.com')

    expect(within(list).getAllByRole('listitem')).toHaveLength(1)
    expect(list.className).toContain(
      'grid-cols-[repeat(auto-fit,minmax(18rem,1fr))]'
    )
    expect(list.className).not.toContain('lg:grid-cols-3')
  })

  it('shows "Not bound" without a tooltip when no email is bound', async () => {
    const list = await renderBindings('')
    const card = within(list).getByRole('listitem')

    expect(within(card).getByText('Not bound')).not.toHaveAttribute('title')
    expect(within(card).getByRole('button', { name: 'Bind' })).toBeVisible()
  })
})
