package controller

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
)

// TOKEN fork: Stripe refunds and chargebacks. Upstream only handles
// checkout events, so a refunded or disputed top-up would keep its credit.
//
// On refund.created and charge.dispute.created TOKEN removes the matching
// credit dollar for dollar (capped at what the order credited), allowing a
// negative balance. A dispute, or any reversal that leaves the balance
// negative (credit already spent), suspends all of the user's API keys.
// The root user is notified and a log entry is written either way.

const tokenReversalNotifyType = "token_payment_reversal"

// stripeTradeNoForPaymentIntent finds the TOKEN order (Checkout Session
// client_reference_id) a payment intent paid for. Replaced in tests.
var stripeTradeNoForPaymentIntent = func(paymentIntentID string) (string, error) {
	stripe.Key = setting.StripeApiSecret
	params := &stripe.CheckoutSessionListParams{PaymentIntent: stripe.String(paymentIntentID)}
	params.Limit = stripe.Int64(1)
	iter := session.List(params)
	for iter.Next() {
		return iter.CheckoutSession().ClientReferenceID, nil
	}
	if err := iter.Err(); err != nil {
		return "", err
	}
	return "", nil
}

// stripeReversalClaims makes each refund/dispute apply once per node; with
// Redis it is once across the cluster.
var stripeReversalClaims sync.Map

func claimStripeReversal(id string) bool {
	if common.RedisEnabled {
		ok, err := common.RDB.SetNX(context.Background(), "token:stripe:reversal:"+id, 1, 90*24*time.Hour).Result()
		if err == nil {
			return ok
		}
		common.SysError("stripe reversal claim via redis failed, using local claim: " + err.Error())
	}
	_, loaded := stripeReversalClaims.LoadOrStore(id, true)
	return !loaded
}

func releaseStripeReversal(id string) {
	if common.RedisEnabled {
		_ = common.RDB.Del(context.Background(), "token:stripe:reversal:"+id).Err()
	}
	stripeReversalClaims.Delete(id)
}

// reversalQuota is the credit to remove for a reversal of amountMinor
// (cents) when one Stripe unit of unitPrice buys $1 of credit, capped at
// the quota the order credited.
func reversalQuota(amountMinor int64, unitPrice float64, creditedQuota int) int {
	if amountMinor <= 0 || creditedQuota <= 0 {
		return 0
	}
	if unitPrice <= 0 || math.IsNaN(unitPrice) {
		unitPrice = 1
	}
	credit := float64(amountMinor) / 100 / unitPrice * common.QuotaPerUnit
	return min(common.QuotaFromFloat(credit), creditedQuota)
}

// handleStripeReversal processes refund.created and charge.dispute.created.
// A non-nil error asks Stripe to retry the event.
func handleStripeReversal(ctx context.Context, event stripe.Event) error {
	kind := "refund"
	if event.Type == stripe.EventTypeChargeDisputeCreated {
		kind = "dispute"
	}
	id := event.GetObjectValue("id")
	paymentIntent := event.GetObjectValue("payment_intent")
	amount, _ := strconv.ParseInt(event.GetObjectValue("amount"), 10, 64)
	if id == "" || paymentIntent == "" || amount <= 0 {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe %s ignored: missing id, payment_intent or amount (event %s)", kind, event.ID))
		return nil
	}
	if !claimStripeReversal(id) {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe %s %s already processed", kind, id))
		return nil
	}

	tradeNo, err := stripeTradeNoForPaymentIntent(paymentIntent)
	if err != nil {
		releaseStripeReversal(id)
		return fmt.Errorf("look up order for payment intent %s: %w", paymentIntent, err)
	}
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if tradeNo == "" || topUp == nil || topUp.PaymentProvider != model.PaymentProviderStripe {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe %s %s: no TOKEN top-up for payment intent %s", kind, id, paymentIntent))
		return nil
	}

	credited, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if err != nil {
		credited = 0
	}
	remove := reversalQuota(amount, setting.StripeUnitPrice, credited)
	if remove > 0 {
		if err := model.DecreaseUserQuota(topUp.UserId, remove, true); err != nil {
			releaseStripeReversal(id)
			return fmt.Errorf("remove credit for %s %s: %w", kind, id, err)
		}
	}
	balance, err := model.GetUserQuota(topUp.UserId, true)
	if err != nil {
		releaseStripeReversal(id)
		return fmt.Errorf("read balance after %s %s: %w", kind, id, err)
	}

	suspended := 0
	if kind == "dispute" || balance < 0 {
		if suspended, err = suspendUserAPIKeys(topUp.UserId); err != nil {
			return fmt.Errorf("suspend API keys after %s %s: %w", kind, id, err)
		}
	}

	currency := event.GetObjectValue("currency")
	summary := fmt.Sprintf("Stripe %s %s on order %s (user %d): %.2f %s reversed, %s credit removed, balance now %s, %d API keys suspended.",
		kind, id, tradeNo, topUp.UserId, float64(amount)/100, currency,
		logger.FormatQuota(remove), logger.FormatQuota(balance), suspended)
	model.RecordLog(topUp.UserId, model.LogTypeManage, summary)
	logger.LogWarn(ctx, summary)
	service.NotifyRootUser(tokenReversalNotifyType, "TOKEN payment "+kind+": action taken", summary)
	return nil
}

// suspendUserAPIKeys disables every enabled API key of a user and drops
// them from the cache so the next request is refused immediately.
func suspendUserAPIKeys(userID int) (int, error) {
	res := model.DB.Model(&model.Token{}).
		Where("user_id = ? AND status = ?", userID, common.TokenStatusEnabled).
		Update("status", common.TokenStatusDisabled)
	if res.Error != nil {
		return 0, res.Error
	}
	if err := model.InvalidateUserTokensCache(userID); err != nil {
		return int(res.RowsAffected), err
	}
	return int(res.RowsAffected), nil
}

// stripeOrderNeedsRetry reports whether a paid checkout event left its
// top-up order pending (crediting failed, e.g. a database error). Upstream
// always answers 200, so Stripe would never redeliver and the payment would
// stay uncredited; answering 500 makes Stripe retry.
func stripeOrderNeedsRetry(event stripe.Event) bool {
	if event.Type == stripe.EventTypeCheckoutSessionCompleted && event.GetObjectValue("payment_status") != "paid" {
		return false
	}
	topUp := model.GetTopUpByTradeNo(event.GetObjectValue("client_reference_id"))
	return topUp != nil && topUp.PaymentProvider == model.PaymentProviderStripe && topUp.Status == common.TopUpStatusPending
}

// withStripeReceipt asks Stripe to email its payment receipt for a top-up.
func withStripeReceipt(params *stripe.CheckoutSessionParams, email string) {
	if email == "" {
		return
	}
	if params.PaymentIntentData == nil {
		params.PaymentIntentData = &stripe.CheckoutSessionPaymentIntentDataParams{}
	}
	params.PaymentIntentData.ReceiptEmail = stripe.String(email)
}
