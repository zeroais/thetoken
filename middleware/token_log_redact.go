package middleware

import (
	"net/url"
	"strings"
)

// TOKEN fork: query parameters that can carry credentials. Gemini-style
// clients send the API key as ?key=sk-...; the access log must not keep it.
var sensitiveLogQueryParams = []string{"key", "api_key", "apikey", "access_token", "token", "code", "state", "secret"}

// redactLogQuery replaces the values of credential-like query parameters
// in a logged path with "REDACTED". The request itself is not changed.
func redactLogQuery(path string) string {
	base, rawQuery, found := strings.Cut(path, "?")
	if !found || rawQuery == "" {
		return path
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return base + "?REDACTED"
	}
	changed := false
	for name := range values {
		for _, sensitive := range sensitiveLogQueryParams {
			if strings.EqualFold(name, sensitive) {
				values[name] = []string{"REDACTED"}
				changed = true
			}
		}
	}
	if !changed {
		return path
	}
	return base + "?" + values.Encode()
}
