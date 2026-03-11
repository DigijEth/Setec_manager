package handlers

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"setec-manager/internal/deploy"
	"setec-manager/internal/system"
)

func (h *Handler) MonitorPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "monitor.html", nil)
}

func (h *Handler) MonitorCPU(w http.ResponseWriter, r *http.Request) {
	cpu, err := system.GetCPUUsage()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"error": err.Error()})
		return
	}

	// Build a summary line matching the previous top-style format.
	sysPct := 0.0
	userPct := cpu.Overall
	if len(cpu.Cores) > 0 {
		// Use aggregate core data for a more accurate breakdown
		var totalUser, totalSys float64
		for _, c := range cpu.Cores {
			totalUser += c.User
			totalSys += c.System
		}
		userPct = totalUser / float64(len(cpu.Cores))
		sysPct = totalSys / float64(len(cpu.Cores))
	}
	cpuLine := fmt.Sprintf("%%Cpu(s): %.1f us, %.1f sy, %.1f id",
		userPct, sysPct, cpu.Idle)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"cpu":     cpuLine,
		"overall": cpu.Overall,
		"idle":    cpu.Idle,
		"cores":   cpu.Cores,
	})
}

func (h *Handler) MonitorMemory(w http.ResponseWriter, r *http.Request) {
	mem, err := system.GetMemory()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"error": err.Error()})
		return
	}

	result := map[string]interface{}{
		"total":      mem.Total,
		"used":       mem.Used,
		"free":       mem.Free,
		"available":  mem.Available,
		"swap_total": mem.SwapTotal,
		"swap_used":  mem.SwapUsed,
		"swap_free":  mem.SwapFree,
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) MonitorDisk(w http.ResponseWriter, r *http.Request) {
	disks, err := system.GetDisk()
	if err != nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	writeJSON(w, http.StatusOK, disks)
}

func (h *Handler) MonitorServices(w http.ResponseWriter, r *http.Request) {
	services := []string{"nginx", "autarch-web", "autarch-dns", "setec-manager", "ufw"}

	type svcStatus struct {
		Name    string `json:"name"`
		Active  string `json:"active"`
		Running bool   `json:"running"`
		Memory  string `json:"memory"`
	}

	var statuses []svcStatus
	for _, svc := range services {
		ss := svcStatus{Name: svc}
		active, err := deploy.IsActive(svc)
		if err == nil && active {
			ss.Active = "active"
			ss.Running = true
		} else {
			ss.Active = "inactive"
			ss.Running = false
		}

		// Get memory usage — no wrapper exists for this property, so use exec
		if ss.Running {
			out, err := exec.Command("systemctl", "show", svc, "--property=MemoryCurrent").Output()
			if err == nil {
				parts := strings.SplitN(string(out), "=", 2)
				if len(parts) == 2 {
					val := strings.TrimSpace(parts[1])
					if val != "[not set]" && val != "" {
						bytes := parseUint64(val)
						ss.Memory = formatBytes(float64(bytes))
					}
				}
			}
		}

		statuses = append(statuses, ss)
	}

	writeJSON(w, http.StatusOK, statuses)
}

// parseUint64 is a helper that returns 0 on failure.
func parseUint64(s string) uint64 {
	var n uint64
	fmt.Sscanf(s, "%d", &n)
	return n
}
