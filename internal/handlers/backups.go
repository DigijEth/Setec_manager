package handlers

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func (h *Handler) BackupList(w http.ResponseWriter, r *http.Request) {
	backups, err := h.DB.ListBackups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, backups)
		return
	}
	h.render(w, "backups.html", backups)
}

func (h *Handler) BackupSite(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	site, err := h.DB.GetSite(id)
	if err != nil || site == nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	// Create backup directory
	backupDir := h.Config.Backups.Dir
	os.MkdirAll(backupDir, 0755)

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("site-%s-%s.tar.gz", site.Domain, timestamp)
	backupPath := filepath.Join(backupDir, filename)

	// Create tar.gz
	cmd := exec.Command("tar", "-czf", backupPath, "-C", filepath.Dir(site.AppRoot), filepath.Base(site.AppRoot))
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("backup failed: %s", string(out)))
		return
	}

	// Get file size
	info, _ := os.Stat(backupPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	bID, _ := h.DB.CreateBackup(&id, "site", backupPath, size)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":   bID,
		"path": backupPath,
		"size": size,
	})
}

func (h *Handler) BackupFull(w http.ResponseWriter, r *http.Request) {
	backupDir := h.Config.Backups.Dir
	os.MkdirAll(backupDir, 0755)

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("full-system-%s.tar.gz", timestamp)
	backupPath := filepath.Join(backupDir, filename)

	// Backup key directories
	dirs := []string{
		h.Config.Nginx.Webroot,
		"/etc/nginx",
		"/opt/setec-manager/data",
	}

	args := []string{"-czf", backupPath}
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			args = append(args, d)
		}
	}

	cmd := exec.Command("tar", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("backup failed: %s", string(out)))
		return
	}

	info, _ := os.Stat(backupPath)
	size := int64(0)
	if info != nil {
		size = info.Size()
	}

	bID, _ := h.DB.CreateBackup(nil, "full", backupPath, size)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":   bID,
		"path": backupPath,
		"size": size,
	})
}

func (h *Handler) BackupDelete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Get backup info to delete file
	var filePath string
	h.DB.Conn().QueryRow(`SELECT file_path FROM backups WHERE id=?`, id).Scan(&filePath)
	if filePath != "" {
		os.Remove(filePath)
	}

	h.DB.DeleteBackup(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) BackupDownload(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var filePath string
	h.DB.Conn().QueryRow(`SELECT file_path FROM backups WHERE id=?`, id).Scan(&filePath)
	if filePath == "" {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(filePath)))
	http.ServeFile(w, r, filePath)
}
