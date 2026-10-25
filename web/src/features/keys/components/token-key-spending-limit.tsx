/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { useTranslation } from 'react-i18next'

/**
 * TOKEN: what a key's spending limit does, as the relay enforces it
 * (relay/token_output_reservation.go): a total cap for the key on top of the
 * wallet balance; each request reserves its largest possible reply first and
 * its maximum output is lowered to what the remaining limit pays.
 */
export function KeySpendingLimitNote(props: { limited: boolean }) {
  const { t } = useTranslation()
  return (
    <p className='text-muted-foreground text-xs'>
      {props.limited
        ? t(
            'The most this key can spend in total; your wallet balance still applies. Each request first reserves the cost of its longest possible reply: when the remaining limit cannot pay for it, the reply is shortened to fit, and requests are refused once even a short reply no longer fits.'
          )
        : t(
            'Without a limit, this key can spend up to your wallet balance. Set a limit for keys used by scripts, shared tools or other people.'
          )}
    </p>
  )
}
