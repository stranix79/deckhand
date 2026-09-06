package hub

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kolo/xmlrpc"
)

// Odoo billing: Odoo Subscriptions (Enterprise) is the subscription engine
// and the invoicing system; the hub only mirrors who is Pro.
//
//   - "Subscribe" sends the user to the Deckhand Pro product on the Odoo
//     shop (ODOO_PRODUCT_URL); Odoo takes the payment (its own Stripe or
//     other provider), stores the card, renews monthly, issues invoices.
//   - "Manage subscription" opens the Odoo customer portal.
//   - Every ODOO_SYNC_INTERVAL the hub lists the in-progress subscriptions
//     over XML-RPC and matches them to hub users by e-mail.
//
// The match key is the e-mail: the customer must use the same address on
// the shop and on the hub. The product page says so.

// odooClient is the minimal XML-RPC client (external API of Odoo).
type odooClient struct {
	url, db, user, password string
	uid                     int
}

func newOdooClient(url, db, user, password string) *odooClient {
	return &odooClient{url: strings.TrimRight(url, "/"), db: db, user: user, password: password}
}

func (c *odooClient) login() error {
	cl, err := xmlrpc.NewClient(c.url+"/xmlrpc/2/common", nil)
	if err != nil {
		return err
	}
	defer func() { _ = cl.Close() }()
	var uid any
	if err := cl.Call("authenticate", []any{c.db, c.user, c.password, map[string]any{}}, &uid); err != nil {
		return err
	}
	id, ok := uid.(int64)
	if !ok || id == 0 {
		return fmt.Errorf("odoo: authentication failed for %s", c.user)
	}
	c.uid = int(id)
	return nil
}

// executeKw calls model.method(args, kwargs).
func (c *odooClient) executeKw(model, method string, args []any, kwargs map[string]any, out any) error {
	if c.uid == 0 {
		if err := c.login(); err != nil {
			return err
		}
	}
	cl, err := xmlrpc.NewClient(c.url+"/xmlrpc/2/object", nil)
	if err != nil {
		return err
	}
	defer func() { _ = cl.Close() }()
	return cl.Call("execute_kw", []any{c.db, c.uid, c.password, model, method, args, kwargs}, out)
}

// odooSubscription is one in-progress subscription seen from Odoo.
type odooSubscription struct {
	PartnerID   int
	Email       string
	Reference   string
	NextInvoice *time.Time
}

// activeSubscriptions lists the "In Progress" subscriptions with their
// customer e-mail (the commercial partner's, then the contact's).
func (c *odooClient) activeSubscriptions() ([]odooSubscription, error) {
	var rows []map[string]any
	domain := []any{[]any{"is_subscription", "=", true}, []any{"subscription_state", "=", "3_progress"}}
	if err := c.executeKw("sale.order", "search_read", []any{domain}, map[string]any{
		"fields": []string{"partner_id", "commercial_partner_id", "name", "next_invoice_date"},
		"limit":  5000,
	}, &rows); err != nil {
		return nil, err
	}
	// Collect partner ids, then read e-mails in one call.
	ids := map[int]bool{}
	pid := func(v any) int {
		if arr, ok := v.([]any); ok && len(arr) > 0 {
			if n, ok := arr[0].(int64); ok {
				return int(n)
			}
		}
		return 0
	}
	for _, r := range rows {
		ids[pid(r["partner_id"])] = true
		ids[pid(r["commercial_partner_id"])] = true
	}
	delete(ids, 0)
	idList := make([]any, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	emails := map[int]string{}
	if len(idList) > 0 {
		var partners []map[string]any
		if err := c.executeKw("res.partner", "read", []any{idList}, map[string]any{"fields": []string{"email"}}, &partners); err != nil {
			return nil, err
		}
		for _, p := range partners {
			if id, ok := p["id"].(int64); ok {
				if e, ok := p["email"].(string); ok {
					emails[int(id)] = strings.ToLower(strings.TrimSpace(e))
				}
			}
		}
	}
	out := make([]odooSubscription, 0, len(rows))
	for _, r := range rows {
		s := odooSubscription{PartnerID: pid(r["partner_id"])}
		s.Email = emails[s.PartnerID]
		if s.Email == "" {
			s.Email = emails[pid(r["commercial_partner_id"])]
		}
		if n, ok := r["name"].(string); ok {
			s.Reference = n
		}
		if d, ok := r["next_invoice_date"].(string); ok && d != "" {
			if t, err := time.Parse("2006-01-02", d); err == nil {
				s.NextInvoice = &t
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// syncOdoo mirrors Odoo's in-progress subscriptions into the subscriptions
// table: users whose e-mail has one become active, users that had an Odoo
// subscription and no longer do become canceled. Stripe rows are untouched.
func (h *Hub) syncOdoo(ctx context.Context) error {
	subs, err := h.odoo.activeSubscriptions()
	if err != nil {
		return err
	}
	byEmail := map[string]odooSubscription{}
	for _, s := range subs {
		if s.Email != "" {
			byEmail[s.Email] = s
		}
	}
	rows, err := h.db.Query(ctx, `SELECT u.id, lower(u.email), COALESCE(s.status, ''), COALESCE(s.stripe_customer_id, '') FROM users u LEFT JOIN subscriptions s ON s.user_id = u.id`)
	if err != nil {
		return err
	}
	type userRow struct{ id, email, status, customer string }
	var users []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.id, &u.email, &u.status, &u.customer); err != nil {
			rows.Close()
			return err
		}
		users = append(users, u)
	}
	rows.Close()

	activated, canceled := 0, 0
	for _, u := range users {
		s, isPro := byEmail[u.email]
		switch {
		case isPro:
			// A grace period of 7 days after the next invoice date covers a
			// late card payment; Odoo's own auto-close handles real churn.
			var end *time.Time
			if s.NextInvoice != nil {
				t := s.NextInvoice.Add(7 * 24 * time.Hour)
				end = &t
			}
			if u.status != "active" {
				activated++
			}
			if _, err := h.db.Exec(ctx, `
				INSERT INTO subscriptions (user_id, stripe_customer_id, stripe_subscription_id, status, current_period_end)
				VALUES ($1, $2, $3, 'active', $4)
				ON CONFLICT (user_id) DO UPDATE SET stripe_customer_id = EXCLUDED.stripe_customer_id,
				  stripe_subscription_id = EXCLUDED.stripe_subscription_id, status = 'active',
				  current_period_end = EXCLUDED.current_period_end, updated_at = now()`,
				u.id, fmt.Sprintf("odoo:%d", s.PartnerID), s.Reference, end); err != nil {
				return err
			}
		case strings.HasPrefix(u.customer, "odoo:") && u.status == "active":
			canceled++
			if _, err := h.db.Exec(ctx, `UPDATE subscriptions SET status = 'canceled', updated_at = now() WHERE user_id = $1`, u.id); err != nil {
				return err
			}
		}
	}
	if activated > 0 || canceled > 0 {
		slog.Info("odoo sync", "in_progress", len(subs), "activated", activated, "canceled", canceled)
	}
	return nil
}

// odooSyncLoop runs syncOdoo at the configured interval.
func (h *Hub) odooSyncLoop(ctx context.Context) {
	run := func() {
		sctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		if err := h.syncOdoo(sctx); err != nil {
			slog.Error("odoo sync", "err", err)
			h.odoo.uid = 0 // force a fresh login next time
		}
	}
	run()
	t := time.NewTicker(h.cfg.OdooSyncInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
