package service

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// TOKEN fork: English transactional emails with TOKEN branding, replacing
// upstream's Chinese verification, password-reset and low-balance texts and
// adding top-up receipts. Templates use html/template, so every value is
// escaped. Senders keep using common.SendEmail (SMTP / Amazon SES).

var tokenEmailLayout = template.Must(template.New("layout").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark"><title>{{.Title}}</title></head>
<body style="margin:0;padding:0;background:#f5f6f8;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Inter,Roboto,sans-serif;color:#0B0D12">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f5f6f8;padding:32px 16px">
<tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:12px;border:1px solid #e4e6eb">
<tr><td style="padding:24px 32px 8px;font-size:20px;font-weight:700;letter-spacing:4px">{{.Brand}}</td></tr>
<tr><td style="padding:8px 32px 0;font-size:18px;font-weight:600">{{.Title}}</td></tr>
<tr><td style="padding:12px 32px 28px;font-size:15px;line-height:1.6">{{.Body}}</td></tr>
</table>
<p style="max-width:560px;margin:16px auto 0;font-size:12px;line-height:1.5;color:#5b6170">
You received this email because of activity on your {{.Brand}} account. If this wasn't you, you can ignore it or reply to reach support.
</p>
</td></tr></table></body></html>`))

func renderTokenEmail(title string, body template.HTML) string {
	var buf bytes.Buffer
	_ = tokenEmailLayout.Execute(&buf, map[string]any{"Brand": common.SystemName, "Title": title, "Body": body})
	return buf.String()
}

// renderBody fills a body fragment template; values are escaped.
func renderBody(fragment string, data any) template.HTML {
	var buf bytes.Buffer
	_ = template.Must(template.New("body").Parse(fragment)).Execute(&buf, data) // fixed templates
	return template.HTML(buf.String())
}

// TokenVerificationEmail is the sign-up / email verification code email.
func TokenVerificationEmail(code string, validMinutes int) (subject, html string) {
	subject = fmt.Sprintf("Your %s verification code", common.SystemName)
	body := renderBody(`<p>Use this code to verify your email address:</p>
<p style="font-size:28px;font-weight:700;letter-spacing:6px;margin:16px 0">{{.Code}}</p>
<p>The code expires in {{.Minutes}} minutes. Never share it with anyone.</p>`,
		map[string]any{"Code": code, "Minutes": validMinutes})
	return subject, renderTokenEmail("Verify your email", body)
}

// TokenPasswordResetEmail is the password reset link email.
func TokenPasswordResetEmail(link string, validMinutes int) (subject, html string) {
	subject = fmt.Sprintf("Reset your %s password", common.SystemName)
	body := renderBody(`<p>We received a request to reset your password.</p>
<p style="margin:20px 0"><a href="{{.Link}}" style="background:#3D5CF5;color:#ffffff;text-decoration:none;padding:10px 18px;border-radius:8px;font-weight:600;display:inline-block">Reset password</a></p>
<p>Or paste this link into your browser:<br><span style="word-break:break-all;color:#5b6170">{{.Link}}</span></p>
<p>The link expires in {{.Minutes}} minutes. If you didn't ask for a reset, ignore this email; your password stays the same.</p>`,
		map[string]any{"Link": link, "Minutes": validMinutes})
	return subject, renderTokenEmail("Reset your password", body)
}

// TopupReceipt describes a completed top-up for the receipt email.
type TopupReceipt struct {
	OrderID     string
	PaidAt      time.Time
	Credited    float64 // USD of credit added
	Paid        float64 // amount charged, in Currency
	Currency    string  // ISO code, e.g. "USD"
	Balance     string  // formatted balance after the top-up
	WalletURL   string
	PaymentNote string // e.g. "Card via Stripe"
}

// TokenTopupReceiptEmail is the receipt sent after a successful top-up.
func TokenTopupReceiptEmail(r TopupReceipt) (subject, html string) {
	subject = fmt.Sprintf("%s receipt: $%.2f credit added", common.SystemName, r.Credited)
	fee := r.Paid - r.Credited
	body := renderBody(`<p>Thanks for your purchase. Your credit is ready to use.</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:16px 0;font-size:14px;border-collapse:collapse">
<tr><td style="padding:6px 0;color:#5b6170">Order</td><td align="right" style="padding:6px 0">{{.R.OrderID}}</td></tr>
<tr><td style="padding:6px 0;color:#5b6170">Date</td><td align="right" style="padding:6px 0">{{.Date}}</td></tr>
<tr><td style="padding:6px 0;color:#5b6170">Credit added</td><td align="right" style="padding:6px 0">${{printf "%.2f" .R.Credited}}</td></tr>
{{if .ShowFee}}<tr><td style="padding:6px 0;color:#5b6170">Credit purchase fee</td><td align="right" style="padding:6px 0">${{printf "%.2f" .Fee}}</td></tr>{{end}}
<tr><td style="padding:8px 0;border-top:1px solid #e4e6eb;font-weight:600">Total paid</td><td align="right" style="padding:8px 0;border-top:1px solid #e4e6eb;font-weight:600">{{printf "%.2f" .R.Paid}} {{.R.Currency}}</td></tr>
{{if .R.PaymentNote}}<tr><td style="padding:6px 0;color:#5b6170">Payment</td><td align="right" style="padding:6px 0">{{.R.PaymentNote}}</td></tr>{{end}}
<tr><td style="padding:6px 0;color:#5b6170">Balance now</td><td align="right" style="padding:6px 0">{{.R.Balance}}</td></tr>
</table>
<p>The credit purchase fee is not refundable; unused credit can be refunded minus that fee. <a href="{{.R.WalletURL}}" style="color:#3D5CF5">View your wallet</a></p>`,
		map[string]any{"R": r, "Date": r.PaidAt.UTC().Format("2 Jan 2006 15:04 UTC"), "Fee": fee,
			"ShowFee": r.Currency == "USD" && fee >= 0.005})
	return subject, renderTokenEmail("Payment receipt", body)
}

// TokenLowBalanceNotify returns the low-balance notification in the shape
// NotifyUser expects ({{value}} placeholders filled from values) for the
// user's chosen channel: plain text for Bark/Gotify, HTML for email/webhook.
func TokenLowBalanceNotify(notifyType, balance, walletURL string, subscription bool) (title, content string, values []any) {
	what := "balance"
	if subscription {
		what = "subscription allowance"
	}
	title = fmt.Sprintf("Your %s %s is running low", common.SystemName, what)
	switch notifyType {
	case dto.NotifyTypeBark, dto.NotifyTypeGotify:
		return title, "{{value}}: {{value}} left. Top up to keep your API keys working.", []any{title, balance}
	}
	if notifyType != dto.NotifyTypeEmail {
		return title, "{{value}}: {{value}} left. Top up at <a href='{{value}}'>{{value}}</a>.", []any{title, balance, walletURL, walletURL}
	}
	body := renderBody(`<p>Your {{.What}} is <strong>{{.Balance}}</strong>.</p>
<p>When it reaches zero, API requests are refused. Top up now to avoid interruptions.</p>
<p style="margin:20px 0"><a href="{{.Link}}" style="background:#3D5CF5;color:#ffffff;text-decoration:none;padding:10px 18px;border-radius:8px;font-weight:600;display:inline-block">Add credit</a></p>
<p style="color:#5b6170;font-size:13px">You can change this alert threshold in your account settings.</p>`,
		map[string]any{"What": what, "Balance": balance, "Link": walletURL})
	// No placeholders: the HTML is final (balance and link already escaped).
	return title, renderTokenEmail("Low balance", body), nil
}
