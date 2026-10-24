/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { t } from 'i18next'

import { createApiKey, rotateApiKey, searchApiKeys } from '@/features/keys/api'
import { API_KEY_STATUS } from '@/features/keys/constants'
import {
  getRememberedTokenKey,
  rememberTokenKey,
} from '@/features/keys/lib/token-key-store'
import {
  requireServerSuccess,
  createServerError,
} from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

// TOKEN: keys are stored hashed and cannot be read back, so chat links use a
// dedicated key. It is created on first use and rotated when this tab does
// not hold it yet (e.g. after a reload); the user's other keys are untouched.
export const CHAT_KEY_NAME = 'Console chat'

export async function fetchActiveChatKey() {
  const result = await searchApiKeys({ keyword: CHAT_KEY_NAME, p: 1, size: 20 })
  if (!result.success) {
    throw createServerError(result, t('Failed to load API keys'))
  }

  const dedicated = (result.data?.items ?? []).find(
    (item) =>
      item.name === CHAT_KEY_NAME && item.status === API_KEY_STATUS.ENABLED
  )
  if (dedicated) {
    const known = getRememberedTokenKey(dedicated.id)
    if (known) return known
    const rotated = await rotateApiKey(dedicated.id)
    if (!rotated.success || !rotated.data?.key) {
      throw createServerError(rotated, t('Failed to load API keys'))
    }
    return rememberTokenKey(dedicated.id, rotated.data.key)
  }

  const created = await createApiKey({
    name: CHAT_KEY_NAME,
    remain_quota: 0,
    expired_time: -1,
    unlimited_quota: true,
    model_limits_enabled: false,
    model_limits: '',
    allow_ips: '',
    group: '',
    auto_groups: [],
    cross_group_retry: false,
  })
  if (!created.success || !created.data?.key) {
    throw createServerError(created, t('Failed to load API keys'))
  }
  return rememberTokenKey(created.data.id, created.data.key)
}

/**
 * Get the currently active API key for chat links
 */
export function useActiveChatKey(enabled: boolean) {
  const userId = useAuthStore((state) => state.auth.user?.id)

  return useQuery({
    queryKey: ['chat-active-key', userId],
    queryFn: async () => requireServerSuccess(await fetchActiveChatKey()),
    enabled: enabled && Boolean(userId),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  })
}
