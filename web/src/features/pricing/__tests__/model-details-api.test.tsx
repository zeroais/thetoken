/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { statusQueryOptions } from '@/lib/status-query'

import { ModelDetailsApi } from '../components/model-details-api'
import type { PricingModel } from '../types'

afterEach(() => cleanup())

function renderApiTab() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // Production: the console's ServerAddress is the app host.
  const status: Record<string, unknown> = {
    server_address: 'https://app.thetoken.dev',
  }
  client.setQueryData(statusQueryOptions.queryKey, status)
  render(
    <QueryClientProvider client={client}>
      <ModelDetailsApi
        model={
          {
            model_name: 'deepseek/deepseek-v4-pro',
            supported_endpoint_types: ['openai'],
          } as PricingModel
        }
        endpointMap={{
          openai: { path: '/v1/chat/completions', method: 'POST' },
        }}
      />
    </QueryClientProvider>
  )
}

describe('model details API tab', () => {
  it('does not show the mock supported parameters or rate limits', () => {
    renderApiTab()
    expect(screen.queryByText('Supported parameters')).not.toBeInTheDocument()
    expect(screen.queryByText('Rate limits')).not.toBeInTheDocument()
    expect(screen.getByText('Authentication')).toBeVisible()
  })

  it('points code samples at the API host, not the console host', async () => {
    renderApiTab()
    expect(
      (
        await screen.findAllByText(
          /https:\/\/api\.thetoken\.dev\/v1\/chat\/completions/
        )
      ).length
    ).toBeGreaterThan(0)
    expect(screen.queryByText(/app\.thetoken\.dev/)).not.toBeInTheDocument()
  })
})
