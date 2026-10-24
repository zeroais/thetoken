package service

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestTokenEmailsAreEnglishAndEscaped(t *testing.T) {
	subject, html := TokenVerificationEmail("123456", 10)
	assert.Contains(t, subject, "verification code")
	assert.Contains(t, html, "123456")
	assert.Contains(t, html, "10 minutes")
	assert.False(t, hasCJK(subject+html))

	subject, html = TokenPasswordResetEmail(`https://app.example.com/user/reset?email=a%40b.c&token=x"><script>`, 30)
	assert.Contains(t, subject, "Reset your")
	assert.NotContains(t, html, "<script>", "link is escaped")
	assert.Contains(t, html, "30 minutes")
	assert.False(t, hasCJK(subject+html))
}

func TestTopupReceiptShowsFee(t *testing.T) {
	r := TopupReceipt{OrderID: "ref_1", PaidAt: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC),
		Credited: 100, Paid: 105.5, Currency: "USD", Balance: "$130.00", WalletURL: "https://app.example.com/wallet"}
	subject, html := TokenTopupReceiptEmail(r)
	assert.Contains(t, subject, "$100.00 credit added")
	assert.Contains(t, html, "Credit purchase fee")
	assert.Contains(t, html, "$5.50")
	assert.Contains(t, html, "105.50 USD")
	assert.Contains(t, html, "9 Oct 2026 12:00 UTC")

	r.Paid = 100 // no fee configured
	_, html = TokenTopupReceiptEmail(r)
	assert.NotContains(t, html, "Credit purchase fee</td>")
}

func TestLowBalanceNotifyPerChannel(t *testing.T) {
	title, content, values := TokenLowBalanceNotify(dto.NotifyTypeEmail, "$0.80", "https://app.example.com/wallet", false)
	assert.Contains(t, title, "balance is running low")
	assert.Nil(t, values, "email HTML is final")
	assert.Contains(t, content, "$0.80")
	assert.False(t, strings.Contains(content, "{{value}}"))

	_, content, values = TokenLowBalanceNotify(dto.NotifyTypeBark, "$0.80", "https://app.example.com/wallet", true)
	assert.Equal(t, 2, strings.Count(content, "{{value}}"))
	assert.Len(t, values, 2)
	assert.Contains(t, values[0], "subscription allowance")
	assert.False(t, hasCJK(content))
}
