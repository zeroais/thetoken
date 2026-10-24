/*
TOKEN: rotate an API key. The server issues a new key (shown once) and the
old key stops working immediately.
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { handleServerError } from '@/lib/handle-server-error'

import { rotateApiKey } from '../../api'
import { ERROR_MESSAGES } from '../../constants'
import { useApiKeys } from '../api-keys-provider'

export function RotateKeyDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, showIssuedKeys, triggerRefresh } =
    useApiKeys()
  const [isRotating, setIsRotating] = useState(false)

  const handleRotate = async () => {
    if (!currentRow) return
    setIsRotating(true)
    try {
      const result = await rotateApiKey(currentRow.id)
      if (result.success && result.data?.key) {
        triggerRefresh()
        showIssuedKeys([
          { id: currentRow.id, name: currentRow.name, key: result.data.key },
        ])
      } else {
        handleServerError(result, t(ERROR_MESSAGES.UNEXPECTED))
      }
    } catch (error) {
      handleServerError(error, t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsRotating(false)
    }
  }

  return (
    <ConfirmDialog
      open={open === 'rotate'}
      onOpenChange={(isOpen) => !isOpen && setOpen(null)}
      title={t('Rotate API key?')}
      desc={t(
        'A new key will be created for {{name}}. The current key stops working immediately; update every app that uses it.',
        { name: currentRow?.name ?? '' }
      )}
      confirmText={t('Rotate key')}
      destructive
      isLoading={isRotating}
      handleConfirm={handleRotate}
    />
  )
}
