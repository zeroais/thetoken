/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import i18next from 'i18next'
import { beforeAll, expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import { statusQueryOptions } from '@/lib/status-query'

import { ConsoleFooter } from '../token-console-footer'

beforeAll(() => {
  // The real English strings, so New API's copyright line reads as shipped.
  i18next.addResourceBundle('en', 'translation', en.translation, true, true)
})

async function renderConsoleFooter(status: Record<string, unknown>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(statusQueryOptions.queryKey, status)
  const root = createRootRoute({ component: ConsoleFooter })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return screen.findByRole('navigation', { name: 'Site links' })
}

it('shows the AGPL credit with a visible link to the New API repository', async () => {
  await renderConsoleFooter({})

  expect(
    screen.getByText(
      /Frontend design and development by New API contributors\./
    )
  ).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'github.com/QuantumNous/new-api' })
  ).toHaveAttribute('href', 'https://github.com/QuantumNous/new-api')
})

it('shows the New API copyright line linking the New API repository', async () => {
  await renderConsoleFooter({})

  const line = screen.getByText(
    /All rights reserved\. Designed and developed by the project contributors\./
  )
  expect(line).toBeVisible()
  expect(line).toHaveTextContent(`© ${new Date().getFullYear()} New API.`)
  expect(within(line).getByRole('link', { name: 'New API' })).toHaveAttribute(
    'href',
    'https://github.com/QuantumNous/new-api'
  )
})

it('links Terms, Privacy, Status and the TOKEN source and hides unset Docs', async () => {
  const nav = await renderConsoleFooter({})

  expect(within(nav).getByRole('link', { name: 'Terms' })).toHaveAttribute(
    'href',
    '/user-agreement'
  )
  expect(within(nav).getByRole('link', { name: 'Privacy' })).toHaveAttribute(
    'href',
    '/privacy-policy'
  )
  expect(
    within(nav).getByRole('link', { name: 'Source code' })
  ).toHaveAttribute('href', 'https://github.com/zeroais/token')
  expect(within(nav).getByRole('link', { name: 'Status' })).toHaveAttribute(
    'href',
    'https://status.thetoken.dev'
  )
  expect(
    within(nav).queryByRole('link', { name: 'Docs' })
  ).not.toBeInTheDocument()
})

it('shows the Docs link when the docs link setting is configured', async () => {
  const nav = await renderConsoleFooter({
    docs_link: 'https://docs.example.com',
  })

  expect(within(nav).getByRole('link', { name: 'Docs' })).toHaveAttribute(
    'href',
    'https://docs.example.com'
  )
})

it('shows the operator copyright in the console footer', async () => {
  await renderConsoleFooter({})

  expect(
    screen.getByText(`© ${new Date().getFullYear()} ZEROAI Establishment`)
  ).toBeVisible()
})
