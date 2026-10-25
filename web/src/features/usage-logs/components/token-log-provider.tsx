/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { useTranslation } from 'react-i18next'

import { parseLogOther } from '../lib/format'

/**
 * TOKEN: the provider that served a request ("via Provider A"), recorded in
 * the consume log's `other.provider` by the server (model/token_log_text.go),
 * and any providers that failed before it (`other.failover_from`; the
 * request was retried automatically). Renders nothing for logs without one.
 */
export function TokenLogProvider(props: { other: string }) {
  const { t } = useTranslation()
  const other = parseLogOther(props.other)
  const provider = other?.provider
  if (typeof provider !== 'string' || provider === '') return null
  const failed = Array.isArray(other?.failover_from)
    ? other.failover_from.filter((p): p is string => typeof p === 'string')
    : []
  return (
    <span className='text-muted-foreground text-xs'>
      {failed.length > 0
        ? t('via {{provider}} (after {{failed}} failed)', {
            provider,
            failed: failed.join(', '),
          })
        : t('via {{provider}}', { provider })}
    </span>
  )
}
