package hub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stripe/stripe-go/v81"
	portalsession "github.com/stripe/stripe-go/v81/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"
)

// Subscription is a subscriptions row.
type Subscription struct {
	CustomerID, SubscriptionID, Status string
	PeriodEnd                          *time.Time
}

func (h *Hub) getSubscription(ctx context.Context, userID string) (*Subscription, error) {
	var s Subscription
	var subID *string
	err := h.db.QueryRow(ctx, `SELECT stripe_customer_id, stripe_subscription_id, status, current_period_end FROM subscriptions WHERE user_id = $1`, userID).
		Scan(&s.CustomerID, &subID, &s.Status, &s.PeriodEnd)
	if err != nil {
		return nil, err
	}
	if subID != nil {
		s.SubscriptionID = *subID
	}
	return &s, nil
}

func (h *Hub) billingPage(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	sub, _ := h.getSubscription(r.Context(), u.ID)
	msg := ""
	switch r.URL.Query().Get("status") {
	case "ok":
		msg = "Thank you! Your subscription is being activated; refresh in a few seconds."
	case "cancel":
		msg = "Checkout cancelled."
	}
	h.render(w, r, "billing.html", map[string]any{"Title": "Plan", "Sub": sub, "StripeEnabled": h.cfg.StripeEnabled(), "OdooEnabled": h.cfg.OdooEnabled(),
		"OdooProductURL": h.cfg.OdooProductURL, "OdooPortalURL": h.cfg.OdooPortal(), "Message": msg})
}

// billingCheckout starts a Stripe Checkout session for the monthly price.
func (h *Hub) billingCheckout(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.StripeEnabled() {
		http.Error(w, "billing not configured", http.StatusNotImplemented)
		return
	}
	u := userOf(r)
	stripe.Key = h.cfg.StripeSecretKey
	params := &stripe.CheckoutSessionParams{
		Mode:                stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems:           []*stripe.CheckoutSessionLineItemParams{{Price: stripe.String(h.cfg.StripePriceID), Quantity: stripe.Int64(1)}},
		SuccessURL:          stripe.String(h.cfg.BaseURL + "/billing?status=ok"),
		CancelURL:           stripe.String(h.cfg.BaseURL + "/billing?status=cancel"),
		ClientReferenceID:   stripe.String(u.ID),
		AllowPromotionCodes: stripe.Bool(true),
	}
	if sub, err := h.getSubscription(r.Context(), u.ID); err == nil && sub.CustomerID != "" {
		params.Customer = stripe.String(sub.CustomerID)
	} else {
		params.CustomerEmail = stripe.String(u.Email)
	}
	s, err := checkoutsession.New(params)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, s.URL, http.StatusSeeOther)
}

// billingPortal opens the Stripe customer portal (cancel, invoices, card).
func (h *Hub) billingPortal(w http.ResponseWriter, r *http.Request) {
	if h.cfg.OdooEnabled() {
		http.Redirect(w, r, h.cfg.OdooPortal(), http.StatusSeeOther)
		return
	}
	u := userOf(r)
	sub, err := h.getSubscription(r.Context(), u.ID)
	if err != nil || !h.cfg.StripeEnabled() {
		http.Redirect(w, r, "/billing", http.StatusSeeOther)
		return
	}
	stripe.Key = h.cfg.StripeSecretKey
	s, err := portalsession.New(&stripe.BillingPortalSessionParams{
		Customer:  stripe.String(sub.CustomerID),
		ReturnURL: stripe.String(h.cfg.BaseURL + "/billing"),
	})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, s.URL, http.StatusSeeOther)
}

// stripeWebhook keeps the subscriptions table in sync. Signature checked
// with the endpoint secret; unknown events are acknowledged and ignored.
func (h *Hub) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	if h.cfg.StripeWebhookSecret == "" {
		http.Error(w, "billing not configured", http.StatusNotImplemented)
		return
	}
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	ev, err := webhook.ConstructEventWithOptions(payload, r.Header.Get("Stripe-Signature"), h.cfg.StripeWebhookSecret, webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}
	if err := h.applyStripeEvent(r.Context(), ev); err != nil {
		slog.Error("stripe event", "type", ev.Type, "err", err)
		http.Error(w, "apply", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// applyStripeEvent is separated from the HTTP handler so it can be unit-tested.
func (h *Hub) applyStripeEvent(ctx context.Context, ev stripe.Event) error {
	switch ev.Type {
	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(ev.Data.Raw, &cs); err != nil {
			return err
		}
		if cs.ClientReferenceID == "" || cs.Customer == nil {
			return errors.New("checkout session without client_reference_id or customer")
		}
		subID := ""
		if cs.Subscription != nil {
			subID = cs.Subscription.ID
		}
		_, err := h.db.Exec(ctx, `
			INSERT INTO subscriptions (user_id, stripe_customer_id, stripe_subscription_id, status)
			VALUES ($1, $2, NULLIF($3, ''), 'active')
			ON CONFLICT (user_id) DO UPDATE SET stripe_customer_id = EXCLUDED.stripe_customer_id,
			  stripe_subscription_id = COALESCE(EXCLUDED.stripe_subscription_id, subscriptions.stripe_subscription_id),
			  status = 'active', updated_at = now()`,
			cs.ClientReferenceID, cs.Customer.ID, subID)
		return err
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(ev.Data.Raw, &sub); err != nil {
			return err
		}
		if sub.Customer == nil {
			return errors.New("subscription without customer")
		}
		status := string(sub.Status)
		if ev.Type == "customer.subscription.deleted" {
			status = "canceled"
		}
		var end *time.Time
		if sub.CurrentPeriodEnd > 0 {
			t := time.Unix(sub.CurrentPeriodEnd, 0)
			end = &t
		}
		tag, err := h.db.Exec(ctx, `UPDATE subscriptions SET stripe_subscription_id = $2, status = $3, current_period_end = $4, updated_at = now() WHERE stripe_customer_id = $1`,
			sub.Customer.ID, sub.ID, status, end)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			slog.Warn("subscription event for unknown customer", "customer", sub.Customer.ID)
		}
		return nil
	}
	return nil
}

var _ = pgx.ErrNoRows
