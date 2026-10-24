package types

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatabaseErrorsAreHiddenFromCustomers(t *testing.T) {
	dbErr := NewError(errors.New("FATAL: terminating connection due to administrator command (SQLSTATE 57P01)"), ErrorCodeBadResponse)
	assert.Equal(t, tokenTemporaryFailureMessage, dbErr.ToOpenAIError().Message)
	assert.Equal(t, tokenTemporaryFailureMessage, dbErr.ToClaudeError().Message)

	queryErr := NewError(errors.New("record lookup failed"), ErrorCodeQueryDataError)
	assert.Equal(t, tokenTemporaryFailureMessage, queryErr.ToOpenAIError().Message)

	other := NewError(errors.New("model e2e/x is not available"), ErrorCodeBadResponse)
	assert.Equal(t, "model e2e/x is not available", other.ToOpenAIError().Message)
}
