/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, it } from 'vitest'

import { statusQueryOptions } from '@/lib/status-query'

import { useTopNavLinks } from '../use-top-nav-links'

function navLinksFor(status: Record<string, unknown>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(statusQueryOptions.queryKey, status)
  const wrapper = (props: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
  return renderHook(() => useTopNavLinks(), { wrapper }).result.current
}

it('omits the Docs link when no docs link is configured', () => {
  const links = navLinksFor({ docs_link: '' })

  expect(links.map((link) => link.title)).not.toContain('Docs')
})

it('links Docs externally to the configured docs link', () => {
  const links = navLinksFor({ docs_link: 'https://docs.example.com' })

  expect(links.find((link) => link.title === 'Docs')).toEqual({
    title: 'Docs',
    href: 'https://docs.example.com',
    external: true,
  })
})
