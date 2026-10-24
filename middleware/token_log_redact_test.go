package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactLogQuery(t *testing.T) {
	for in, want := range map[string]string{
		"/v1beta/models/gemini:generateContent?key=sk-secret123": "/v1beta/models/gemini:generateContent?key=REDACTED",
		"/v1/models?alt=sse&KEY=sk-abc":                          "/v1/models?KEY=REDACTED&alt=sse",
		"/api/status":                                            "/api/status",
		"/api/log/?p=1&page_size=10":                             "/api/log/?p=1&page_size=10",
		"/x?access_token=abc&token=def":                          "/x?access_token=REDACTED&token=REDACTED",
		"/x?%zz":                                                 "/x?REDACTED",
	} {
		assert.Equal(t, want, redactLogQuery(in), in)
	}
}
