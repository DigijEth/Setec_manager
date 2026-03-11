package handlers

import (
	"encoding/json"
	"net/http"

	"setec-manager/internal/system"
)

type firewallRule struct {
	ID        int64  `json:"id"`
	Direction string `json:"direction"`
	Protocol  string `json:"protocol"`
	Port      string `json:"port"`
	Source    string `json:"source"`
	Action    string `json:"action"`
	Comment   string `json:"comment"`
}

type firewallStatus struct {
	Enabled bool           `json:"enabled"`
	Rules   []firewallRule `json:"rules"`
	UFWOut  string         `json:"ufw_output"`
}

func (h *Handler) FirewallList(w http.ResponseWriter, r *http.Request) {
	status := h.getFirewallStatus()
	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, status)
		return
	}
	h.render(w, "firewall.html", status)
}

func (h *Handler) FirewallStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.getFirewallStatus())
}

func (h *Handler) FirewallAddRule(w http.ResponseWriter, r *http.Request) {
	var rule firewallRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		rule.Port = r.FormValue("port")
		rule.Protocol = r.FormValue("protocol")
		rule.Source = r.FormValue("source")
		rule.Action = r.FormValue("action")
		rule.Comment = r.FormValue("comment")
	}

	if rule.Port == "" {
		writeError(w, http.StatusBadRequest, "port is required")
		return
	}
	if rule.Protocol == "" {
		rule.Protocol = "tcp"
	}
	if rule.Action == "" {
		rule.Action = "allow"
	}
	if rule.Source == "" {
		rule.Source = "any"
	}

	ufwRule := system.UFWRule{
		Port:     rule.Port,
		Protocol: rule.Protocol,
		Source:   rule.Source,
		Action:   rule.Action,
		Comment:  rule.Comment,
	}

	if err := system.FirewallAddRule(ufwRule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Save to DB
	h.DB.Conn().Exec(`INSERT INTO firewall_rules (direction, protocol, port, source, action, comment)
		VALUES (?, ?, ?, ?, ?, ?)`, "in", rule.Protocol, rule.Port, rule.Source, rule.Action, rule.Comment)

	writeJSON(w, http.StatusCreated, map[string]string{"status": "rule added"})
}

func (h *Handler) FirewallDeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Get rule from DB to build delete command
	var port, protocol, action string
	err = h.DB.Conn().QueryRow(`SELECT port, protocol, action FROM firewall_rules WHERE id=?`, id).
		Scan(&port, &protocol, &action)
	if err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	system.FirewallDeleteRule(system.UFWRule{
		Port:     port,
		Protocol: protocol,
		Action:   action,
	})
	h.DB.Conn().Exec(`DELETE FROM firewall_rules WHERE id=?`, id)

	writeJSON(w, http.StatusOK, map[string]string{"status": "rule deleted"})
}

func (h *Handler) FirewallEnable(w http.ResponseWriter, r *http.Request) {
	if err := system.FirewallEnable(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled"})
}

func (h *Handler) FirewallDisable(w http.ResponseWriter, r *http.Request) {
	if err := system.FirewallDisable(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func (h *Handler) getFirewallStatus() firewallStatus {
	status := firewallStatus{}

	enabled, _, raw, _ := system.FirewallStatus()
	status.UFWOut = raw
	status.Enabled = enabled

	// Load rules from DB
	rows, err := h.DB.Conn().Query(`SELECT id, direction, protocol, port, source, action, comment
		FROM firewall_rules WHERE enabled=TRUE ORDER BY id`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var rule firewallRule
			rows.Scan(&rule.ID, &rule.Direction, &rule.Protocol, &rule.Port,
				&rule.Source, &rule.Action, &rule.Comment)
			status.Rules = append(status.Rules, rule)
		}
	}

	return status
}

func (h *Handler) InstallDefaultFirewall() error {
	// Set default policies
	system.FirewallSetDefaults("deny", "allow")

	// Add default rules
	defaultRules := []system.UFWRule{
		{Port: "22", Protocol: "tcp", Action: "allow", Comment: "SSH"},
		{Port: "80", Protocol: "tcp", Action: "allow", Comment: "HTTP"},
		{Port: "443", Protocol: "tcp", Action: "allow", Comment: "HTTPS"},
		{Port: "9090", Protocol: "tcp", Action: "allow", Comment: "Setec Manager"},
		{Port: "8181", Protocol: "tcp", Action: "allow", Comment: "AUTARCH Web"},
		{Port: "53", Protocol: "", Action: "allow", Comment: "AUTARCH DNS"},
	}

	for _, rule := range defaultRules {
		system.FirewallAddRule(rule)
	}

	// Enable the firewall
	system.FirewallEnable()

	// Record in DB
	dbRules := []firewallRule{
		{Port: "22", Protocol: "tcp", Action: "allow", Comment: "SSH"},
		{Port: "80", Protocol: "tcp", Action: "allow", Comment: "HTTP"},
		{Port: "443", Protocol: "tcp", Action: "allow", Comment: "HTTPS"},
		{Port: "9090", Protocol: "tcp", Action: "allow", Comment: "Setec Manager"},
		{Port: "8181", Protocol: "tcp", Action: "allow", Comment: "AUTARCH Web"},
		{Port: "53", Protocol: "tcp", Action: "allow", Comment: "AUTARCH DNS"},
	}
	for _, rule := range dbRules {
		h.DB.Conn().Exec(`INSERT OR IGNORE INTO firewall_rules (direction, protocol, port, source, action, comment)
			VALUES ('in', ?, ?, 'any', ?, ?)`, rule.Protocol, rule.Port, rule.Action, rule.Comment)
	}

	return nil
}
