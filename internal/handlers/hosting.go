package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"setec-manager/internal/hosting"
)

// providerInfo is the view model sent to the hosting template and JSON responses.
type providerInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Connected   bool   `json:"connected"`
	HasConfig   bool   `json:"has_config"`
}

// listProviderInfo builds a summary of every registered provider and its config status.
func (h *Handler) listProviderInfo() []providerInfo {
	names := hosting.List()
	out := make([]providerInfo, 0, len(names))
	for _, name := range names {
		p, ok := hosting.Get(name)
		if !ok {
			continue
		}
		pi := providerInfo{
			Name:        p.Name(),
			DisplayName: p.DisplayName(),
		}
		if h.HostingConfigs != nil {
			cfg, err := h.HostingConfigs.Load(name)
			if err == nil && cfg != nil {
				pi.HasConfig = true
				if cfg.APIKey != "" {
					pi.Connected = true
				}
			}
		}
		out = append(out, pi)
	}
	return out
}

// getProvider retrieves the provider from the URL and returns it. On error it
// writes an HTTP error and returns nil.
func (h *Handler) getProvider(w http.ResponseWriter, r *http.Request) hosting.Provider {
	name := paramStr(r, "provider")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing provider name")
		return nil
	}
	p, ok := hosting.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "hosting provider "+name+" not registered")
		return nil
	}
	return p
}

// configureProvider loads saved credentials for a provider and calls Configure
// on it so it is ready for API calls. Returns false if no config is saved.
func (h *Handler) configureProvider(p hosting.Provider) bool {
	if h.HostingConfigs == nil {
		return false
	}
	cfg, err := h.HostingConfigs.Load(p.Name())
	if err != nil || cfg == nil || cfg.APIKey == "" {
		return false
	}
	if err := p.Configure(*cfg); err != nil {
		log.Printf("[hosting] configure %s: %v", p.Name(), err)
		return false
	}
	return true
}

// ─── Page Handlers ───────────────────────────────────────────────────────────

// HostingProviders renders the hosting management page (GET /hosting).
func (h *Handler) HostingProviders(w http.ResponseWriter, r *http.Request) {
	providers := h.listProviderInfo()
	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, providers)
		return
	}
	h.render(w, "hosting.html", map[string]interface{}{
		"Providers": providers,
	})
}

// HostingProviderConfig returns the config page/detail for a single provider.
func (h *Handler) HostingProviderConfig(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}

	var savedCfg *hosting.ProviderConfig
	if h.HostingConfigs != nil {
		savedCfg, _ = h.HostingConfigs.Load(p.Name())
	}

	data := map[string]interface{}{
		"Provider":  providerInfo{Name: p.Name(), DisplayName: p.DisplayName()},
		"Config":    savedCfg,
		"Providers": h.listProviderInfo(),
	}

	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, data)
		return
	}
	h.render(w, "hosting.html", data)
}

// ─── Configuration ───────────────────────────────────────────────────────────

// HostingProviderSave saves API credentials and tests the connection.
func (h *Handler) HostingProviderSave(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}

	var body struct {
		APIKey    string            `json:"api_key"`
		APISecret string            `json:"api_secret"`
		Extra     map[string]string `json:"extra"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if body.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	cfg := hosting.ProviderConfig{
		APIKey:    body.APIKey,
		APISecret: body.APISecret,
		Extra:     body.Extra,
	}

	// Configure the provider to validate credentials.
	if err := p.Configure(cfg); err != nil {
		writeError(w, http.StatusBadRequest, "configure: "+err.Error())
		return
	}

	// Test the connection.
	connected := true
	if err := p.TestConnection(); err != nil {
		log.Printf("[hosting] test %s failed: %v", p.Name(), err)
		connected = false
	}

	// Persist.
	if h.HostingConfigs != nil {
		if err := h.HostingConfigs.Save(p.Name(), cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "saved",
		"connected": connected,
	})
}

// HostingProviderTest tests the connection to a provider without saving.
func (h *Handler) HostingProviderTest(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}

	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured — save credentials first")
		return
	}

	if err := p.TestConnection(); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"connected": false,
			"error":     err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"connected": true,
	})
}

// ─── DNS ─────────────────────────────────────────────────────────────────────

// HostingDNSList returns DNS records for a domain.
func (h *Handler) HostingDNSList(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	domain := paramStr(r, "domain")
	records, err := p.ListDNSRecords(domain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, records)
}

// HostingDNSUpdate replaces DNS records for a domain.
func (h *Handler) HostingDNSUpdate(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	domain := paramStr(r, "domain")

	var body struct {
		Records   []hosting.DNSRecord `json:"records"`
		Overwrite bool                `json:"overwrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := p.UpdateDNSRecords(domain, body.Records, body.Overwrite); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HostingDNSDelete deletes DNS records matching name+type for a domain.
func (h *Handler) HostingDNSDelete(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	domain := paramStr(r, "domain")

	var body struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	filter := hosting.DNSRecordFilter{Name: body.Name, Type: body.Type}
	if err := p.DeleteDNSRecord(domain, filter); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HostingDNSReset resets DNS records for a domain to provider defaults.
func (h *Handler) HostingDNSReset(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	domain := paramStr(r, "domain")

	if err := p.ResetDNSRecords(domain); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

// ─── Domains ─────────────────────────────────────────────────────────────────

// HostingDomainsList returns all domains registered with the provider.
func (h *Handler) HostingDomainsList(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	domains, err := p.ListDomains()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

// HostingDomainsCheck checks availability of a domain across TLDs.
func (h *Handler) HostingDomainsCheck(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	var body struct {
		Domain string   `json:"domain"`
		TLDs   []string `json:"tlds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	results, err := p.CheckDomainAvailability(body.Domain, body.TLDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// HostingDomainsPurchase purchases a domain.
func (h *Handler) HostingDomainsPurchase(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	var req hosting.DomainPurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}
	if req.Years <= 0 {
		req.Years = 1
	}

	result, err := p.PurchaseDomain(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// HostingDomainNameservers updates nameservers for a domain.
func (h *Handler) HostingDomainNameservers(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	domain := paramStr(r, "domain")

	var body struct {
		Nameservers []string `json:"nameservers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.Nameservers) == 0 {
		writeError(w, http.StatusBadRequest, "nameservers list is empty")
		return
	}

	if err := p.SetNameservers(domain, body.Nameservers); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HostingDomainLock toggles the registrar lock on a domain.
func (h *Handler) HostingDomainLock(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	domain := paramStr(r, "domain")

	var body struct {
		Locked bool `json:"locked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var err error
	if body.Locked {
		err = p.EnableDomainLock(domain)
	} else {
		err = p.DisableDomainLock(domain)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "updated", "locked": body.Locked})
}

// HostingDomainPrivacy toggles privacy protection on a domain.
func (h *Handler) HostingDomainPrivacy(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	domain := paramStr(r, "domain")

	var body struct {
		Privacy bool `json:"privacy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var err error
	if body.Privacy {
		err = p.EnablePrivacyProtection(domain)
	} else {
		err = p.DisablePrivacyProtection(domain)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "updated", "privacy": body.Privacy})
}

// ─── VMs / VPS ───────────────────────────────────────────────────────────────

// HostingVMsList lists all VMs for a provider.
func (h *Handler) HostingVMsList(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	vms, err := p.ListVMs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vms)
}

// HostingVMGet returns details for a single VM.
func (h *Handler) HostingVMGet(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	id := paramStr(r, "id")
	vm, err := p.GetVM(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if vm == nil {
		writeError(w, http.StatusNotFound, "VM not found")
		return
	}
	writeJSON(w, http.StatusOK, vm)
}

// HostingVMCreate creates a new VM.
func (h *Handler) HostingVMCreate(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	var req hosting.VMCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Plan == "" {
		writeError(w, http.StatusBadRequest, "plan is required")
		return
	}
	if req.DataCenterID == "" {
		writeError(w, http.StatusBadRequest, "data_center_id is required")
		return
	}

	result, err := p.CreateVM(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// HostingDataCenters lists available data centers.
func (h *Handler) HostingDataCenters(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	dcs, err := p.ListDataCenters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dcs)
}

// ─── SSH Keys ────────────────────────────────────────────────────────────────

// HostingSSHKeys lists SSH keys for the provider account.
func (h *Handler) HostingSSHKeys(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	keys, err := p.ListSSHKeys()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// HostingSSHKeyAdd adds an SSH key.
func (h *Handler) HostingSSHKeyAdd(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	var body struct {
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Name == "" || body.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "name and public_key are required")
		return
	}

	key, err := p.AddSSHKey(body.Name, body.PublicKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

// HostingSSHKeyDelete deletes an SSH key.
func (h *Handler) HostingSSHKeyDelete(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	id := paramStr(r, "id")
	if err := p.DeleteSSHKey(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Billing ─────────────────────────────────────────────────────────────────

// HostingSubscriptions lists billing subscriptions.
func (h *Handler) HostingSubscriptions(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	subs, err := p.ListSubscriptions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

// HostingCatalog returns the product catalog.
func (h *Handler) HostingCatalog(w http.ResponseWriter, r *http.Request) {
	p := h.getProvider(w, r)
	if p == nil {
		return
	}
	if !h.configureProvider(p) {
		writeError(w, http.StatusBadRequest, "provider not configured")
		return
	}

	category := r.URL.Query().Get("category")
	items, err := p.GetCatalog(category)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}
