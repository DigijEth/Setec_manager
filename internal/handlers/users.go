package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"setec-manager/internal/system"
)

// ── System Users ────────────────────────────────────────────────────

type sysUser struct {
	Username string `json:"username"`
	UID      string `json:"uid"`
	HomeDir  string `json:"home_dir"`
	Shell    string `json:"shell"`
}

func (h *Handler) UserList(w http.ResponseWriter, r *http.Request) {
	users := listSystemUsers()
	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, users)
		return
	}
	h.render(w, "users.html", users)
}

func (h *Handler) UserCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Shell    string `json:"shell"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.Username = r.FormValue("username")
		body.Password = r.FormValue("password")
		body.Shell = r.FormValue("shell")
	}

	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if body.Shell == "" {
		body.Shell = "/bin/bash"
	}

	if err := system.CreateUser(body.Username, body.Password, body.Shell); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create user failed: %s", err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "username": body.Username})
}

func (h *Handler) UserDelete(w http.ResponseWriter, r *http.Request) {
	id := paramStr(r, "id") // actually username for system users
	if id == "" {
		writeError(w, http.StatusBadRequest, "username required")
		return
	}

	// Safety check
	if id == "root" || id == "autarch" {
		writeError(w, http.StatusForbidden, "cannot delete system accounts")
		return
	}

	if err := system.DeleteUser(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func listSystemUsers() []sysUser {
	systemUsers, err := system.ListUsers()
	if err != nil {
		return nil
	}

	var users []sysUser
	for _, su := range systemUsers {
		users = append(users, sysUser{
			Username: su.Username,
			UID:      fmt.Sprintf("%d", su.UID),
			HomeDir:  su.HomeDir,
			Shell:    su.Shell,
		})
	}
	return users
}

// ── Panel Users ─────────────────────────────────────────────────────

func (h *Handler) PanelUserList(w http.ResponseWriter, r *http.Request) {
	users, err := h.DB.ListManagerUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, users)
		return
	}
	h.render(w, "users.html", map[string]interface{}{"PanelUsers": users})
}

func (h *Handler) PanelUserCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.Username = r.FormValue("username")
		body.Password = r.FormValue("password")
		body.Role = r.FormValue("role")
	}

	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	if body.Role == "" {
		body.Role = "admin"
	}

	id, err := h.DB.CreateManagerUser(body.Username, body.Password, body.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "username": body.Username})
}

func (h *Handler) PanelUserUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body struct {
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if body.Password != "" {
		h.DB.UpdateManagerUserPassword(id, body.Password)
	}
	if body.Role != "" {
		h.DB.UpdateManagerUserRole(id, body.Role)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) PanelUserDelete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.DB.DeleteManagerUser(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
