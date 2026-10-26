/*
TOKEN: API keys are stored hashed on the server and shown only once, when
they are created or rotated. Keys shown in this tab are kept in memory (never
in browser storage) so copy and integration actions keep working until the
page is reloaded.
*/

const knownKeys = new Map<number, string>()

export const KEY_SHOWN_ONCE_MESSAGE =
  'API keys are shown only once, when they are created. Rotate the key to get a new one.'

/**
 * The key as a client sends it: keys in TOKEN's format ("thetoken_…",
 * common/token_api_key.go) are complete; older keys and the masked hints of
 * older keys get upstream's "sk-" prefix.
 */
export function withSkPrefix(key: string): string {
  return key.startsWith('sk-') || key.startsWith('thetoken_')
    ? key
    : `sk-${key}`
}

export function rememberTokenKey(id: number, key: string): string {
  const full = withSkPrefix(key)
  knownKeys.set(id, full)
  return full
}

export function getRememberedTokenKey(id: number): string | undefined {
  return knownKeys.get(id)
}

export function forgetTokenKeys(): void {
  knownKeys.clear()
}
