package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAccountEmailRefusesDisposableDomains(t *testing.T) {
	for _, email := range []string{"x@mailinator.com", "X@YOPMAIL.COM", "x@inbox.guerrillamail.com", "x@sharklasers.com"} {
		_, err := ValidateAccountEmail(email)
		require.Error(t, err, email)
		assert.True(t, errors.Is(err, ErrAccountEmailRestricted), "reported like any restricted address")
		assert.Contains(t, err.Error(), "Disposable email addresses", email)
	}
	for _, email := range []string{"x@gmail.com", "x@company.com", "x@notmailinator.com", "x@mailinator.com.example.org"} {
		got, err := ValidateAccountEmail(email)
		require.NoError(t, err, email)
		assert.Equal(t, email, got)
	}
}

func TestDisposableDomainListIsClean(t *testing.T) {
	set := tokenDisposableDomains()
	assert.Greater(t, len(set), 100)
	for _, d := range []string{"gmail.com", "outlook.com", "yahoo.com", "icloud.com", "proton.me", "hotmail.com"} {
		assert.False(t, set[d], d)
	}
}
