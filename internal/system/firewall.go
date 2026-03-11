package system

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ── Types ───────────────────────────────────────────────────────────

type UFWRule struct {
	Direction string `json:"direction"` // "in" or "out"
	Protocol  string `json:"protocol"`  // "tcp", "udp", or "" for both
	Port      string `json:"port"`      // e.g. "22", "80:90"
	Source    string `json:"source"`    // IP/CIDR or "any"/"Anywhere"
	Action    string `json:"action"`    // "allow", "deny", "reject", "limit"
	Comment   string `json:"comment"`
}

// ── Firewall (UFW) ──────────────────────────────────────────────────

// Status parses `ufw status verbose` and returns the enable state, parsed rules,
// and the raw command output.
func FirewallStatus() (enabled bool, rules []UFWRule, raw string, err error) {
	out, cmdErr := exec.Command("ufw", "status", "verbose").CombinedOutput()
	raw = string(out)
	if cmdErr != nil {
		// ufw may return non-zero when inactive; check output
		if strings.Contains(raw, "Status: inactive") {
			return false, nil, raw, nil
		}
		err = fmt.Errorf("ufw status failed: %w (%s)", cmdErr, raw)
		return
	}

	enabled = strings.Contains(raw, "Status: active")

	// Parse rule lines. After the header block, rules look like:
	//   22/tcp                     ALLOW IN    Anywhere                   # SSH
	//   80/tcp                     ALLOW IN    192.168.1.0/24             # Web
	// We find lines after the "---" separator.
	lines := strings.Split(raw, "\n")
	pastSeparator := false
	// Match: port/proto (or port)  ACTION DIRECTION  source  # optional comment
	ruleRegex := regexp.MustCompile(
		`^(\S+)\s+(ALLOW|DENY|REJECT|LIMIT)\s+(IN|OUT|FWD)?\s*(.+?)(?:\s+#\s*(.*))?$`,
	)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "---") {
			pastSeparator = true
			continue
		}
		if !pastSeparator || trimmed == "" {
			continue
		}

		matches := ruleRegex.FindStringSubmatch(trimmed)
		if matches == nil {
			continue
		}

		portProto := matches[1]
		action := strings.ToLower(matches[2])
		direction := strings.ToLower(matches[3])
		source := strings.TrimSpace(matches[4])
		comment := strings.TrimSpace(matches[5])

		if direction == "" {
			direction = "in"
		}

		// Split port/protocol
		var port, proto string
		if strings.Contains(portProto, "/") {
			parts := strings.SplitN(portProto, "/", 2)
			port = parts[0]
			proto = parts[1]
		} else {
			port = portProto
		}

		// Normalize source
		if source == "Anywhere" || source == "Anywhere (v6)" {
			source = "any"
		}

		rules = append(rules, UFWRule{
			Direction: direction,
			Protocol:  proto,
			Port:      port,
			Source:    source,
			Action:    action,
			Comment:   comment,
		})
	}

	return enabled, rules, raw, nil
}

// FirewallEnable enables UFW with --force to skip the interactive prompt.
func FirewallEnable() error {
	out, err := exec.Command("ufw", "--force", "enable").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw enable failed: %w (%s)", err, string(out))
	}
	return nil
}

// FirewallDisable disables UFW.
func FirewallDisable() error {
	out, err := exec.Command("ufw", "disable").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw disable failed: %w (%s)", err, string(out))
	}
	return nil
}

// FirewallAddRule constructs and executes a ufw command from the given rule struct.
func FirewallAddRule(rule UFWRule) error {
	if rule.Port == "" {
		return fmt.Errorf("port is required")
	}
	if rule.Action == "" {
		rule.Action = "allow"
	}
	if rule.Protocol == "" {
		rule.Protocol = "tcp"
	}
	if rule.Source == "" || rule.Source == "any" {
		rule.Source = ""
	}

	// Validate action
	switch rule.Action {
	case "allow", "deny", "reject", "limit":
		// valid
	default:
		return fmt.Errorf("invalid action %q: must be allow, deny, reject, or limit", rule.Action)
	}

	// Validate protocol
	switch rule.Protocol {
	case "tcp", "udp":
		// valid
	default:
		return fmt.Errorf("invalid protocol %q: must be tcp or udp", rule.Protocol)
	}

	// Validate direction
	if rule.Direction != "" && rule.Direction != "in" && rule.Direction != "out" {
		return fmt.Errorf("invalid direction %q: must be in or out", rule.Direction)
	}

	// Build argument list
	args := []string{rule.Action}

	// Direction
	if rule.Direction == "out" {
		args = append(args, "out")
	}

	// Source filter
	if rule.Source != "" {
		args = append(args, "from", rule.Source)
	}

	args = append(args, "to", "any", "port", rule.Port, "proto", rule.Protocol)

	// Comment
	if rule.Comment != "" {
		args = append(args, "comment", rule.Comment)
	}

	out, err := exec.Command("ufw", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw add rule failed: %w (%s)", err, string(out))
	}
	return nil
}

// FirewallDeleteRule constructs and executes a ufw delete command for the given rule.
func FirewallDeleteRule(rule UFWRule) error {
	if rule.Port == "" {
		return fmt.Errorf("port is required")
	}
	if rule.Action == "" {
		rule.Action = "allow"
	}

	// Build the rule specification that matches what was added
	args := []string{"delete", rule.Action}

	if rule.Direction == "out" {
		args = append(args, "out")
	}

	if rule.Source != "" && rule.Source != "any" {
		args = append(args, "from", rule.Source)
	}

	portSpec := rule.Port
	if rule.Protocol != "" {
		portSpec = rule.Port + "/" + rule.Protocol
	}
	args = append(args, portSpec)

	out, err := exec.Command("ufw", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw delete rule failed: %w (%s)", err, string(out))
	}
	return nil
}

// FirewallSetDefaults sets the default incoming and outgoing policies.
// Valid values are "allow", "deny", "reject".
func FirewallSetDefaults(incoming, outgoing string) error {
	validPolicy := map[string]bool{"allow": true, "deny": true, "reject": true}

	if incoming != "" {
		if !validPolicy[incoming] {
			return fmt.Errorf("invalid incoming policy %q: must be allow, deny, or reject", incoming)
		}
		out, err := exec.Command("ufw", "default", incoming, "incoming").CombinedOutput()
		if err != nil {
			return fmt.Errorf("setting default incoming policy failed: %w (%s)", err, string(out))
		}
	}

	if outgoing != "" {
		if !validPolicy[outgoing] {
			return fmt.Errorf("invalid outgoing policy %q: must be allow, deny, or reject", outgoing)
		}
		out, err := exec.Command("ufw", "default", outgoing, "outgoing").CombinedOutput()
		if err != nil {
			return fmt.Errorf("setting default outgoing policy failed: %w (%s)", err, string(out))
		}
	}

	return nil
}
