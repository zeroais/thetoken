package controller

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// TOKEN: Moyasar wallet top-ups (Saudi Arabia: mada, Visa, Mastercard,
// Apple Pay, STC Pay) through Moyasar Invoices, charged in SAR. Mirrors the
// Stripe gateway: the same credit purchase fee, receipt email and reversal
// handling. Moyasar is authoritative: a callback or webhook only triggers a
// fetch of the invoice with the secret key, and crediting happens only when
// that invoice is paid in full, once per invoice id.

type moyasarPayment struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Refunded  int64  `json:"refunded"`
	InvoiceID string `json:"invoice_id"`
}

type moyasarInvoice struct {
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Amount      int64             `json:"amount"`
	Currency    string            `json:"currency"`
	URL         string            `json:"url"`
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
	Payments    []moyasarPayment  `json:"payments"`
}

var moyasarHTTPClient = &http.Client{Timeout: 20 * time.Second}

// moyasarCall sends one authenticated request (HTTP Basic, secret key as the
// user name) and decodes the JSON response.
func moyasarCall(ctx context.Context, cfg moyasarConfig, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := common.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.apiBase+path, reader)
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.secretKey, "")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := moyasarHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		_ = common.Unmarshal(raw, &apiErr)
		return fmt.Errorf("moyasar %s %s: HTTP %d %s %s", method, path, resp.StatusCode, apiErr.Type, apiErr.Message)
	}
	return common.Unmarshal(raw, out)
}

func fetchMoyasarInvoice(ctx context.Context, cfg moyasarConfig, invoiceID string) (*moyasarInvoice, error) {
	if invoiceID == "" || strings.ContainsAny(invoiceID, "/?#%") {
		return nil, errors.New("invalid invoice id")
	}
	var inv moyasarInvoice
	if err := moyasarCall(ctx, cfg, http.MethodGet, "/invoices/"+url.PathEscape(invoiceID), nil, &inv); err != nil {
		return nil, err
	}
	if inv.ID != invoiceID {
		return nil, fmt.Errorf("moyasar returned invoice %q for %q", inv.ID, invoiceID)
	}
	return &inv, nil
}

// paidInFull reports whether an invoice is paid in SAR with paid payments
// covering its amount.
func (inv *moyasarInvoice) paidInFull() bool {
	if inv.Status != "paid" || !strings.EqualFold(inv.Currency, "SAR") || inv.Amount <= 0 {
		return false
	}
	var paid int64
	for _, p := range inv.Payments {
		if p.Status == "paid" && strings.EqualFold(p.Currency, "SAR") && (p.InvoiceID == "" || p.InvoiceID == inv.ID) {
			paid += p.Amount
		}
	}
	return paid >= inv.Amount
}

func moyasarQuoteForUser(c *gin.Context, amount int64) (moyasarQuote, string, bool) {
	cfg := loadMoyasarConfig()
	if amount < cfg.minTopUp {
		return moyasarQuote{}, fmt.Sprintf("Minimum top-up is %d", cfg.minTopUp), false
	}
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		return moyasarQuote{}, "Failed to load your account", false
	}
	q, ok := quoteMoyasarTopUp(amount, group)
	if !ok {
		return moyasarQuote{}, "Invalid top-up amount", false
	}
	return q, "", true
}

// RequestMoyasarAmount returns the charge as its USD equivalent (6 decimals,
// so the SAR amount at 3.75 is exact) for the confirmation dialog.
func RequestMoyasarAmount(c *gin.Context) {
	var req StripePayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || !isMoyasarTopUpEnabledFor(c) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Moyasar is not available"})
		return
	}
	q, msg, ok := moyasarQuoteForUser(c, req.Amount)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": usdFromHalalas(q.TotalHalalas).StringFixed(6)})
}

// RequestMoyasarPay creates a Moyasar invoice and its pending top-up order,
// and returns the hosted invoice page.
func RequestMoyasarPay(c *gin.Context) {
	ctx := c.Request.Context()
	var req StripePayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.PaymentMethod != model.PaymentMethodMoyasar {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Unsupported payment method"})
		return
	}
	if !isMoyasarTopUpEnabledFor(c) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Moyasar is not available"})
		return
	}
	q, msg, ok := moyasarQuoteForUser(c, req.Amount)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": msg})
		return
	}
	userID := c.GetInt("id")
	creditedQuota, err := common.WalletQuotaFromDecimalStrict(q.CreditUSD.Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if err != nil || model.ValidateTopUpQuotaCapacity(userID, creditedQuota) != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "This top-up would exceed the wallet limit"})
		return
	}

	cfg := loadMoyasarConfig()
	creditUSD := q.CreditUSD.StringFixed(2)
	description := fmt.Sprintf("%s credit: $%s (%s SAR at 3.75 SAR/USD)", common.SystemName, creditUSD, sarString(q.BaseHalalas))
	if q.FeeHalalas > 0 {
		description += fmt.Sprintf(" + credit purchase fee %s SAR", sarString(q.FeeHalalas))
	}
	body := map[string]any{
		"amount":       q.TotalHalalas,
		"currency":     "SAR",
		"description":  description,
		"callback_url": paymentReturnPath("/api/moyasar/callback"),
		"success_url":  paymentReturnPath("/wallet"),
		"back_url":     paymentReturnPath("/wallet"),
		"expired_at":   time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		"metadata": map[string]string{
			"user_id":    fmt.Sprint(userID),
			"credit_usd": creditUSD,
		},
	}
	var inv moyasarInvoice
	if err := moyasarCall(ctx, cfg, http.MethodPost, "/invoices", body, &inv); err != nil || inv.ID == "" || inv.URL == "" {
		if err == nil {
			err = errors.New("invoice without id or url")
		}
		logger.LogError(ctx, fmt.Sprintf("Moyasar invoice creation failed user_id=%d error=%q", userID, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Could not start the payment"})
		return
	}

	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          req.Amount,
		Money:           q.CreditUSD.InexactFloat64(),
		TradeNo:         model.MoyasarTradeNo(inv.ID),
		PaymentMethod:   model.PaymentMethodMoyasar,
		PaymentProvider: model.PaymentProviderMoyasar,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Moyasar order insert failed invoice=%s error=%q", inv.ID, err.Error()))
		// Unpayable without an order: cancel the invoice (best effort).
		var ignored moyasarInvoice
		_ = moyasarCall(ctx, cfg, http.MethodPut, "/invoices/"+url.PathEscape(inv.ID)+"/cancel", nil, &ignored)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Could not start the payment"})
		return
	}
	if err := model.TokenRecordTopUpCharge(topUp.TradeNo, "SAR", q.BaseHalalas, q.FeeHalalas, q.TotalHalalas); err != nil {
		// The order still works; only its itemised history line is missing.
		logger.LogError(ctx, fmt.Sprintf("Moyasar order charge not recorded invoice=%s error=%q", inv.ID, err.Error()))
	}

	logger.LogInfo(ctx, fmt.Sprintf("Moyasar invoice created invoice=%s user_id=%d credit_usd=%s total_sar=%s", inv.ID, userID, creditUSD, sarString(q.TotalHalalas)))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"pay_link": inv.URL}})
}

// settleMoyasarInvoice confirms an invoice with Moyasar and credits its
// order once. retry is true when the caller should answer non-2xx so
// Moyasar redelivers (a transient failure on our side or theirs).
func settleMoyasarInvoice(ctx context.Context, invoiceID, callerIP string) (retry bool) {
	tradeNo := model.MoyasarTradeNo(invoiceID)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderMoyasar {
		logger.LogWarn(ctx, fmt.Sprintf("Moyasar notification for unknown invoice %q", invoiceID))
		return false
	}
	if topUp.Status != common.TopUpStatusPending {
		return false // already credited (or closed): idempotent
	}
	cfg := loadMoyasarConfig()
	inv, err := fetchMoyasarInvoice(ctx, cfg, invoiceID)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Moyasar invoice fetch failed invoice=%s error=%q", invoiceID, err.Error()))
		return true
	}
	if !inv.paidInFull() {
		logger.LogInfo(ctx, fmt.Sprintf("Moyasar invoice %s not paid yet (status %s)", invoiceID, inv.Status))
		return false
	}
	credited, err := model.RechargeMoyasar(tradeNo, callerIP)
	if err != nil {
		return !errors.Is(err, model.ErrTopUpStatusInvalid) && !errors.Is(err, model.ErrPaymentMethodMismatch)
	}
	if credited {
		logger.LogInfo(ctx, fmt.Sprintf("Moyasar top-up credited invoice=%s paid=%s SAR", invoiceID, sarString(inv.Amount)))
		sendMoyasarTopupReceipt(ctx, tradeNo, inv.Amount)
	}
	return false
}

// MoyasarCallback receives the invoice callback_url POST. Its body is not
// authenticated, so only the invoice id is used, and only for an invoice
// that has a pending order here; the invoice itself is fetched from Moyasar.
func MoyasarCallback(c *gin.Context) {
	ctx := c.Request.Context()
	if !loadMoyasarConfig().usable() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err != nil || body.ID == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if settleMoyasarInvoice(ctx, body.ID, c.ClientIP()) {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	c.Status(http.StatusOK)
}

type moyasarWebhookEvent struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	SecretToken string         `json:"secret_token"`
	Live        bool           `json:"live"`
	Data        moyasarPayment `json:"data"`
}

// MoyasarWebhook handles Moyasar webhooks. Authenticity: the secret_token in
// the body must equal MOYASAR_WEBHOOK_SECRET (constant-time compare) and
// the event's live flag must match the key mode. Non-2xx answers make
// Moyasar retry (5 times over about 2 hours).
func MoyasarWebhook(c *gin.Context) {
	ctx := c.Request.Context()
	cfg := loadMoyasarConfig()
	if !cfg.usable() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var event moyasarWebhookEvent
	if err := common.DecodeJson(c.Request.Body, &event); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(event.SecretToken), []byte(cfg.webhookSecret)) != 1 {
		logger.LogWarn(ctx, fmt.Sprintf("Moyasar webhook rejected: bad secret_token client_ip=%s", c.ClientIP()))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if event.Live != strings.HasPrefix(cfg.secretKey, "sk_live_") {
		logger.LogWarn(ctx, fmt.Sprintf("Moyasar webhook %s ignored: live=%v does not match the key mode", event.ID, event.Live))
		c.Status(http.StatusOK)
		return
	}
	logger.LogInfo(ctx, fmt.Sprintf("Moyasar webhook event=%s type=%s payment=%s invoice=%s", event.ID, event.Type, event.Data.ID, event.Data.InvoiceID))

	switch event.Type {
	case "payment_paid", "payment_captured":
		if event.Data.InvoiceID != "" && settleMoyasarInvoice(ctx, event.Data.InvoiceID, c.ClientIP()) {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
	case "payment_refunded":
		if err := handleMoyasarRefund(ctx, cfg, event.Data); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Moyasar refund failed, will retry event=%s error=%q", event.ID, err.Error()))
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
	}
	c.Status(http.StatusOK)
}

// handleMoyasarRefund removes credit for a refunded top-up. Moyasar reports
// the payment's cumulative refunded amount; each distinct cumulative amount
// is applied once (model.ApplyTopUpReversal), so partial refunds add up and
// redeliveries are no-ops. Credit removed is the refunded SAR at 3.75 per
// USD, capped at what the order credited (the fee is not credit).
func handleMoyasarRefund(ctx context.Context, cfg moyasarConfig, data moyasarPayment) error {
	if data.InvoiceID == "" || data.ID == "" {
		return nil // not an invoice top-up
	}
	tradeNo := model.MoyasarTradeNo(data.InvoiceID)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderMoyasar || topUp.Status != common.TopUpStatusSuccess {
		logger.LogWarn(ctx, fmt.Sprintf("Moyasar refund for invoice %s: no credited top-up", data.InvoiceID))
		return nil
	}
	// Trust only Moyasar's own view of the payment.
	inv, err := fetchMoyasarInvoice(ctx, cfg, data.InvoiceID)
	if err != nil {
		return err
	}
	var payment *moyasarPayment
	for i := range inv.Payments {
		if inv.Payments[i].ID == data.ID {
			payment = &inv.Payments[i]
		}
	}
	if payment == nil || payment.Refunded <= 0 || !strings.EqualFold(payment.Currency, "SAR") {
		logger.LogWarn(ctx, fmt.Sprintf("Moyasar refund for payment %s: nothing refunded on the invoice", data.ID))
		return nil
	}
	refunded := min(payment.Refunded, payment.Amount)
	res, err := model.ApplyTopUpReversal(model.PaymentProviderMoyasar,
		fmt.Sprintf("%s:refunded:%d", payment.ID, refunded), "refund", tradeNo, refunded,
		decimal.NewFromInt(1).Div(decimal.NewFromInt(moyasarHalalasPerUSD)))
	if errors.Is(err, model.ErrReversalAlreadyApplied) {
		return nil
	}
	if err != nil {
		return err
	}
	suspended := 0
	if res.Balance < 0 {
		if suspended, err = suspendUserAPIKeys(res.UserId); err != nil {
			return err
		}
	}
	summary := fmt.Sprintf("Moyasar refund on order %s (user %d): %s SAR refunded so far, %s credit removed now, balance %s, %d API keys suspended.",
		tradeNo, res.UserId, sarString(refunded), logger.FormatQuota(res.QuotaRemoved), logger.FormatQuota(res.Balance), suspended)
	model.RecordLog(res.UserId, model.LogTypeManage, summary)
	logger.LogWarn(ctx, summary)
	service.NotifyRootUser(tokenReversalNotifyType, "TOKEN payment refund: action taken", summary)
	return nil
}

// sendMoyasarTopupReceipt emails the receipt for a credited Moyasar top-up
// (background; a mail failure never affects the payment).
func sendMoyasarTopupReceipt(ctx context.Context, tradeNo string, paidHalalas int64) {
	if common.SMTPServer == "" {
		return
	}
	gopool.Go(func() {
		topUp := model.GetTopUpByTradeNo(tradeNo)
		if topUp == nil || topUp.Status != common.TopUpStatusSuccess {
			return
		}
		user, err := model.GetUserById(topUp.UserId, false)
		if err != nil || user.Email == "" {
			return
		}
		balance, _ := model.GetUserQuota(topUp.UserId, true)
		subject, html := service.TokenTopupReceiptEmail(service.TopupReceipt{
			OrderID:     tradeNo,
			PaidAt:      time.Unix(topUp.CompleteTime, 0),
			Credited:    topUp.Money,
			Paid:        float64(paidHalalas) / 100,
			Currency:    "SAR",
			Balance:     logger.FormatQuota(balance),
			WalletURL:   paymentReturnPath("/wallet"),
			PaymentNote: "Card via Moyasar (SAR, 1 USD = 3.75 SAR)",
		})
		if err := common.SendEmail(subject, user.Email, html); err != nil {
			logger.LogError(ctx, fmt.Sprintf("top-up receipt email failed trade_no=%s error=%q", tradeNo, err.Error()))
		}
	})
}

// addMoyasarTopUpInfo adds Moyasar to the wallet's payment methods.
func addMoyasarTopUpInfo(c *gin.Context, data gin.H) {
	enabled := isMoyasarTopUpEnabledFor(c)
	data["enable_moyasar_topup"] = enabled
	if !enabled {
		return
	}
	cfg := loadMoyasarConfig()
	data["moyasar_min_topup"] = cfg.minTopUp
	methods, _ := data["pay_methods"].([]map[string]string)
	out := make([]map[string]string, 0, len(methods)+1)
	out = append(out, methods...)
	name := "Moyasar (mada, cards, Apple Pay)"
	if moyasarTestMode() {
		name = "Moyasar TEST MODE (administrators only, test cards)"
	}
	out = append(out, map[string]string{
		"name":      name,
		"type":      model.PaymentMethodMoyasar,
		"color":     "#0F766E",
		"min_topup": fmt.Sprint(cfg.minTopUp),
	})
	data["pay_methods"] = out
}
