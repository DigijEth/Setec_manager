package hostinger

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"setec-manager/internal/hosting"
)

// hostingerSubscription is the Hostinger API representation of a subscription.
type hostingerSubscription struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	RenewalDate string `json:"renewal_date"`
	Price       struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"price"`
}

// hostingerCatalogItem is the Hostinger API representation of a catalog item.
type hostingerCatalogItem struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Category string            `json:"category"`
	Price    float64           `json:"price"`
	Currency string            `json:"currency"`
	Features map[string]string `json:"features,omitempty"`
}

// hostingerPaymentMethod is the Hostinger API representation of a payment method.
type hostingerPaymentMethod struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
	Default  bool   `json:"default"`
}

// PaymentMethod is the exported type for payment method information.
type PaymentMethod struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
	Default  bool   `json:"default"`
}

// ListSubscriptions retrieves all billing subscriptions.
func (c *Client) ListSubscriptions() ([]hosting.Subscription, error) {
	var subs []hostingerSubscription
	if err := c.doRequest(http.MethodGet, "/api/billing/v1/subscriptions", nil, &subs); err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}

	result := make([]hosting.Subscription, 0, len(subs))
	for _, s := range subs {
		renewsAt, _ := time.Parse(time.RFC3339, s.RenewalDate)
		result = append(result, hosting.Subscription{
			ID:       strconv.Itoa(s.ID),
			Name:     s.Name,
			Status:   s.Status,
			RenewsAt: renewsAt,
			Price:    s.Price.Amount,
			Currency: s.Price.Currency,
		})
	}
	return result, nil
}

// GetCatalog retrieves the product catalog, optionally filtered by category.
// If category is empty, all catalog items are returned.
func (c *Client) GetCatalog(category string) ([]hosting.CatalogItem, error) {
	path := "/api/billing/v1/catalog"
	if category != "" {
		path += "?" + url.Values{"category": {category}}.Encode()
	}

	var items []hostingerCatalogItem
	if err := c.doRequest(http.MethodGet, path, nil, &items); err != nil {
		return nil, fmt.Errorf("get catalog: %w", err)
	}

	result := make([]hosting.CatalogItem, 0, len(items))
	for _, item := range items {
		result = append(result, hosting.CatalogItem{
			ID:       item.ID,
			Name:     item.Name,
			Category: item.Category,
			Price:    item.Price,
			Currency: item.Currency,
			Features: item.Features,
		})
	}
	return result, nil
}

// ListPaymentMethods retrieves all payment methods on the account.
// This is a Hostinger-specific method not part of the generic Provider interface.
func (c *Client) ListPaymentMethods() ([]PaymentMethod, error) {
	var methods []hostingerPaymentMethod
	if err := c.doRequest(http.MethodGet, "/api/billing/v1/payment-methods", nil, &methods); err != nil {
		return nil, fmt.Errorf("list payment methods: %w", err)
	}

	result := make([]PaymentMethod, 0, len(methods))
	for _, m := range methods {
		result = append(result, PaymentMethod{
			ID:       strconv.Itoa(m.ID),
			Type:     m.Type,
			Name:     m.Name,
			Last4:    m.Last4,
			ExpMonth: m.ExpMonth,
			ExpYear:  m.ExpYear,
			Default:  m.Default,
		})
	}
	return result, nil
}
