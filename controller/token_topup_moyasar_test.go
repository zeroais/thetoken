package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeMoyasar is an in-memory Moyasar API (test mode): invoices with
// payments, Basic auth with the secret key, and switches for failures.
type fakeMoyasar struct {
	t        *testing.T
	secret   string
	mu       sync.Mutex
	invoices map[string]*moyasarInvoice
	created  []map[string]any
	fetches  int
	failGet  bool
	next     int
}

func (f *fakeMoyasar) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok || user != f.secret || pass != "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"authentication_error","message":"Invalid authorization credentials"}`))
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case r.Method == http.MethodPost && path == "/invoices":
		var body map[string]any
		require.NoError(f.t, common.DecodeJson(r.Body, &body))
		f.created = append(f.created, body)
		f.next++
		id := fmt.Sprintf("inv-%04d-test", f.next)
		amount := int64(body["amount"].(float64))
		inv := &moyasarInvoice{ID: id, Status: "initiated", Amount: amount, Currency: body["currency"].(string),
			URL: "https://checkout.moyasar.test/invoices/" + id, Description: body["description"].(string)}
		f.invoices[id] = inv
		w.WriteHeader(http.StatusCreated)
		data, _ := common.Marshal(inv)
		_, _ = w.Write(data)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/invoices/"):
		f.fetches++
		if f.failGet {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		inv := f.invoices[strings.TrimPrefix(path, "/invoices/")]
		if inv == nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"type":"invalid_request_error","message":"Object not found"}`))
			return
		}
		data, _ := common.Marshal(inv)
		_, _ = w.Write(data)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// pay marks an invoice paid with one payment of paidHalalas.
func (f *fakeMoyasar) pay(id string, paidHalalas int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv := f.invoices[id]
	inv.Status = "paid"
	inv.Payments = []moyasarPayment{{ID: "pay-" + id, Status: "paid", Amount: paidHalalas, Currency: "SAR", InvoiceID: id}}
}

// refund sets the payment's cumulative refunded amount.
func (f *fakeMoyasar) refund(id string, cumulative int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invoices[id].Payments[0].Refunded = cumulative
}

const moyasarTestWebhookSecret = "whsec_moyasar_test_only"

// setupMoyasar prepares a database (SQLite, or the DSN given), a fake
// Moyasar API, test-mode keys, a confirmed payment compliance and the 5.5%
// credit purchase fee, and a customer with two API keys.
func setupMoyasar(t *testing.T, dsn string) (*fakeMoyasar, *model.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousSQLite, previousBatch := common.RedisEnabled, common.SQLitePath, common.BatchUpdateEnabled
	ps := operation_setting.GetPaymentSetting()
	previousConfirmed, previousTerms := ps.ComplianceConfirmed, ps.ComplianceTermsVersion
	billing := operation_setting.GetTokenBillingSetting()
	previousFee := billing.TopupFeeRate
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousType, previousLogType)
		common.RedisEnabled, common.SQLitePath, common.BatchUpdateEnabled = previousRedis, previousSQLite, previousBatch
		ps.ComplianceConfirmed, ps.ComplianceTermsVersion = previousConfirmed, previousTerms
		billing.TopupFeeRate = previousFee
	})
	ps.ComplianceConfirmed, ps.ComplianceTermsVersion = true, operation_setting.CurrentComplianceTermsVersion
	billing.TopupFeeRate = 0.055

	t.Setenv("SQL_DSN", dsn)
	t.Setenv("LOG_SQL_DSN", "")
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	common.SQLitePath = filepath.Join(t.TempDir(), "moyasar.db")
	require.NoError(t, model.InitDB())
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	model.LOG_DB = model.DB
	common.SetLogDatabaseType(common.MainDatabaseType())
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.TopUp{}, &model.Log{}, &model.Option{}, &model.TokenPaymentReversal{}, &model.TokenTopUpCharge{}))
	for _, table := range []any{&model.TokenTopUpCharge{}, &model.TokenPaymentReversal{}, &model.TopUp{}, &model.Token{}, &model.Log{}, &model.User{}} {
		require.NoError(t, model.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error)
	}

	require.NoError(t, model.DB.Create(&model.User{Username: "root", AffCode: "aff-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	customer := &model.User{Username: "customer", AffCode: "aff-customer", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, model.DB.Create(customer).Error)
	for i := range 2 {
		require.NoError(t, model.DB.Create(&model.Token{UserId: customer.Id, Name: fmt.Sprintf("key%d", i),
			Key: fmt.Sprintf("moyasarkey%d%s", i, strings.Repeat("x", 36)), Status: common.TokenStatusEnabled}).Error)
	}

	fake := &fakeMoyasar{t: t, secret: "sk_test_moyasar_fake", invoices: map[string]*moyasarInvoice{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	t.Setenv("MOYASAR_SECRET_KEY", fake.secret)
	t.Setenv("MOYASAR_PUBLISHABLE_KEY", "pk_test_moyasar_fake")
	t.Setenv("MOYASAR_WEBHOOK_SECRET", moyasarTestWebhookSecret)
	t.Setenv("MOYASAR_API_BASE", srv.URL+"/v1")
	t.Setenv("MOYASAR_MIN_TOPUP", "5")
	return fake, customer
}

func moyasarPost(t *testing.T, handler gin.HandlerFunc, path string, body any, userID int) *httptest.ResponseRecorder {
	t.Helper()
	data, err := common.Marshal(body)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	c.Request.Header.Set("Content-Type", "application/json")
	if userID > 0 {
		c.Set("id", userID)
		c.Set("role", common.RoleAdminUser) // test keys: only administrators may pay
	}
	handler(c)
	return rec
}

// startTopUp requests a $10 top-up and returns the invoice id.
func startTopUp(t *testing.T, fake *fakeMoyasar, userID int) string {
	t.Helper()
	rec := moyasarPost(t, RequestMoyasarPay, "/api/user/moyasar/pay", map[string]any{"amount": 10, "payment_method": "moyasar"}, userID)
	var resp struct {
		Message string `json:"message"`
		Data    struct {
			PayLink string `json:"pay_link"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	require.Equal(t, "success", resp.Message, rec.Body.String())
	id := strings.TrimPrefix(resp.Data.PayLink, "https://checkout.moyasar.test/invoices/")
	require.Contains(t, fake.invoices, id)
	return id
}

func moyasarWebhook(t *testing.T, secret, eventType string, live bool, payment moyasarPayment) int {
	t.Helper()
	return moyasarPost(t, MoyasarWebhook, "/api/moyasar/webhook", map[string]any{
		"id": "evt-" + eventType + "-" + payment.ID, "type": eventType, "created_at": time.Now().Format(time.RFC3339),
		"secret_token": secret, "live": live, "account_name": "TOKEN", "data": payment,
	}, 0).Code
}

func quotaUSD(t *testing.T, userID int) float64 {
	t.Helper()
	q, err := model.GetUserQuota(userID, true)
	require.NoError(t, err)
	return float64(q) / common.QuotaPerUnit
}

func TestMoyasarQuote(t *testing.T) {
	billing := operation_setting.GetTokenBillingSetting()
	previous := billing.TopupFeeRate
	t.Cleanup(func() { billing.TopupFeeRate = previous })

	for _, tc := range []struct {
		name              string
		amount            int64
		fee               float64
		base, feeH, total int64
		usdEquivalent     string
	}{
		// $10 = 37.50 SAR; 5.5% fee = 2.0625 SAR -> 207 halalas (always up).
		{"10 USD with fee", 10, 0.055, 3750, 207, 3957, "10.552000"},
		// 5.5% of 375.00 SAR = 20.625 SAR -> 2063 halalas.
		{"100 USD with fee", 100, 0.055, 37500, 2063, 39563, "105.501333"},
		// A fee that is already whole halalas is not bumped: 2% of 37.50 = 0.75.
		{"10 USD exact fee", 10, 0.02, 3750, 75, 3825, "10.200000"},
		{"10 USD no fee", 10, 0, 3750, 0, 3750, "10.000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing.TopupFeeRate = tc.fee
			q, ok := quoteMoyasarTopUp(tc.amount, "default")
			require.True(t, ok)
			assert.Equal(t, tc.base, q.BaseHalalas)
			assert.Equal(t, tc.feeH, q.FeeHalalas)
			assert.Equal(t, tc.total, q.TotalHalalas)
			assert.Equal(t, tc.usdEquivalent, usdFromHalalas(q.TotalHalalas).StringFixed(6))
			assert.Equal(t, fmt.Sprint(tc.amount), q.CreditUSD.String())
		})
	}
	for _, bad := range []int64{0, -5, 10001} {
		_, ok := quoteMoyasarTopUp(bad, "default")
		assert.False(t, ok, "amount %d", bad)
	}
}

func TestMoyasarKeysMustMatchModeAndEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name              string
		secret, pub, base string
		usable            bool
	}{
		{"test keys on mock", "sk_test_a", "pk_test_a", "http://mock/v1", true},
		{"live keys on Moyasar", "sk_live_a", "pk_live_a", "", true},
		{"live key on a mock endpoint is refused", "sk_live_a", "pk_live_a", "http://mock/v1", false},
		{"mixed modes are refused", "sk_test_a", "pk_live_a", "", false},
		{"not a secret key", "pk_test_a", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MOYASAR_SECRET_KEY", tc.secret)
			t.Setenv("MOYASAR_PUBLISHABLE_KEY", tc.pub)
			t.Setenv("MOYASAR_WEBHOOK_SECRET", "whsec")
			t.Setenv("MOYASAR_API_BASE", tc.base)
			assert.Equal(t, tc.usable, loadMoyasarConfig().usable())
		})
	}
	t.Setenv("MOYASAR_WEBHOOK_SECRET", "")
	t.Setenv("MOYASAR_SECRET_KEY", "sk_test_a")
	assert.False(t, loadMoyasarConfig().usable(), "a webhook secret is required")
}

// moyasarDatabases runs a scenario on SQLite and, when configured, on real
// MySQL and PostgreSQL (TEST_MYSQL_DSN / TEST_POSTGRES_DSN).
func moyasarDatabases(t *testing.T, scenario func(t *testing.T, dsn string)) {
	for _, db := range []struct{ name, dsn string }{
		{"sqlite", ""},
		{"mysql", os.Getenv("TEST_MYSQL_DSN")},
		{"postgres", os.Getenv("TEST_POSTGRES_DSN")},
	} {
		t.Run(db.name, func(t *testing.T) {
			if db.name != "sqlite" && db.dsn == "" {
				t.Skip("set TEST_" + strings.ToUpper(map[string]string{"mysql": "MYSQL", "postgres": "POSTGRES"}[db.name]) + "_DSN")
			}
			scenario(t, db.dsn)
		})
	}
}

func TestMoyasarTopUpCreditsOncePerInvoice(t *testing.T) {
	moyasarDatabases(t, func(t *testing.T, dsn string) {
		fake, customer := setupMoyasar(t, dsn)
		id := startTopUp(t, fake, customer.Id)

		// The invoice: 39.57 SAR (fee rounded up), with the fee itemised.
		inv := fake.created[0]
		assert.Equal(t, float64(3957), inv["amount"])
		assert.Equal(t, "SAR", inv["currency"])
		assert.Contains(t, inv["description"], "$10.00 (37.50 SAR at 3.75 SAR/USD) + credit purchase fee 2.07 SAR")
		assert.True(t, strings.HasSuffix(inv["callback_url"].(string), "/api/moyasar/callback"))
		order := model.GetTopUpByTradeNo(model.MoyasarTradeNo(id))
		require.NotNil(t, order)
		assert.Equal(t, common.TopUpStatusPending, order.Status)
		assert.Equal(t, 0.0, quotaUSD(t, customer.Id), "nothing credited before payment")

		// Wrong secret or wrong mode: rejected, nothing credited.
		payment := moyasarPayment{ID: "pay-" + id, Status: "paid", Amount: 3957, Currency: "SAR", InvoiceID: id}
		fake.pay(id, 3957)
		assert.Equal(t, http.StatusUnauthorized, moyasarWebhook(t, "wrong-secret", "payment_paid", false, payment))
		assert.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_paid", true, payment), "a live event is ignored by a test-mode server")
		assert.Equal(t, 0.0, quotaUSD(t, customer.Id))

		// The real event credits $10 (not $10.55) once; redeliveries and the
		// invoice callback do not credit again.
		assert.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_paid", false, payment))
		assert.Equal(t, 10.0, quotaUSD(t, customer.Id))
		assert.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_paid", false, payment))
		assert.Equal(t, http.StatusOK, moyasarPost(t, MoyasarCallback, "/api/moyasar/callback", map[string]any{"id": id, "status": "paid"}, 0).Code)
		assert.Equal(t, 10.0, quotaUSD(t, customer.Id))
		assert.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(model.MoyasarTradeNo(id)).Status)
	})
}

// The order history itemises each Moyasar order as charged: the credit's
// price, the credit purchase fee and the total, in halalas.
func TestMoyasarOrderHistoryIsItemised(t *testing.T) {
	moyasarDatabases(t, func(t *testing.T, dsn string) {
		fake, customer := setupMoyasar(t, dsn)
		startTopUp(t, fake, customer.Id)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/self?p=1&page_size=10", nil)
		c.Set("id", customer.Id)
		GetUserTopUps(c)
		var resp struct {
			Data struct {
				Items []struct {
					Amount   int64                   `json:"amount"`
					Money    float64                 `json:"money"`
					Charge   *model.TokenTopUpCharge `json:"charge"`
					TestMode bool                    `json:"test_mode"`
				} `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
		require.Len(t, resp.Data.Items, 1)
		item := resp.Data.Items[0]
		assert.EqualValues(t, 10, item.Amount)
		require.NotNil(t, item.Charge, rec.Body.String())
		assert.Equal(t, model.TokenTopUpCharge{Currency: "SAR", Mode: "test", BaseMinor: 3750, FeeMinor: 207, TotalMinor: 3957}, *item.Charge,
			"$10 of credit is 37.50 SAR, the 5.5% fee rounds up to 2.07 SAR")
		assert.True(t, item.TestMode, "paid with test keys")

		// An order from before charges were recorded (the 23 October test
		// payments) is marked as a test-mode order.
		require.NoError(t, model.DB.Create(&model.TopUp{UserId: customer.Id, Amount: 100, Money: 100, TradeNo: "moyasar_old-test-order",
			PaymentMethod: model.PaymentMethodMoyasar, PaymentProvider: model.PaymentProviderMoyasar, Status: common.TopUpStatusSuccess, CreateTime: time.Now().Unix()}).Error)
		rec = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/self?p=1&page_size=10", nil)
		c.Set("id", customer.Id)
		GetUserTopUps(c)
		require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
		require.Len(t, resp.Data.Items, 2)
		for _, it := range resp.Data.Items {
			assert.True(t, it.TestMode)
		}
	})
}

func TestMoyasarCallbackTrustsOnlyMoyasar(t *testing.T) {
	fake, customer := setupMoyasar(t, "")
	id := startTopUp(t, fake, customer.Id)

	// The callback body claims "paid" but Moyasar says initiated: no credit.
	assert.Equal(t, http.StatusOK, moyasarPost(t, MoyasarCallback, "/api/moyasar/callback", map[string]any{"id": id, "status": "paid", "amount": 3957}, 0).Code)
	assert.Equal(t, 0.0, quotaUSD(t, customer.Id))

	// Paid for less than the invoice amount: no credit.
	fake.pay(id, 100)
	moyasarPost(t, MoyasarCallback, "/api/moyasar/callback", map[string]any{"id": id}, 0)
	assert.Equal(t, 0.0, quotaUSD(t, customer.Id))

	// Unknown invoice ids never reach Moyasar's API.
	before := fake.fetches
	moyasarPost(t, MoyasarCallback, "/api/moyasar/callback", map[string]any{"id": "inv-unknown"}, 0)
	assert.Equal(t, before, fake.fetches)

	// Paid in full: the callback alone credits it.
	fake.pay(id, 3957)
	assert.Equal(t, http.StatusOK, moyasarPost(t, MoyasarCallback, "/api/moyasar/callback", map[string]any{"id": id}, 0).Code)
	assert.Equal(t, 10.0, quotaUSD(t, customer.Id))
}

func TestMoyasarTransientFailureAsksForRetry(t *testing.T) {
	fake, customer := setupMoyasar(t, "")
	id := startTopUp(t, fake, customer.Id)
	fake.pay(id, 3957)
	payment := moyasarPayment{ID: "pay-" + id, Status: "paid", Amount: 3957, Currency: "SAR", InvoiceID: id}

	fake.failGet = true
	assert.Equal(t, http.StatusServiceUnavailable, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_paid", false, payment), "Moyasar retries non-2xx")
	assert.Equal(t, 0.0, quotaUSD(t, customer.Id))

	fake.failGet = false
	assert.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_paid", false, payment))
	assert.Equal(t, 10.0, quotaUSD(t, customer.Id))
}

func TestMoyasarRefundsRemoveCreditOnce(t *testing.T) {
	moyasarDatabases(t, func(t *testing.T, dsn string) {
		fake, customer := setupMoyasar(t, dsn)
		id := startTopUp(t, fake, customer.Id)
		fake.pay(id, 3957)
		payment := moyasarPayment{ID: "pay-" + id, Status: "paid", Amount: 3957, Currency: "SAR", InvoiceID: id}
		require.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_paid", false, payment))
		require.Equal(t, 10.0, quotaUSD(t, customer.Id))

		// Spend $3, then a partial refund of 18.75 SAR ($5): $5 removed.
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", int(7*common.QuotaPerUnit)).Error)
		fake.refund(id, 1875)
		payment.Status, payment.Refunded = "refunded", 1875
		assert.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_refunded", false, payment))
		assert.Equal(t, 2.0, quotaUSD(t, customer.Id))
		// Redelivery: nothing more.
		assert.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_refunded", false, payment))
		assert.Equal(t, 2.0, quotaUSD(t, customer.Id))
		assertKeysEnabled(t, customer.Id, 2)

		// Refund the rest, fee included (39.57 SAR): removal stops at the $10
		// the order credited; the balance goes negative and keys are suspended.
		fake.refund(id, 3957)
		payment.Refunded = 3957
		assert.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_refunded", false, payment))
		assert.Equal(t, -3.0, quotaUSD(t, customer.Id))
		assertKeysEnabled(t, customer.Id, 0)

		var total int64
		require.NoError(t, model.DB.Model(&model.TokenPaymentReversal{}).Where("trade_no = ?", model.MoyasarTradeNo(id)).
			Select("COALESCE(SUM(quota_removed), 0)").Scan(&total).Error)
		assert.Equal(t, int64(10*common.QuotaPerUnit), total)

		// A forged refund event (wrong secret) changes nothing.
		assert.Equal(t, http.StatusUnauthorized, moyasarWebhook(t, "forged", "payment_refunded", false, payment))
		assert.Equal(t, -3.0, quotaUSD(t, customer.Id))
	})
}

// TOKEN: re-syncing an order from Moyasar applies what Moyasar confirms and
// the notifications missed (a paid invoice, a refund), once each; running it
// again changes nothing, and it refuses orders that are not Moyasar's.
func TestMoyasarResyncAppliesMissedNotificationsOnce(t *testing.T) {
	moyasarDatabases(t, func(t *testing.T, dsn string) {
		fake, customer := setupMoyasar(t, dsn)
		id := startTopUp(t, fake, customer.Id)
		resync := func(tradeNo string) (bool, string) {
			rec := moyasarPost(t, MoyasarResync, "/api/user/moyasar/resync", map[string]any{"trade_no": tradeNo}, 1)
			var resp struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
			return resp.Success, resp.Message
		}

		// Not paid yet: nothing to apply.
		ok, _ := resync(model.MoyasarTradeNo(id))
		require.True(t, ok)
		assert.Equal(t, 0.0, quotaUSD(t, customer.Id))

		// Paid, but the webhook never arrived: credited once.
		fake.pay(id, 3957)
		ok, msg := resync(model.MoyasarTradeNo(id))
		require.True(t, ok, msg)
		assert.Equal(t, 10.0, quotaUSD(t, customer.Id))
		ok, msg = resync(model.MoyasarTradeNo(id))
		require.True(t, ok)
		assert.Contains(t, msg, "already up to date")
		assert.Equal(t, 10.0, quotaUSD(t, customer.Id))

		// Refunded in full, the webhook missed: the credit ($10) is removed once.
		fake.refund(id, 3957)
		ok, msg = resync(model.MoyasarTradeNo(id))
		require.True(t, ok, msg)
		assert.Equal(t, 0.0, quotaUSD(t, customer.Id))
		ok, _ = resync(model.MoyasarTradeNo(id))
		require.True(t, ok)
		assert.Equal(t, 0.0, quotaUSD(t, customer.Id))

		ok, msg = resync("stripe_cs_123")
		assert.False(t, ok)
		assert.Contains(t, msg, "not a Moyasar order")
		ok, _ = resync("moyasar_no-such-invoice")
		assert.False(t, ok)
	})
}

// TOKEN: a chargeback recorded by the root user removes the order's credit
// once per reference (never more than it credited) and suspends the keys when
// the balance goes negative; unknown invoices change nothing.
func TestMoyasarChargebackRemovesCreditOnce(t *testing.T) {
	moyasarDatabases(t, func(t *testing.T, dsn string) {
		fake, customer := setupMoyasar(t, dsn)
		id := startTopUp(t, fake, customer.Id)
		fake.pay(id, 3957)
		payment := moyasarPayment{ID: "pay-" + id, Status: "paid", Amount: 3957, Currency: "SAR", InvoiceID: id}
		require.Equal(t, http.StatusOK, moyasarWebhook(t, moyasarTestWebhookSecret, "payment_paid", false, payment))
		// $6 of the $10 already spent.
		require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", customer.Id).Update("quota", int(4*common.QuotaPerUnit)).Error)

		chargeback := func(body map[string]any) (bool, string) {
			rec := moyasarPost(t, MoyasarChargeback, "/api/user/moyasar/chargeback", body, 0)
			var resp struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
			return resp.Success, resp.Message
		}
		ok, msg := chargeback(map[string]any{"invoice_id": "inv-unknown", "reference": "dp-1"})
		assert.False(t, ok, msg)
		assert.Equal(t, 4.0, quotaUSD(t, customer.Id))

		ok, msg = chargeback(map[string]any{"invoice_id": id, "reference": "dp-1"})
		require.True(t, ok, msg)
		assert.Equal(t, -6.0, quotaUSD(t, customer.Id), "the whole $10 credit is removed")
		assertKeysEnabled(t, customer.Id, 0)

		ok, msg = chargeback(map[string]any{"invoice_id": id, "reference": "dp-1"})
		assert.False(t, ok)
		assert.Contains(t, msg, "already recorded")
		ok, _ = chargeback(map[string]any{"invoice_id": id, "reference": "dp-2"})
		require.True(t, ok)
		assert.Equal(t, -6.0, quotaUSD(t, customer.Id), "never more than the order credited")
	})
}

func assertKeysEnabled(t *testing.T, userID int, want int64) {
	t.Helper()
	var n int64
	require.NoError(t, model.DB.Model(&model.Token{}).Where("user_id = ? AND status = ?", userID, common.TokenStatusEnabled).Count(&n).Error)
	assert.Equal(t, want, n)
}

func TestMoyasarTopUpInfoListsTheGateway(t *testing.T) {
	_, _ = setupMoyasar(t, "")
	data := gin.H{"pay_methods": []map[string]string{{"type": "stripe", "name": "Stripe"}}}
	admin, _ := gin.CreateTestContext(httptest.NewRecorder())
	admin.Set("role", common.RoleAdminUser)
	addMoyasarTopUpInfo(admin, data)
	assert.Equal(t, true, data["enable_moyasar_topup"])
	assert.Equal(t, int64(5), data["moyasar_min_topup"])
	methods := data["pay_methods"].([]map[string]string)
	require.Len(t, methods, 2)
	assert.Equal(t, "stripe", methods[0]["type"], "existing gateways untouched")
	assert.Equal(t, "moyasar", methods[1]["type"])

	assert.Contains(t, methods[1]["name"], "TEST MODE")

	t.Setenv("MOYASAR_SECRET_KEY", "")
	data = gin.H{"pay_methods": []map[string]string{}}
	addMoyasarTopUpInfo(admin, data)
	assert.Equal(t, false, data["enable_moyasar_topup"])
	assert.Empty(t, data["pay_methods"])
}

func TestMoyasarTestKeysAreForAdministratorsOnly(t *testing.T) {
	moyasarDatabases(t, func(t *testing.T, dsn string) {
		fake, customer := setupMoyasar(t, dsn)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/user/moyasar/pay", strings.NewReader(`{"amount":10,"payment_method":"moyasar"}`))
		c.Set("id", customer.Id)
		c.Set("role", common.RoleCommonUser)
		RequestMoyasarPay(c)
		assert.Contains(t, rec.Body.String(), "Moyasar is not available")
		assert.Empty(t, fake.created, "no invoice for a customer while the keys are test keys")

		data := gin.H{"pay_methods": []map[string]string{}}
		addMoyasarTopUpInfo(c, data)
		assert.Equal(t, false, data["enable_moyasar_topup"])
		assert.Empty(t, data["pay_methods"])
	})
}

// TestMoyasarRealTestMode talks to Moyasar's real API in test mode. It runs
// only with MOYASAR_LIVE_TEST=1 and a sk_test_ key, never in CI:
//
//	MOYASAR_LIVE_TEST=1 MOYASAR_SECRET_KEY=sk_test_... MOYASAR_WEBHOOK_SECRET=x \
//	  go test ./controller/ -run TestMoyasarRealTestMode -v
//
// It creates a 39.57 SAR invoice, reads it back unpaid, and cancels it. Paying
// it (test card 4111 1111 1111 1111) on the returned page, then running the
// webhook against a public URL, completes the end-to-end check by hand.
func TestMoyasarRealTestMode(t *testing.T) {
	if os.Getenv("MOYASAR_LIVE_TEST") != "1" || !strings.HasPrefix(os.Getenv("MOYASAR_SECRET_KEY"), "sk_test_") {
		t.Skip("set MOYASAR_LIVE_TEST=1 and a sk_test_ MOYASAR_SECRET_KEY")
	}
	t.Setenv("MOYASAR_API_BASE", "")
	cfg := loadMoyasarConfig()
	require.True(t, cfg.usable(), "test keys and a webhook secret are required")
	ctx := t.Context()

	var inv moyasarInvoice
	require.NoError(t, moyasarCall(ctx, cfg, http.MethodPost, "/invoices", map[string]any{
		"amount": 3957, "currency": "SAR", "description": "TOKEN test-mode invoice (automated check)",
		"expired_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		"metadata":   map[string]string{"purpose": "token-test"},
	}, &inv))
	require.NotEmpty(t, inv.ID)
	assert.NotEmpty(t, inv.URL)
	t.Logf("test invoice %s: %s", inv.ID, inv.URL)

	got, err := fetchMoyasarInvoice(ctx, cfg, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3957), got.Amount)
	assert.Equal(t, "SAR", strings.ToUpper(got.Currency))
	assert.False(t, got.paidInFull(), "a new invoice is not paid")

	var canceled moyasarInvoice
	require.NoError(t, moyasarCall(ctx, cfg, http.MethodPut, "/invoices/"+inv.ID+"/cancel", nil, &canceled))
	assert.Equal(t, "canceled", canceled.Status)
}
