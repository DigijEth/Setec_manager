package handlers

import (
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"setec-manager/internal/deploy"
)

func (h *Handler) LogsPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "logs.html", nil)
}

func (h *Handler) LogsSystem(w http.ResponseWriter, r *http.Request) {
	linesStr := r.URL.Query().Get("lines")
	if linesStr == "" {
		linesStr = "100"
	}
	lines, err := strconv.Atoi(linesStr)
	if err != nil {
		lines = 100
	}

	// deploy.Logs requires a unit name; for system-wide logs we pass an empty
	// unit and use journalctl directly. However, deploy.Logs always passes -u,
	// so we use it with a broad scope by requesting the system journal for a
	// pseudo-unit. Instead, keep using journalctl directly for system-wide logs
	// since deploy.Logs is unit-scoped.
	out, err := exec.Command("journalctl", "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso").Output()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": string(out)})
}

func (h *Handler) LogsNginx(w http.ResponseWriter, r *http.Request) {
	logType := r.URL.Query().Get("type")
	if logType == "" {
		logType = "access"
	}

	var logPath string
	switch logType {
	case "access":
		logPath = "/var/log/nginx/access.log"
	case "error":
		logPath = "/var/log/nginx/error.log"
	default:
		writeError(w, http.StatusBadRequest, "invalid log type")
		return
	}

	out, err := exec.Command("tail", "-n", "200", logPath).Output()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": string(out), "type": logType})
}

func (h *Handler) LogsUnit(w http.ResponseWriter, r *http.Request) {
	unit := r.URL.Query().Get("unit")
	if unit == "" {
		writeError(w, http.StatusBadRequest, "unit parameter required")
		return
	}
	linesStr := r.URL.Query().Get("lines")
	if linesStr == "" {
		linesStr = "100"
	}
	lines, err := strconv.Atoi(linesStr)
	if err != nil {
		lines = 100
	}

	out, err := deploy.Logs(unit, lines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": out, "unit": unit})
}

func (h *Handler) LogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	unit := r.URL.Query().Get("unit")
	if unit == "" {
		unit = "autarch-web"
	}

	// SSE live streaming requires journalctl -f which the deploy package does
	// not support (it only returns a snapshot). Keep inline exec.Command here.
	cmd := exec.Command("journalctl", "-u", unit, "-f", "-n", "0", "--no-pager", "-o", "short-iso")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Start()
	defer cmd.Process.Kill()

	buf := make([]byte, 4096)
	for {
		select {
		case <-r.Context().Done():
			return
		default:
			n, err := stdout.Read(buf)
			if err != nil {
				return
			}
			if n > 0 {
				lines := strings.Split(strings.TrimSpace(string(buf[:n])), "\n")
				for _, line := range lines {
					fmt.Fprintf(w, "data: %s\n\n", line)
				}
				flusher.Flush()
			}
		}
	}
}
