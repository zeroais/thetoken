package controller

import (
	"errors"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/form"
)

// TOKEN: TOKEN_STRIPE_API_BASE points the Stripe client at a fake Stripe
// (a mock Stripe server) for local end-to-end tests. Live keys are
// refused while it is set, so a misconfigured production node can never send
// real payments to a fake endpoint.

var errStripeLiveKeyOnMock = errors.New("TOKEN_STRIPE_API_BASE is set: live Stripe keys are refused")

type tokenTestStripeBackend struct {
	stripe.Backend
}

func refuseLiveKey(key string) error {
	if strings.HasPrefix(key, "sk_live_") || strings.HasPrefix(key, "rk_live_") {
		return errStripeLiveKeyOnMock
	}
	return nil
}

func (b tokenTestStripeBackend) Call(method, path, key string, params stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	if err := refuseLiveKey(key); err != nil {
		return err
	}
	return b.Backend.Call(method, path, key, params, v)
}

func (b tokenTestStripeBackend) CallRaw(method, path, key string, body *form.Values, params *stripe.Params, v stripe.LastResponseSetter) error {
	if err := refuseLiveKey(key); err != nil {
		return err
	}
	return b.Backend.CallRaw(method, path, key, body, params, v)
}

func init() {
	base := strings.TrimRight(os.Getenv("TOKEN_STRIPE_API_BASE"), "/")
	if base == "" {
		return
	}
	backend := stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(base)})
	stripe.SetBackend(stripe.APIBackend, tokenTestStripeBackend{Backend: backend})
	common.SysLog("TOKEN: Stripe API calls go to " + base + " (test stack; live keys refused)")
}
