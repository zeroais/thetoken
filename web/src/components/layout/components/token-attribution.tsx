/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { Link } from '@tanstack/react-router'
import { Fragment } from 'react'
import { useTranslation } from 'react-i18next'

import { useStatus } from '@/hooks/use-status'
import { TOKEN_BRAND } from '@/lib/token-brand'
import { cn } from '@/lib/utils'

const linkClassName =
  'text-foreground underline-offset-4 hover:underline focus-visible:underline'

/**
 * AGPL credit required by the TOKEN fork: readable size, normal contrast, and a
 * visible link to the upstream New API repository. Do not shrink, hide, or
 * color-match it with the background.
 */
export function NewApiCredit(props: { className?: string }) {
  const { t } = useTranslation()
  return (
    <p className={cn('text-muted-foreground text-sm', props.className)}>
      {t('Frontend design and development by New API contributors.')}{' '}
      <a
        href={TOKEN_BRAND.upstreamRepoUrl}
        target='_blank'
        rel='noopener noreferrer'
        className={linkClassName}
      >
        github.com/QuantumNous/new-api
      </a>
    </p>
  )
}

/** The operator's copyright line: "© <year> ZEROAI Establishment". */
export function OperatorCopyright(props: { className?: string }) {
  return (
    <p className={cn('text-muted-foreground text-sm', props.className)}>
      &copy; {new Date().getFullYear()} {TOKEN_BRAND.operator}
    </p>
  )
}

/** Terms, Privacy, Status, Docs and the TOKEN source link. Unset URLs are hidden. */
export function TokenSiteLinks(props: { className?: string }) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const docsUrl = (status?.docs_link as string | undefined)?.trim() ?? ''

  const items = [
    { key: 'terms', label: t('Terms'), to: TOKEN_BRAND.termsPath },
    { key: 'privacy', label: t('Privacy'), to: TOKEN_BRAND.privacyPath },
    { key: 'status', label: t('Status'), href: TOKEN_BRAND.statusUrl },
    { key: 'docs', label: t('Docs'), href: docsUrl },
    {
      key: 'source',
      label: t('Source code'),
      href: TOKEN_BRAND.sourceRepoUrl,
    },
  ].filter((item) => item.to || item.href)

  return (
    <nav
      aria-label={t('Site links')}
      className={cn(
        'text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-sm',
        props.className
      )}
    >
      {items.map((item, index) => (
        <Fragment key={item.key}>
          {index > 0 && <span aria-hidden='true'>·</span>}
          {item.to ? (
            <Link to={item.to} className={linkClassName}>
              {item.label}
            </Link>
          ) : (
            <a
              href={item.href}
              target='_blank'
              rel='noopener noreferrer'
              className={linkClassName}
            >
              {item.label}
            </a>
          )}
        </Fragment>
      ))}
    </nav>
  )
}

/** Credit block for the About page; shown whatever About content is set. */
export function ProjectCredits() {
  return (
    <section className='border-border/60 mx-auto flex max-w-4xl flex-col items-center gap-2 border-t px-4 py-6 text-center'>
      <NewApiCredit />
      <TokenSiteLinks className='justify-center' />
    </section>
  )
}
