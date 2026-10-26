package common

import (
	"hash/crc32"
	"strings"
)

// TOKEN: API keys have a distinctive format so they can be recognised when
// leaked (GitHub secret scanning, log scanners): "tkn_" + 43 random
// base62 characters (about 256 bits) + a 6-character base62 CRC32 checksum
// of the random part, like GitHub's own tokens. The checksum lets a scanner
// reject look-alikes offline. Keys made before this format ("sk-" + 48
// characters) keep working: authentication hashes whatever the client sends.
// Keys issued on 2026-10-26 used the prefix "thetoken_" (same body); they
// keep working and are still recognised.
const TokenAPIKeyPrefix = "tkn_"

// TokenAPIKeyPrefixes are every prefix TOKEN has issued keys with, current
// first.
var TokenAPIKeyPrefixes = []string{TokenAPIKeyPrefix, "thetoken_"}

const (
	tokenAPIKeyRandomLength   = 43
	tokenAPIKeyChecksumLength = 6
)

// GenerateTokenAPIKey returns a new API key in TOKEN's format.
func GenerateTokenAPIKey() (string, error) {
	random, err := GenerateRandomCharsKey(tokenAPIKeyRandomLength)
	if err != nil {
		return "", err
	}
	return TokenAPIKeyPrefix + random + tokenAPIKeyChecksum(random), nil
}

// IsTokenAPIKey reports whether s is a well-formed TOKEN API key (one of
// TokenAPIKeyPrefixes, length, alphabet and checksum).
func IsTokenAPIKey(s string) bool {
	_, body, ok := CutTokenAPIKeyPrefix(s)
	if !ok || len(body) != tokenAPIKeyRandomLength+tokenAPIKeyChecksumLength {
		return false
	}
	for _, r := range body {
		if !strings.ContainsRune(keyChars, r) {
			return false
		}
	}
	random, sum := body[:tokenAPIKeyRandomLength], body[tokenAPIKeyRandomLength:]
	return tokenAPIKeyChecksum(random) == sum
}

// CutTokenAPIKeyPrefix splits s into the TOKEN key prefix it starts with
// and the rest.
func CutTokenAPIKeyPrefix(s string) (prefix, body string, ok bool) {
	for _, prefix := range TokenAPIKeyPrefixes {
		if body, ok := strings.CutPrefix(s, prefix); ok {
			return prefix, body, true
		}
	}
	return "", s, false
}

// tokenAPIKeyChecksum is the CRC32 (IEEE) of random in base62, left-padded
// with "0" to 6 characters.
func tokenAPIKeyChecksum(random string) string {
	n := crc32.ChecksumIEEE([]byte(random))
	out := make([]byte, tokenAPIKeyChecksumLength)
	for i := tokenAPIKeyChecksumLength - 1; i >= 0; i-- {
		out[i] = keyChars[n%62]
		n /= 62
	}
	return string(out)
}
