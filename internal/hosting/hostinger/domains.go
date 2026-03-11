package hostinger

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"setec-manager/internal/hosting"
)

// hostingerDomain is the Hostinger API representation of a domain.
type hostingerDomain struct {
	Domain            string   `json:"domain"`
	Status            string   `json:"status"`
	ExpirationDate    string   `json:"expiration_date"`
	AutoRenew         bool     `json:"auto_renew"`
	DomainLock        bool     `json:"domain_lock"`
	PrivacyProtection bool     `json:"privacy_protection"`
	Nameservers       []string `json:"nameservers"`
}

// hostingerDomainList wraps the list response.
type hostingerDomainList struct {
	Domains []hostingerDomain `json:"domains"`
}

// hostingerAvailabilityRequest is the check-availability request body.
type hostingerAvailabilityRequest struct {
	Domains []string `json:"domains"`
}

// hostingerAvailabilityResult is a single domain availability result.
type hostingerAvailabilityResult struct {
	Domain    string `json:"domain"`
	Available bool   `json:"available"`
	Price     *struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"price,omitempty"`
}

// hostingerPurchaseRequest is the domain purchase request body.
type hostingerPurchaseRequest struct {
	Domain          string `json:"domain"`
	Period          int    `json:"period"`
	AutoRenew       bool   `json:"auto_renew"`
	Privacy         bool   `json:"privacy_protection"`
	PaymentMethodID string `json:"payment_method_id,omitempty"`
}

// hostingerNameserversRequest is the body for updating nameservers.
type hostingerNameserversRequest struct {
	Nameservers []string `json:"nameservers"`
}

// ListDomains retrieves all domains in the account portfolio.
func (c *Client) ListDomains() ([]hosting.Domain, error) {
	var list hostingerDomainList
	if err := c.doRequest(http.MethodGet, "/api/domains/v1/portfolio", nil, &list); err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}

	domains := make([]hosting.Domain, 0, len(list.Domains))
	for _, d := range list.Domains {
		domains = append(domains, toSummaryDomain(d))
	}
	return domains, nil
}

// GetDomain retrieves details for a specific domain.
func (c *Client) GetDomain(domain string) (*hosting.DomainDetail, error) {
	path := fmt.Sprintf("/api/domains/v1/portfolio/%s", url.PathEscape(domain))

	var d hostingerDomain
	if err := c.doRequest(http.MethodGet, path, nil, &d); err != nil {
		return nil, fmt.Errorf("get domain %s: %w", domain, err)
	}

	result := toDetailDomain(d)
	return &result, nil
}

// CheckDomainAvailability checks whether the given domain is available for
// registration across the specified TLDs. If tlds is empty, the domain string
// is checked as-is.
func (c *Client) CheckDomainAvailability(domain string, tlds []string) ([]hosting.DomainAvailability, error) {
	// Build the list of fully qualified domain names to check.
	var domains []string
	if len(tlds) == 0 {
		domains = []string{domain}
	} else {
		for _, tld := range tlds {
			tld = strings.TrimPrefix(tld, ".")
			domains = append(domains, domain+"."+tld)
		}
	}

	req := hostingerAvailabilityRequest{Domains: domains}

	var results []hostingerAvailabilityResult
	if err := c.doRequest(http.MethodPost, "/api/domains/v1/availability", req, &results); err != nil {
		return nil, fmt.Errorf("check domain availability: %w", err)
	}

	avail := make([]hosting.DomainAvailability, 0, len(results))
	for _, r := range results {
		da := hosting.DomainAvailability{
			Domain:    r.Domain,
			Available: r.Available,
		}
		// Extract TLD from the domain name.
		if idx := strings.Index(r.Domain, "."); idx >= 0 {
			da.TLD = r.Domain[idx+1:]
		}
		if r.Price != nil {
			da.Price = r.Price.Amount
			da.Currency = r.Price.Currency
		}
		avail = append(avail, da)
	}
	return avail, nil
}

// PurchaseDomain registers a new domain.
func (c *Client) PurchaseDomain(req hosting.DomainPurchaseRequest) (*hosting.OrderResult, error) {
	body := hostingerPurchaseRequest{
		Domain:          req.Domain,
		Period:          req.Years,
		AutoRenew:       req.AutoRenew,
		Privacy:         req.Privacy,
		PaymentMethodID: req.PaymentMethod,
	}

	var d hostingerDomain
	if err := c.doRequest(http.MethodPost, "/api/domains/v1/portfolio", body, &d); err != nil {
		return nil, fmt.Errorf("purchase domain %s: %w", req.Domain, err)
	}

	return &hosting.OrderResult{
		OrderID: d.Domain,
		Status:  "completed",
		Message: fmt.Sprintf("domain %s registered", d.Domain),
	}, nil
}

// SetNameservers updates the nameservers for a domain.
func (c *Client) SetNameservers(domain string, nameservers []string) error {
	path := fmt.Sprintf("/api/domains/v1/portfolio/%s/nameservers", url.PathEscape(domain))
	body := hostingerNameserversRequest{Nameservers: nameservers}

	if err := c.doRequest(http.MethodPut, path, body, nil); err != nil {
		return fmt.Errorf("set nameservers for %s: %w", domain, err)
	}
	return nil
}

// EnableDomainLock enables the registrar lock for a domain.
func (c *Client) EnableDomainLock(domain string) error {
	path := fmt.Sprintf("/api/domains/v1/portfolio/%s/domain-lock", url.PathEscape(domain))
	if err := c.doRequest(http.MethodPut, path, nil, nil); err != nil {
		return fmt.Errorf("enable domain lock for %s: %w", domain, err)
	}
	return nil
}

// DisableDomainLock disables the registrar lock for a domain.
func (c *Client) DisableDomainLock(domain string) error {
	path := fmt.Sprintf("/api/domains/v1/portfolio/%s/domain-lock", url.PathEscape(domain))
	if err := c.doRequest(http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("disable domain lock for %s: %w", domain, err)
	}
	return nil
}

// EnablePrivacyProtection enables WHOIS privacy protection for a domain.
func (c *Client) EnablePrivacyProtection(domain string) error {
	path := fmt.Sprintf("/api/domains/v1/portfolio/%s/privacy-protection", url.PathEscape(domain))
	if err := c.doRequest(http.MethodPut, path, nil, nil); err != nil {
		return fmt.Errorf("enable privacy protection for %s: %w", domain, err)
	}
	return nil
}

// DisablePrivacyProtection disables WHOIS privacy protection for a domain.
func (c *Client) DisablePrivacyProtection(domain string) error {
	path := fmt.Sprintf("/api/domains/v1/portfolio/%s/privacy-protection", url.PathEscape(domain))
	if err := c.doRequest(http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("disable privacy protection for %s: %w", domain, err)
	}
	return nil
}

// toSummaryDomain converts a Hostinger domain to the summary Domain type.
func toSummaryDomain(d hostingerDomain) hosting.Domain {
	expires, _ := time.Parse(time.RFC3339, d.ExpirationDate)
	return hosting.Domain{
		Name:      d.Domain,
		Status:    d.Status,
		ExpiresAt: expires,
	}
}

// toDetailDomain converts a Hostinger domain to the full DomainDetail type.
func toDetailDomain(d hostingerDomain) hosting.DomainDetail {
	expires, _ := time.Parse(time.RFC3339, d.ExpirationDate)
	return hosting.DomainDetail{
		Name:              d.Domain,
		Status:            d.Status,
		Registrar:         "hostinger",
		ExpiresAt:         expires,
		AutoRenew:         d.AutoRenew,
		Locked:            d.DomainLock,
		PrivacyProtection: d.PrivacyProtection,
		Nameservers:       d.Nameservers,
	}
}
