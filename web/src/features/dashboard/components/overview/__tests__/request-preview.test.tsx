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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { OverviewDashboard } from '../overview-dashboard'

let client: QueryClient
let userModels: () => Promise<unknown>

beforeEach(() => {
  window.localStorage.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  // A user who has not finished setup sees the guide with the request preview.
  useAuthStore.getState().auth.setUser({ id: 1, username: 'new-user', role: 1 })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    switch (url) {
      case '/api/token/?p=1&size=10':
        return {
          data: {
            success: true,
            data: {
              items: [{ id: 1, name: 'App key', key: 'masked', status: 1 }],
            },
          },
        }
      case '/api/status':
        return { data: { data: { api_info_enabled: false } } }
      case '/api/user/models':
        return userModels()
      case '/api/data/self':
        return { data: { success: true, data: [] } }
      default:
        throw new Error(`Unexpected dashboard request: ${url}`)
    }
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  window.localStorage.clear()
})

async function renderOverview() {
  const router = createRouter({
    routeTree: createRootRoute({ component: OverviewDashboard }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

function modelSignal() {
  return screen.getByText('Model selected').closest('div')?.lastElementChild
}

describe('overview request preview', () => {
  it('shows no models instead of loading forever when the user has none', async () => {
    userModels = async () => ({ data: { success: true, data: [] } })
    await renderOverview()

    expect(await screen.findByText('No models available')).toBeVisible()
    expect(modelSignal()).toHaveTextContent('No models available')
  })

  it('shows loading while the model list is still being fetched', async () => {
    userModels = () => new Promise(() => {})
    await renderOverview()

    await screen.findByText('Model selected')
    expect(modelSignal()).toHaveTextContent('Loading')
  })

  it('uses a TOKEN model in the sample request, never gpt-4o-mini', async () => {
    userModels = async () => ({
      data: {
        success: true,
        data: ['baai/bge-m3', 'deepseek/deepseek-v4.1-flash'],
      },
    })
    await renderOverview()

    expect(
      await screen.findByText('deepseek/deepseek-v4.1-flash')
    ).toBeVisible()
    expect(
      screen.getAllByText(/"model":"deepseek\/deepseek-v4\.1-flash"/).length
    ).toBeGreaterThan(0)
    expect(screen.queryByText(/gpt-4o-mini/)).not.toBeInTheDocument()
  })
})
