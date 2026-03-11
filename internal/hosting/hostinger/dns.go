package hostinger

import (
	"fmt"
	"net/http"
	"net/url"

	"setec-manager/internal/hosting"
)

// hostingerDNSRecord is the Hostinger API representation of a DNS record.
type hostingerDNSRecord struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
}

// hostingerDNSUpdateRequest is the request body for updating DNS records.
type hostingerDNSUpdateRequest struct {
	Records   []hostingerDNSRecord `json:"records"`
	Overwrite bool                 `json:"overwrite"`
}

// hostingerDNSValidateRequest is the request body for validating DNS records.
type hostingerDNSValidateRequest struct {
	Records []hostingerDNSRecord `json:"records"`
}

// ListDNSRecords retrieves all DNS records for the given domain.
func (c *Client) ListDNSRecords(domain string) ([]hosting.DNSRecord, error) {
	path := fmt.Sprintf("/api/dns/v1/zones/%s", url.PathEscape(domain))

	var apiRecords []hostingerDNSRecord
	if err := c.doRequest(http.MethodGet, path, nil, &apiRecords); err != nil {
		return nil, fmt.Errorf("list DNS records for %s: %w", domain, err)
	}

	records := make([]hosting.DNSRecord, 0, len(apiRecords))
	for _, r := range apiRecords {
		records = append(records, toGenericDNSRecord(r))
	}
	return records, nil
}

// UpdateDNSRecords updates DNS records for the given domain.
// If overwrite is true, existing records are replaced entirely.
func (c *Client) UpdateDNSRecords(domain string, records []hosting.DNSRecord, overwrite bool) error {
	path := fmt.Sprintf("/api/dns/v1/zones/%s", url.PathEscape(domain))

	hostingerRecords := make([]hostingerDNSRecord, 0, len(records))
	for _, r := range records {
		hostingerRecords = append(hostingerRecords, toHostingerDNSRecord(r))
	}

	// Validate first.
	validatePath := fmt.Sprintf("/api/dns/v1/zones/%s/validate", url.PathEscape(domain))
	validateReq := hostingerDNSValidateRequest{Records: hostingerRecords}
	if err := c.doRequest(http.MethodPost, validatePath, validateReq, nil); err != nil {
		return fmt.Errorf("validate DNS records for %s: %w", domain, err)
	}

	req := hostingerDNSUpdateRequest{
		Records:   hostingerRecords,
		Overwrite: overwrite,
	}
	if err := c.doRequest(http.MethodPut, path, req, nil); err != nil {
		return fmt.Errorf("update DNS records for %s: %w", domain, err)
	}
	return nil
}

// CreateDNSRecord adds a single DNS record to the domain without overwriting.
func (c *Client) CreateDNSRecord(domain string, record hosting.DNSRecord) error {
	return c.UpdateDNSRecords(domain, []hosting.DNSRecord{record}, false)
}

// DeleteDNSRecord removes DNS records matching the given filter.
func (c *Client) DeleteDNSRecord(domain string, filter hosting.DNSRecordFilter) error {
	path := fmt.Sprintf("/api/dns/v1/zones/%s", url.PathEscape(domain))

	params := url.Values{}
	if filter.Name != "" {
		params.Set("name", filter.Name)
	}
	if filter.Type != "" {
		params.Set("type", filter.Type)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	if err := c.doRequest(http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("delete DNS record %s/%s for %s: %w", filter.Name, filter.Type, domain, err)
	}
	return nil
}

// ResetDNSRecords resets the domain's DNS zone to default records.
func (c *Client) ResetDNSRecords(domain string) error {
	path := fmt.Sprintf("/api/dns/v1/zones/%s/reset", url.PathEscape(domain))
	if err := c.doRequest(http.MethodPost, path, nil, nil); err != nil {
		return fmt.Errorf("reset DNS records for %s: %w", domain, err)
	}
	return nil
}

// toGenericDNSRecord converts a Hostinger DNS record to the generic type.
func toGenericDNSRecord(r hostingerDNSRecord) hosting.DNSRecord {
	rec := hosting.DNSRecord{
		Type:    r.Type,
		Name:    r.Name,
		Content: r.Content,
		TTL:     r.TTL,
	}
	if r.Priority != nil {
		rec.Priority = *r.Priority
	}
	return rec
}

// toHostingerDNSRecord converts a generic DNS record to the Hostinger format.
func toHostingerDNSRecord(r hosting.DNSRecord) hostingerDNSRecord {
	rec := hostingerDNSRecord{
		Type:    r.Type,
		Name:    r.Name,
		Content: r.Content,
		TTL:     r.TTL,
	}
	if r.Priority != 0 {
		p := r.Priority
		rec.Priority = &p
	}
	return rec
}
