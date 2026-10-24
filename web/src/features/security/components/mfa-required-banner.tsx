/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { useLocation } from '@tanstack/react-router'
import { ShieldAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

/** Shown when the console sent an admin here to enrol a second factor. */
export function MFARequiredBanner() {
  const { t } = useTranslation()
  const required = useLocation({
    select: (location) => location.searchStr.includes('mfa=required'),
  })
  if (!required) return null
  return (
    <Alert variant='destructive' className='mb-4'>
      <ShieldAlert aria-hidden='true' />
      <AlertTitle>{t('Two-factor authentication required')}</AlertTitle>
      <AlertDescription>
        {t(
          'Admin accounts must use an authenticator app or a passkey. Set one up below to unlock the console.'
        )}
      </AlertDescription>
    </Alert>
  )
}
