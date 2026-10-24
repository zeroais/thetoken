/*
Copyright (C) 2026 TOKEN contributors

This file is part of TOKEN, a fork of New API
(https://github.com/QuantumNous/new-api), licensed under the GNU Affero
General Public License v3.0 or later. See LICENSE.
*/
import { ProjectAttribution } from './footer'
import {
  NewApiCredit,
  OperatorCopyright,
  TokenSiteLinks,
} from './token-attribution'

/**
 * Slim footer bar for the signed-in console: the operator's copyright, New
 * API's own copyright line (as in the public footer), the AGPL credit and the
 * site links.
 */
export function ConsoleFooter() {
  return (
    <footer className='border-border/60 flex shrink-0 flex-wrap items-center justify-between gap-x-6 gap-y-1 border-t px-4 py-2 text-sm'>
      <OperatorCopyright />
      <ProjectAttribution currentYear={new Date().getFullYear()} inline />
      <NewApiCredit />
      <TokenSiteLinks />
    </footer>
  )
}
