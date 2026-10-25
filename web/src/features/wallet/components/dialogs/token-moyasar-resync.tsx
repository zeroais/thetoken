/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { resyncMoyasarOrder } from '../../api'

interface MoyasarResyncButtonProps {
  tradeNo: string
  onDone: () => void
}

/**
 * TOKEN: "Re-sync from Moyasar" on a Moyasar order, for the root account
 * only (controller/token_moyasar_resync.go): applies what Moyasar confirms
 * and the notifications missed, a payment or refunds, once each.
 */
export function MoyasarResyncButton(props: MoyasarResyncButtonProps) {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  if (role < ROLE.SUPER_ADMIN || !props.tradeNo.startsWith('moyasar_')) {
    return null
  }

  const handleConfirm = async () => {
    setBusy(true)
    try {
      const res = await resyncMoyasarOrder(props.tradeNo)
      if (res.success) {
        toast.success(res.message || t('Order re-synced from Moyasar'))
        setOpen(false)
        props.onDone()
      } else {
        toast.error(res.message || t('Re-sync failed'))
      }
    } catch {
      toast.error(t('Re-sync failed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Button
        size='sm'
        variant='outline'
        onClick={() => setOpen(true)}
        disabled={busy}
      >
        {t('Re-sync from Moyasar')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t('Re-sync this order from Moyasar?')}
        desc={t(
          'TOKEN reads the invoice from Moyasar and applies what it confirms but was missed: a payment is credited, a refund removes its credit. Each is applied once; running this again changes nothing.'
        )}
        confirmText={t('Re-sync')}
        handleConfirm={handleConfirm}
        isLoading={busy}
      />
    </>
  )
}
