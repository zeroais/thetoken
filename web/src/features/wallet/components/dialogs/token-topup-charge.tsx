/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { formatCurrencyFromUSD } from '@/lib/currency'

import { MOYASAR_SAR_PER_USD } from '../../constants'
import type { TopupCharge } from '../../types'

interface TopupChargeDetailsProps {
  /** Credit added to the wallet, in USD */
  creditUSD: number
  charge: TopupCharge
}

const money = (usd: number) =>
  formatCurrencyFromUSD(usd, {
    digitsLarge: 2,
    digitsSmall: 2,
    abbreviate: false,
  })

/**
 * TOKEN: one order of the billing history, itemised as it was charged: the
 * credit added, the credit purchase fee and the total paid, in the charge's
 * currency with the USD equivalent for SAR (fixed 3.75 SAR per USD).
 */
export function TopupChargeDetails(props: TopupChargeDetailsProps) {
  const { t } = useTranslation()
  const amount = (minor: number) => {
    const major = (minor / 100).toFixed(2)
    if (props.charge.currency !== 'SAR')
      return `${props.charge.currency} ${major}`
    return `SAR ${major} (${money(minor / 100 / MOYASAR_SAR_PER_USD)})`
  }
  return (
    <>
      <div className='space-y-1'>
        <Label className='text-muted-foreground text-xs'>
          {t('Credit added')}
        </Label>
        <div className='text-sm font-semibold'>{money(props.creditUSD)}</div>
      </div>
      <div className='space-y-1'>
        <Label className='text-muted-foreground text-xs'>
          {t('Credit purchase fee')}
        </Label>
        <div className='text-sm font-medium'>
          {amount(props.charge.fee_minor)}
        </div>
      </div>
      <div className='space-y-1'>
        <Label className='text-muted-foreground text-xs'>
          {t('Total paid')}
        </Label>
        <div className='text-sm font-semibold'>
          {amount(props.charge.total_minor)}
        </div>
      </div>
    </>
  )
}
