package handlers

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"setec-manager/internal/deploy"
	"setec-manager/internal/system"
)

type systemInfo struct {
	Hostname    string        `json:"hostname"`
	OS          string        `json:"os"`
	Arch        string        `json:"arch"`
	CPUs        int           `json:"cpus"`
	Uptime      string        `json:"uptime"`
	LoadAvg     string        `json:"load_avg"`
	MemTotal    string        `json:"mem_total"`
	MemUsed     string        `json:"mem_used"`
	MemPercent  float64       `json:"mem_percent"`
	DiskTotal   string        `json:"disk_total"`
	DiskUsed    string        `json:"disk_used"`
	DiskPercent float64       `json:"disk_percent"`
	SiteCount   int           `json:"site_count"`
	Services    []serviceInfo `json:"services"`
}

type serviceInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Running bool   `json:"running"`
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	info := h.gatherSystemInfo()
	h.render(w, "dashboard.html", info)
}

func (h *Handler) SystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.gatherSystemInfo())
}

func (h *Handler) gatherSystemInfo() systemInfo {
	info := systemInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
		CPUs: runtime.NumCPU(),
	}

	// Hostname — no wrapper, keep exec.Command
	if out, err := exec.Command("hostname").Output(); err == nil {
		info.Hostname = strings.TrimSpace(string(out))
	}

	// Uptime
	if ut, err := system.GetUptime(); err == nil {
		info.Uptime = "up " + ut.HumanReadable
	}

	// Load average
	if la, err := system.GetLoadAvg(); err == nil {
		info.LoadAvg = fmt.Sprintf("%.2f %.2f %.2f", la.Load1, la.Load5, la.Load15)
	}

	// Memory
	if mem, err := system.GetMemory(); err == nil {
		info.MemTotal = mem.Total
		info.MemUsed = mem.Used
		if mem.TotalBytes > 0 {
			info.MemPercent = float64(mem.UsedBytes) / float64(mem.TotalBytes) * 100
		}
	}

	// Disk — find the root mount from the disk list
	if disks, err := system.GetDisk(); err == nil {
		for _, d := range disks {
			if d.MountPoint == "/" {
				info.DiskTotal = d.Size
				info.DiskUsed = d.Used
				pct := strings.TrimSuffix(d.UsePercent, "%")
				info.DiskPercent, _ = strconv.ParseFloat(pct, 64)
				break
			}
		}
		// If no root mount found but we have disks, use the first one
		if info.DiskTotal == "" && len(disks) > 0 {
			d := disks[0]
			info.DiskTotal = d.Size
			info.DiskUsed = d.Used
			pct := strings.TrimSuffix(d.UsePercent, "%")
			info.DiskPercent, _ = strconv.ParseFloat(pct, 64)
		}
	}

	// Site count
	if sites, err := h.DB.ListSites(); err == nil {
		info.SiteCount = len(sites)
	}

	// Services
	services := []struct{ name, unit string }{
		{"Nginx", "nginx"},
		{"AUTARCH Web", "autarch-web"},
		{"AUTARCH DNS", "autarch-dns"},
		{"Setec Manager", "setec-manager"},
	}
	for _, svc := range services {
		si := serviceInfo{Name: svc.name}
		active, err := deploy.IsActive(svc.unit)
		if err == nil && active {
			si.Status = "active"
			si.Running = true
		} else {
			si.Status = "inactive"
			si.Running = false
		}
		info.Services = append(info.Services, si)
	}

	return info
}

func formatBytes(b float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	return strconv.FormatFloat(b, 'f', 1, 64) + " " + units[i]
}

// uptimeSince returns a human-readable duration.
func uptimeSince(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return strconv.Itoa(days) + "d " + strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
	}
	if hours > 0 {
		return strconv.Itoa(hours) + "h " + strconv.Itoa(mins) + "m"
	}
	return strconv.Itoa(mins) + "m"
}
