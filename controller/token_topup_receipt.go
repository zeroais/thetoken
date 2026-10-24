package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/stripe/stripe-go/v81"
)

// TOKEN fork: email a receipt after a Stripe top-up is credited. Sent in
// the background; a mail failure never affects the credited payment.
func sendStripeTopupReceipt(ctx context.Context, event stripe.Event, tradeNo string) {
	if common.SMTPServer == "" {
		return
	}
	paidMinor, _ := strconv.ParseFloat(event.GetObjectValue("amount_total"), 64)
	currency := strings.ToUpper(event.GetObjectValue("currency"))
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
			Paid:        paidMinor / 100,
			Currency:    currency,
			Balance:     logger.FormatQuota(balance),
			WalletURL:   paymentReturnPath("/wallet"),
			PaymentNote: "Card via Stripe",
		})
		if err := common.SendEmail(subject, user.Email, html); err != nil {
			logger.LogError(ctx, fmt.Sprintf("top-up receipt email failed trade_no=%s error=%q", tradeNo, err.Error()))
		}
	})
}
