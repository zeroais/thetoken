/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { HeroTerminalDemo } from '../components/hero-terminal-demo'
import { Features } from '../components/sections/features'
import { Hero } from '../components/sections/hero'
import { Stats } from '../components/sections/stats'

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { docs_link: 'https://thetoken.dev/docs/' } }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { to: string; children?: unknown }) => (
    <a href={props.to}>{props.children as never}</a>
  ),
}))

beforeEach(() => {
  // Reduced motion: the counters show their final values at once.
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

it('shows the model count and TOKEN policies instead of integration counts', () => {
  render(<Stats />)

  for (const value of ['11 models', '5.5%', '30 days', '90 days']) {
    expect(screen.getByText(value)).toBeVisible()
  }
  expect(
    screen.getByText('credit purchase fee, shown at checkout')
  ).toBeVisible()
  expect(
    screen.queryByText('upstream services integrated')
  ).not.toBeInTheDocument()
})

it('shows no Claude tab in the code demo', () => {
  render(<HeroTerminalDemo />)

  expect(screen.getByRole('button', { name: 'Chat' })).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Claude' })
  ).not.toBeInTheDocument()
})

it('names only model families TOKEN sells', () => {
  render(<Features />)

  for (const family of ['DeepSeek', 'Kimi', 'GLM', 'gpt-oss', 'Gemma']) {
    expect(screen.getByText(family)).toBeVisible()
  }
  for (const unsold of ['Claude', 'Gemini', 'Qwen']) {
    expect(screen.queryByText(unsold)).not.toBeInTheDocument()
  }
})

it('lists only tools that have a TOKEN guide, each linking to it', () => {
  render(<Hero isAuthenticated={false} />)

  expect(screen.getByRole('link', { name: 'Claude Code' })).toHaveAttribute(
    'href',
    'https://thetoken.dev/docs/coding-tools/#claude-code'
  )
  expect(screen.getByRole('link', { name: 'OpenAI SDK' })).toHaveAttribute(
    'href',
    'https://thetoken.dev/docs/openai-sdk/'
  )
  for (const name of ['Cline', 'OpenCode']) {
    expect(screen.getByRole('link', { name })).toBeVisible()
  }
  expect(screen.queryByText('Cherry Studio')).not.toBeInTheDocument()
  expect(screen.queryByText('CC Switch')).not.toBeInTheDocument()
})
