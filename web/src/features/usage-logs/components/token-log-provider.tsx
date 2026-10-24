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
 * the consume log's `other.provider` by the server (model/token_log_text.go).
 * Renders nothing for logs without one.
 */
export function TokenLogProvider(props: { other: string }) {
  const { t } = useTranslation()
  const provider = parseLogOther(props.other)?.provider
  if (typeof provider !== 'string' || provider === '') return null
  return (
    <span className='text-muted-foreground text-xs'>
      {t('via {{provider}}', { provider })}
    </span>
  )
}
