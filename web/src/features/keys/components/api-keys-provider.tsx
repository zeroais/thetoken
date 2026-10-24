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
import React, { useState, useCallback, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import useDialogState from '@/hooks/use-dialog'

import {
  KEY_SHOWN_ONCE_MESSAGE,
  getRememberedTokenKey,
  rememberTokenKey,
} from '../lib/token-key-store'
import type { ApiKey, ApiKeysDialogType } from '../types'

export type ShownApiKey = { id: number; name?: string; key: string }

type ApiKeysContextType = {
  open: ApiKeysDialogType | null
  setOpen: (str: ApiKeysDialogType | null) => void
  currentRow: ApiKey | null
  setCurrentRow: React.Dispatch<React.SetStateAction<ApiKey | null>>
  refreshTrigger: number
  triggerRefresh: () => void
  resolvedKey: string
  setResolvedKey: React.Dispatch<React.SetStateAction<string>>
  resolveRealKey: (id: number) => Promise<string | null>
  resolveRealKeysBatch: (ids: number[]) => Promise<Record<number, string>>
  resolvedKeys: Record<number, string>
  loadingKeys: Record<number, boolean>
  copiedKeyId: number | null
  markKeyCopied: (id: number) => void
  shownKeys: ShownApiKey[]
  showIssuedKeys: (keys: ShownApiKey[]) => void
}

const ApiKeysContext = React.createContext<ApiKeysContextType | null>(null)

export function ApiKeysProvider({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation()
  const [open, setOpen] = useDialogState<ApiKeysDialogType>(null)
  const [currentRow, setCurrentRow] = useState<ApiKey | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const [resolvedKey, setResolvedKey] = useState('')

  // TOKEN: keys are shown once (create/rotate) and never fetched again;
  // this holds the keys issued in this tab.
  const [resolvedKeys, setResolvedKeys] = useState<Record<number, string>>({})
  const [shownKeys, setShownKeys] = useState<ShownApiKey[]>([])
  const loadingKeys: Record<number, boolean> = {}

  const [copiedKeyId, setCopiedKeyId] = useState<number | null>(null)
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => {
    return () => clearTimeout(copiedTimerRef.current)
  }, [])

  const markKeyCopied = useCallback((id: number) => {
    setCopiedKeyId(id)
    clearTimeout(copiedTimerRef.current)
    copiedTimerRef.current = setTimeout(() => setCopiedKeyId(null), 2000)
  }, [])

  const triggerRefresh = useCallback(() => {
    setRefreshTrigger((prev) => prev + 1)
  }, [])

  const showIssuedKeys = useCallback(
    (keys: ShownApiKey[]) => {
      if (keys.length === 0) return
      const remembered: Record<number, string> = {}
      const shown = keys.map((k) => {
        const full = rememberTokenKey(k.id, k.key)
        remembered[k.id] = full
        return { ...k, key: full }
      })
      setResolvedKeys((prev) => ({ ...prev, ...remembered }))
      setShownKeys(shown)
      setOpen('show-key')
    },
    [setOpen]
  )

  const resolveRealKey = useCallback(
    async (id: number): Promise<string | null> => {
      const known = resolvedKeys[id] || getRememberedTokenKey(id)
      if (known) return known
      toast.info(t(KEY_SHOWN_ONCE_MESSAGE))
      return null
    },
    [resolvedKeys, t]
  )

  const resolveRealKeysBatch = useCallback(
    async (ids: number[]): Promise<Record<number, string>> => {
      const result: Record<number, string> = {}
      for (const id of ids) {
        const known = resolvedKeys[id] || getRememberedTokenKey(id)
        if (known) result[id] = known
      }
      if (Object.keys(result).length < ids.length) {
        toast.info(t(KEY_SHOWN_ONCE_MESSAGE))
      }
      return result
    },
    [resolvedKeys, t]
  )

  return (
    <ApiKeysContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
        resolvedKey,
        setResolvedKey,
        resolveRealKey,
        resolveRealKeysBatch,
        resolvedKeys,
        loadingKeys,
        copiedKeyId,
        markKeyCopied,
        shownKeys,
        showIssuedKeys,
      }}
    >
      {children}
    </ApiKeysContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useApiKeys = () => {
  const apiKeysContext = React.useContext(ApiKeysContext)

  if (!apiKeysContext) {
    throw new Error('useApiKeys has to be used within <ApiKeysContext>')
  }

  return apiKeysContext
}
