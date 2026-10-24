/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/

/**
 * TOKEN fork: public brand links shown in the console footer and About page.
 * Empty URLs hide their link. Status lives outside our infrastructure, so it
 * is hosted by Better Stack.
 */
export const TOKEN_BRAND = {
  sourceRepoUrl: 'https://github.com/zeroais/thetoken',
  upstreamRepoUrl: 'https://github.com/QuantumNous/new-api',
  statusUrl: 'https://status.thetoken.dev',
  termsPath: '/user-agreement',
  privacyPath: '/privacy-policy',
  // Chat model shown in code samples while the user's models load, or when
  // their list includes it.
  sampleChatModel: 'deepseek/deepseek-v4.1-flash',
  // TOKEN has no referral programme: the wallet hides New API's referral card.
  referralProgram: false,
  // No redemption codes: the wallet hides New API's "Have a Code?" box.
  redemptionCodes: false,
  // New API keys use this routing group unless the person picks another.
  defaultKeyGroup: 'default',
  // Operator named in the footer copyright (legal pages: same entity).
  operator: 'ZEROAI Establishment',
  // Tools the home page lists: only those with a TOKEN guide, linked to it
  // (paths under the docs link, site/src/content/docs/docs/).
  documentedTools: [
    {
      name: 'Claude Code',
      icon: 'Claude.Color',
      path: 'coding-tools/#claude-code',
    },
    { name: 'Cline', icon: 'Cline', path: 'coding-tools/#cline' },
    { name: 'OpenCode', icon: 'OpenCode', path: 'coding-tools/#opencode' },
    { name: 'OpenAI SDK', icon: 'OpenAI', path: 'openai-sdk/' },
  ],
  docsFallbackUrl: 'https://thetoken.dev/docs/',
} as const

/**
 * TOKEN fork: the release shown in the console. The server version is
 * "<New API base tag>+token.<TOKEN release>-<commit>" (VERSION stamp); people
 * see the TOKEN release and a short commit, never the upstream rc tag.
 */
export function tokenVersionLabel(version: string): string {
  const release = version.split('+token.')[1]
  if (!release) return version
  const match = release.match(/^(v\d+\.\d+\.\d+)-([0-9a-f]{7,40})$/)
  if (!match) return release
  return `${match[1]} (${match[2].slice(0, 7)})`
}

/**
 * TOKEN fork: the public API origin for code samples. The console runs on
 * app.<domain> and the API on api.<domain>; any other origin (local
 * development) serves both.
 */
export function tokenApiOrigin(consoleOrigin: string): string {
  const url = new URL(consoleOrigin)
  if (!url.hostname.startsWith('app.')) return url.origin
  url.hostname = `api.${url.hostname.slice('app.'.length)}`
  return url.origin
}

/**
 * TOKEN fork: the model used in code samples: the TOKEN sample model when the
 * user may use it (or before their list is known), else their first model.
 */
export function tokenSampleModel(models: string[] | undefined): string {
  if (!models?.length || models.includes(TOKEN_BRAND.sampleChatModel)) {
    return TOKEN_BRAND.sampleChatModel
  }
  return models[0]
}
