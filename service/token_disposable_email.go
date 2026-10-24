package service

import (
	_ "embed"
	"strings"
	"sync"
)

// TOKEN: sign-up and email binding refuse disposable (throwaway) mailboxes,
// so the starter credit cannot be farmed with them. ValidateAccountEmail
// calls tokenDisposableEmailDomain; the list is token_disposable_domains.txt
// (one domain per line, # comments), and subdomains of a listed domain are
// refused too.

//go:embed token_disposable_domains.txt
var tokenDisposableDomainsText string

var tokenDisposableDomains = sync.OnceValue(func() map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(tokenDisposableDomainsText, "\n") {
		if d := strings.ToLower(strings.TrimSpace(line)); d != "" && !strings.HasPrefix(d, "#") {
			set[d] = true
		}
	}
	return set
})

// tokenEmailError is refused like ErrAccountEmailRestricted, with its own message.
type tokenEmailError string

func (e tokenEmailError) Error() string        { return string(e) }
func (e tokenEmailError) Is(target error) bool { return target == ErrAccountEmailRestricted }

var ErrAccountEmailDisposable error = tokenEmailError("Disposable email addresses can't be used. Please sign up with a permanent address.")

func tokenDisposableEmailDomain(domain string) bool {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	set := tokenDisposableDomains()
	for domain != "" {
		if set[domain] {
			return true
		}
		_, rest, ok := strings.Cut(domain, ".")
		if !ok {
			return false
		}
		domain = rest
	}
	return false
}
