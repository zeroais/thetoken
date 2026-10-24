/*
TOKEN: shows newly issued API keys once. The server stores only a hash, so
this is the only time the full key is visible.
*/
import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import type { ShownApiKey } from '../api-keys-provider'

type ShowOnceKeyDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  keys: ShownApiKey[]
}

export function ShowOnceKeyDialog(props: ShowOnceKeyDialogProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState<string | null>(null)

  const copy = async (value: string, id: string) => {
    if (await copyToClipboard(value)) {
      setCopied(id)
      setTimeout(() => setCopied((cur) => (cur === id ? null : cur)), 2000)
    }
  }

  const allKeys = props.keys.map((k) => k.key).join('\n')

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        props.keys.length > 1 ? t('Save your API keys') : t('Save your API key')
      }
      description={t(
        'Copy it now and store it somewhere safe. For your security it will not be shown again. If you lose it, rotate the key to get a new one.'
      )}
      contentClassName='sm:max-w-lg'
      bodyClassName='space-y-3'
      footer={
        <>
          {props.keys.length > 1 && (
            <Button variant='outline' onClick={() => copy(allKeys, 'all')}>
              {copied === 'all' ? t('Copied!') : t('Copy all')}
            </Button>
          )}
          <Button onClick={() => props.onOpenChange(false)}>
            {t('I have saved it')}
          </Button>
        </>
      }
    >
      {props.keys.map((k) => (
        <div key={k.id} className='space-y-1'>
          {k.name && <p className='text-muted-foreground text-xs'>{k.name}</p>}
          <div className='flex items-center gap-2'>
            <input
              readOnly
              value={k.key}
              aria-label={t('API key')}
              onFocus={(e) => e.target.select()}
              className='bg-muted/50 w-full min-w-0 rounded-md border px-3 py-2 font-mono text-xs outline-none'
            />
            <Button
              variant='outline'
              size='icon'
              aria-label={t('Copy API key')}
              onClick={() => copy(k.key, String(k.id))}
            >
              {copied === String(k.id) ? (
                <Check className='size-4 text-green-600' />
              ) : (
                <Copy className='size-4' />
              )}
            </Button>
          </div>
        </div>
      ))}
    </Dialog>
  )
}
