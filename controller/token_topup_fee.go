package controller

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/price"
)

// TOKEN fork: Stripe charges the unit amount configured on the Price ID, so
// the credit purchase fee is added as its own line item computed from that
// same Price, keeping the receipt explicit ("Credit purchase fee 5.5%").

// stripeTopupFeeLineItem returns the fee line item for buying `quantity`
// units of priceID, or nil when no fee is configured.
func stripeTopupFeeLineItem(priceID string, quantity int64) (*stripe.CheckoutSessionLineItemParams, error) {
	if operation_setting.TopupFeeRate() == 0 {
		return nil, nil
	}
	p, err := price.Get(priceID, nil)
	if err != nil {
		return nil, fmt.Errorf("load Stripe price for top-up fee: %w", err)
	}
	if p.UnitAmount <= 0 || p.UnitAmount > math.MaxInt64/max(quantity, 1) {
		return nil, fmt.Errorf("stripe price %s has no usable unit_amount for the top-up fee", priceID)
	}
	return topupFeeLineItem(string(p.Currency), p.UnitAmount*quantity), nil
}

// topupFeeLineItem builds the fee line item for a top-up costing baseMinor
// minor units of currency before the fee.
func topupFeeLineItem(currency string, baseMinor int64) *stripe.CheckoutSessionLineItemParams {
	fee := operation_setting.TopupFeeMinor(baseMinor)
	if fee <= 0 {
		return nil
	}
	rate := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", operation_setting.TopupFeeRate()*100), "0"), ".")
	return &stripe.CheckoutSessionLineItemParams{
		Quantity: stripe.Int64(1),
		PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
			Currency:   stripe.String(currency),
			UnitAmount: stripe.Int64(fee),
			ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
				Name:        stripe.String("Credit purchase fee (" + rate + "%)"),
				Description: stripe.String("Payment processing fee. Not refundable."),
			},
		},
	}
}
