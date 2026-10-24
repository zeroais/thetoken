package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

// setupStripeReversalDB gives each test a fresh SQLite database with a root
// user (the reversal alert goes to root) and a customer who topped up $100.
func setupStripeReversalDB(t *testing.T) (customer *model.User, topUp *model.TopUp) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousSQLite, previousBatch := common.RedisEnabled, common.SQLitePath, common.BatchUpdateEnabled
	previousUnitPrice := setting.StripeUnitPrice
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.RedisEnabled, common.SQLitePath, common.BatchUpdateEnabled = previousRedis, previousSQLite, previousBatch
		setting.StripeUnitPrice = previousUnitPrice
	})
	setting.StripeUnitPrice = 1 // TOKEN: $1 of credit per $1, as TOKEN configures it
	t.Setenv("SQL_DSN", "")
	t.Setenv("LOG_SQL_DSN", "")
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "reversal.db")
	require.NoError(t, model.InitDB())
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	model.LOG_DB = model.DB
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.TopUp{}, &model.Log{}, &model.Option{}))

	require.NoError(t, model.DB.Create(&model.User{Username: "root", AffCode: "aff-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	customer = &model.User{Username: "customer", AffCode: "aff-customer", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(customer).Error)
	topUp = &model.TopUp{UserId: customer.Id, Amount: 100, Money: 100, TradeNo: "ref_" + t.Name(),
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending, CreateTime: time.Now().Unix()}
	require.NoError(t, model.DB.Create(topUp).Error)
	for i := range 2 {
		require.NoError(t, model.DB.Create(&model.Token{UserId: customer.Id, Name: fmt.Sprintf("key%d", i),
			Key: fmt.Sprintf("key%d%s", i, strings.Repeat("x", 40)), Status: common.TokenStatusEnabled}).Error)
	}

	previousLookup := stripeTradeNoForPaymentIntent
	stripeTradeNoForPaymentIntent = func(pi string) (string, error) {
		if pi == "pi_"+t.Name() {
			return topUp.TradeNo, nil
		}
		return "", nil
	}
	t.Cleanup(func() { stripeTradeNoForPaymentIntent = previousLookup })
	stripeReversalClaims.Range(func(k, _ any) bool { stripeReversalClaims.Delete(k); return true })
	return customer, topUp
}

func setBalance(t *testing.T, userID int, usd float64) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("quota", int(usd*common.QuotaPerUnit)).Error)
}

func balanceUSD(t *testing.T, userID int) float64 {
	t.Helper()
	q, err := model.GetUserQuota(userID, true)
	require.NoError(t, err)
	return float64(q) / common.QuotaPerUnit
}

func enabledKeys(t *testing.T, userID int) int64 {
	t.Helper()
	var n int64
	require.NoError(t, model.DB.Model(&model.Token{}).Where("user_id = ? AND status = ?", userID, common.TokenStatusEnabled).Count(&n).Error)
	return n
}

func reversalEvent(t *testing.T, eventType stripe.EventType, id string, amountMinor int64) stripe.Event {
	return stripe.Event{ID: "evt_" + id, Type: eventType, Data: &stripe.EventData{Object: map[string]any{
		"id": id, "payment_intent": "pi_" + t.Name(), "amount": float64(amountMinor), "currency": "usd",
	}}}
}

func TestReversalQuota(t *testing.T) {
	for _, tc := range []struct {
		amountMinor int64
		unitPrice   float64
		credited    int
		want        int
	}{
		{5000, 1, 100 * 500000, 50 * 500000},   // $50 refunded -> $50 credit removed
		{10550, 1, 100 * 500000, 100 * 500000}, // full refund incl. 5.5% fee -> capped at credit
		{0, 1, 100 * 500000, 0},
		{5000, 0, 100 * 500000, 50 * 500000}, // missing unit price defaults to $1
	} {
		assert.Equal(t, tc.want, reversalQuota(tc.amountMinor, tc.unitPrice, tc.credited))
	}
}

func TestRefundAfterCreditWasUsedSuspendsKeys(t *testing.T) {
	customer, _ := setupStripeReversalDB(t)
	setBalance(t, customer.Id, 30) // bought $100, spent $70

	event := reversalEvent(t, stripe.EventTypeRefundCreated, "re_used", 10000)
	require.NoError(t, handleStripeReversal(context.Background(), event))
	assert.InDelta(t, -70, balanceUSD(t, customer.Id), 1e-6, "negative balance is allowed")
	assert.Zero(t, enabledKeys(t, customer.Id), "keys suspended")

	// Stripe redelivers the same refund: nothing more is removed.
	require.NoError(t, handleStripeReversal(context.Background(), event))
	assert.InDelta(t, -70, balanceUSD(t, customer.Id), 1e-6)
}

func TestRefundOfUnusedCreditKeepsKeys(t *testing.T) {
	customer, _ := setupStripeReversalDB(t)
	setBalance(t, customer.Id, 100)

	require.NoError(t, handleStripeReversal(context.Background(), reversalEvent(t, stripe.EventTypeRefundCreated, "re_unused", 4000)))
	assert.InDelta(t, 60, balanceUSD(t, customer.Id), 1e-6)
	assert.Equal(t, int64(2), enabledKeys(t, customer.Id))
}

func TestDisputeAlwaysSuspendsKeys(t *testing.T) {
	customer, _ := setupStripeReversalDB(t)
	setBalance(t, customer.Id, 100)

	require.NoError(t, handleStripeReversal(context.Background(), reversalEvent(t, stripe.EventTypeChargeDisputeCreated, "dp_1", 10550)))
	assert.InDelta(t, 0, balanceUSD(t, customer.Id), 1e-6, "disputed credit removed, capped at the order's credit")
	assert.Zero(t, enabledKeys(t, customer.Id))
}

func TestReversalLookupFailureIsRetried(t *testing.T) {
	customer, topUp := setupStripeReversalDB(t)
	setBalance(t, customer.Id, 100)
	stripeTradeNoForPaymentIntent = func(string) (string, error) { return "", errors.New("stripe unavailable") }

	event := reversalEvent(t, stripe.EventTypeRefundCreated, "re_retry", 1000)
	require.Error(t, handleStripeReversal(context.Background(), event))
	assert.InDelta(t, 100, balanceUSD(t, customer.Id), 1e-6)

	stripeTradeNoForPaymentIntent = func(string) (string, error) { return topUp.TradeNo, nil }
	require.NoError(t, handleStripeReversal(context.Background(), event), "the claim was released, so the retry applies")
	assert.InDelta(t, 90, balanceUSD(t, customer.Id), 1e-6)
}

func TestStripeWebhookCreditsOnceAndRetriesStuckOrders(t *testing.T) {
	customer, topUp := setupStripeReversalDB(t)
	gin.SetMode(gin.TestMode)

	saved := []string{setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId}
	ps := operation_setting.GetPaymentSetting()
	savedCompliance, savedVersion := ps.ComplianceConfirmed, ps.ComplianceTermsVersion
	t.Cleanup(func() {
		setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = saved[0], saved[1], saved[2]
		ps.ComplianceConfirmed, ps.ComplianceTermsVersion = savedCompliance, savedVersion
	})
	setting.StripeApiSecret, setting.StripeWebhookSecret, setting.StripePriceId = "sk_test_x", "whsec_test", "price_x"
	ps.ComplianceConfirmed, ps.ComplianceTermsVersion = true, operation_setting.CurrentComplianceTermsVersion

	send := func(payload string, secret string) int {
		signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: []byte(payload), Secret: secret})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(payload))
		c.Request.Header.Set("Stripe-Signature", signed.Header)
		StripeWebhook(c)
		return w.Code
	}
	completed := fmt.Sprintf(`{"id":"evt_1","object":"event","type":"checkout.session.completed","api_version":"2024-12-18.acacia","data":{"object":{"id":"cs_1","object":"checkout.session","client_reference_id":%q,"customer":"cus_1","status":"complete","payment_status":"paid","amount_total":10550,"currency":"usd"}}}`, topUp.TradeNo)

	assert.Equal(t, http.StatusBadRequest, send(completed, "whsec_wrong"), "bad signature rejected")
	assert.InDelta(t, 0, balanceUSD(t, customer.Id), 1e-6)

	assert.Equal(t, http.StatusOK, send(completed, "whsec_test"))
	assert.InDelta(t, 100, balanceUSD(t, customer.Id), 1e-6, "credit is the top-up amount, not amount + fee")
	assert.Equal(t, http.StatusOK, send(completed, "whsec_test"), "duplicate delivery is acknowledged")
	assert.InDelta(t, 100, balanceUSD(t, customer.Id), 1e-6, "and credits nothing")

	// An order that stays pending after a paid event asks Stripe to retry.
	stuck := &model.TopUp{UserId: customer.Id, Amount: 5, Money: 5, TradeNo: "ref_stuck", PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending}
	require.NoError(t, model.DB.Create(stuck).Error)
	event := stripe.Event{Type: stripe.EventTypeCheckoutSessionCompleted, Data: &stripe.EventData{Object: map[string]any{
		"client_reference_id": "ref_stuck", "payment_status": "paid"}}}
	assert.True(t, stripeOrderNeedsRetry(event))
	event.Data.Object["client_reference_id"] = topUp.TradeNo
	assert.False(t, stripeOrderNeedsRetry(event), "a credited order needs no retry")
}
