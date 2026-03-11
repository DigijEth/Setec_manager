package handlers

import (
	"fmt"
	"net/http"
	"time"

	"setec-manager/internal/acme"
)

type certInfo struct {
	Domain    string    `json:"domain"`
	Issuer    string    `json:"issuer"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	DaysLeft  int       `json:"days_left"`
	AutoRenew bool      `json:"auto_renew"`
}

func (h *Handler) SSLOverview(w http.ResponseWriter, r *http.Request) {
	certs := h.listCerts()
	h.render(w, "ssl.html", certs)
}

func (h *Handler) SSLStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.listCerts())
}

func (h *Handler) SSLIssue(w http.ResponseWriter, r *http.Request) {
	domain := paramStr(r, "domain")
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain required")
		return
	}

	client := acme.NewClient(
		h.Config.ACME.Email,
		h.Config.ACME.Staging,
		h.Config.Nginx.CertbotWebroot,
		h.Config.ACME.AccountDir,
	)

	info, err := client.Issue(domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("certbot failed: %s", err))
		return
	}

	// Update site SSL paths
	site, _ := h.DB.GetSiteByDomain(domain)
	if site != nil {
		site.SSLEnabled = true
		site.SSLCertPath = info.CertPath
		site.SSLKeyPath = info.KeyPath
		h.DB.UpdateSite(site)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "issued", "cert": info.CertPath})
}

func (h *Handler) SSLRenew(w http.ResponseWriter, r *http.Request) {
	domain := paramStr(r, "domain")

	client := acme.NewClient(
		h.Config.ACME.Email,
		h.Config.ACME.Staging,
		h.Config.Nginx.CertbotWebroot,
		h.Config.ACME.AccountDir,
	)

	if err := client.Renew(domain); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("renewal failed: %s", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "renewed"})
}

func (h *Handler) listCerts() []certInfo {
	var certs []certInfo

	// First, gather certs from DB-tracked sites
	sites, _ := h.DB.ListSites()
	for _, site := range sites {
		if !site.SSLEnabled || site.SSLCertPath == "" {
			continue
		}

		ci := certInfo{
			Domain:    site.Domain,
			AutoRenew: site.SSLAuto,
		}

		client := acme.NewClient(
			h.Config.ACME.Email,
			h.Config.ACME.Staging,
			h.Config.Nginx.CertbotWebroot,
			h.Config.ACME.AccountDir,
		)

		info, err := client.GetCertInfo(site.Domain)
		if err == nil {
			ci.Issuer = info.Issuer
			ci.NotBefore = info.ExpiresAt.Add(-90 * 24 * time.Hour) // approximate
			ci.NotAfter = info.ExpiresAt
			ci.DaysLeft = info.DaysLeft
		}

		certs = append(certs, ci)
	}

	// Also check Let's Encrypt certs directory via ACME client
	client := acme.NewClient(
		h.Config.ACME.Email,
		h.Config.ACME.Staging,
		h.Config.Nginx.CertbotWebroot,
		h.Config.ACME.AccountDir,
	)

	leCerts, _ := client.ListCerts()
	for _, le := range leCerts {
		// Skip if already found via site
		found := false
		for _, c := range certs {
			if c.Domain == le.Domain {
				found = true
				break
			}
		}
		if found {
			continue
		}

		certs = append(certs, certInfo{
			Domain:   le.Domain,
			Issuer:   le.Issuer,
			NotAfter: le.ExpiresAt,
			DaysLeft: le.DaysLeft,
		})
	}

	return certs
}
