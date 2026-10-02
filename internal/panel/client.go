// Package panel is the Gamaj API client used by Gamaj Bot. The bot has no
// database access by design: every operation goes through the Gamaj API with
// the bot's dedicated API key.
package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Plan is a purchasable service plan exposed by the Gamaj API.
type Plan struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Price        int64  `json:"price"`
	Duration     int64  `json:"duration_days"`
	DataLimit    int64  `json:"data_limit"`
	IPCommission int    `json:"ip_limit,omitempty"`
	Description  string `json:"description,omitempty"`
}

// Order is a purchase order awaiting payment or already paid.
type Order struct {
	ID        string `json:"id"`
	PlanID    int64  `json:"plan_id"`
	PlanName  string `json:"plan_name,omitempty"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// Wallet is the buyer wallet state reported by the Gamaj API.
type Wallet struct {
	Balance  int64  `json:"balance"`
	Currency string `json:"currency,omitempty"`
}

// PaymentLink is a payment gateway redirect for an order.
type PaymentLink struct {
	PaymentURL string `json:"payment_url"`
	OrderID    string `json:"order_id,omitempty"`
}

// ServiceView is a provisioned service with its credentials.
type ServiceView struct {
	Username        string            `json:"username"`
	Status          string            `json:"status"`
	UsedTraffic     int64             `json:"used_traffic"`
	DataLimit       int64             `json:"data_limit"`
	Expire          int64             `json:"expire"`
	SubscriptionURL string            `json:"subscription_url"`
	Links           []string          `json:"links"`
	Extra           map[string]string `json:"-"`
}

// Client talks to the Gamaj API with the bot API key as bearer token.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New creates a Gamaj API client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, payload, target any) error {
	endpoint := c.baseURL + path
	var reader io.Reader
	body := []byte(nil)
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode Gamaj API request: %w", err)
		}
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return errors.New("prepare Gamaj API request")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return errors.New("Gamaj API request failed")
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return errors.New("read Gamaj API response")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var detail struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(data, &detail) == nil && detail.Detail != "" {
			return fmt.Errorf("Gamaj API %d: %s", res.StatusCode, detail.Detail)
		}
		return fmt.Errorf("Gamaj API returned HTTP %d", res.StatusCode)
	}
	if target != nil && len(bytes.TrimSpace(data)) > 0 {
		return json.Unmarshal(data, target)
	}
	return nil
}

// Ping validates the API key against the Gamaj API.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/admin", nil, nil)
}

// Plans lists purchasable plans.
func (c *Client) Plans(ctx context.Context) ([]Plan, error) {
	var plans []Plan
	if err := c.do(ctx, http.MethodGet, "/bot/plans", nil, &plans); err != nil {
		return nil, err
	}
	return plans, nil
}

// CreateOrder starts a purchase for a plan with the buyer's Telegram ID.
func (c *Client) CreateOrder(ctx context.Context, planID int64, telegramID int64) (Order, error) {
	var order Order
	payload := map[string]string{"telegram_id": strconv.FormatInt(telegramID, 10)}
	if err := c.do(ctx, http.MethodPost, "/bot/plans/"+strconv.FormatInt(planID, 10)+"/orders", payload, &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

// Order fetches one order by ID.
func (c *Client) Order(ctx context.Context, orderID string) (Order, error) {
	var order Order
	if err := c.do(ctx, http.MethodGet, "/bot/orders/"+url.PathEscape(orderID), nil, &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

// Wallet returns the wallet balance for a buyer.
func (c *Client) Wallet(ctx context.Context, telegramID int64) (Wallet, error) {
	var wallet Wallet
	path := "/bot/wallet?telegram_id=" + strconv.FormatInt(telegramID, 10)
	if err := c.do(ctx, http.MethodGet, path, nil, &wallet); err != nil {
		return Wallet{}, err
	}
	return wallet, nil
}

// PayWithWallet settles an order from the buyer wallet balance.
func (c *Client) PayWithWallet(ctx context.Context, orderID string) (Order, error) {
	var order Order
	if err := c.do(ctx, http.MethodPost, "/bot/orders/"+url.PathEscape(orderID)+"/pay-wallet", nil, &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

// CreatePaymentLink requests a gateway payment URL (Tetraminator) for an order.
func (c *Client) CreatePaymentLink(ctx context.Context, orderID string) (PaymentLink, error) {
	var link PaymentLink
	if err := c.do(ctx, http.MethodPost, "/bot/orders/"+url.PathEscape(orderID)+"/payment-link", nil, &link); err != nil {
		return PaymentLink{}, err
	}
	return link, nil
}

// VerifyPayment asks the Gamaj API to verify a gateway payment; the API only
// provisions the service after a confirmed inquiry result.
func (c *Client) VerifyPayment(ctx context.Context, orderID, payID string) (Order, error) {
	var order Order
	payload := map[string]string{"pay_id": payID}
	if err := c.do(ctx, http.MethodPost, "/bot/orders/"+url.PathEscape(orderID)+"/verify", payload, &order); err != nil {
		return Order{}, err
	}
	return order, nil
}

// FormatBytes renders a byte count in human-readable units.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	dv := int64(unit)
	for _, exp := range []string{"KiB", "MiB", "GiB", "TiB", "PiB"} {
		if bytes < dv*unit {
			return fmt.Sprintf("%.1f %s", float64(bytes)/float64(dv), exp)
		}
		dv *= unit
	}
	return fmt.Sprintf("%d B", bytes)
}

// Service returns the provisioned service and credentials for a buyer.
func (c *Client) Service(ctx context.Context, telegramID int64) (ServiceView, error) {
	var service ServiceView
	path := "/bot/service?telegram_id=" + strconv.FormatInt(telegramID, 10)
	if err := c.do(ctx, http.MethodGet, path, nil, &service); err != nil {
		return ServiceView{}, err
	}
	return service, nil
}
