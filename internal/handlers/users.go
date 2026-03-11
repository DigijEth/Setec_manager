package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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
	sysUsers := listSystemUsers()

	panelUsers, _ := h.DB.ListManagerUsers()
	groups, _ := h.DB.ListGroups()

	// Attach group info per panel user
	type panelUserView struct {
		ID          int64    `json:"id"`
		Username    string   `json:"username"`
		Role        string   `json:"role"`
		Groups      []string `json:"groups"`
		DomainCount int      `json:"domain_count"`
		CreatedAt   string   `json:"created_at"`
		LastLogin   string   `json:"last_login,omitempty"`
	}
	var panelViews []panelUserView
	for _, u := range panelUsers {
		pv := panelUserView{
			ID:        u.ID,
			Username:  u.Username,
			Role:      u.Role,
			CreatedAt: u.CreatedAt.Format("2006-01-02 15:04"),
		}
		if u.LastLogin != nil {
			pv.LastLogin = u.LastLogin.Format("2006-01-02 15:04")
		}
		uGroups, _ := h.DB.GetUserGroups(u.ID)
		for _, g := range uGroups {
			pv.Groups = append(pv.Groups, g.DisplayName)
		}
		accesses, _ := h.DB.GetUserDomainAccess(u.ID)
		pv.DomainCount = len(accesses)
		panelViews = append(panelViews, pv)
	}

	// Group views with member counts
	type groupView struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
		IsSystem    bool   `json:"is_system"`
		MemberCount int    `json:"member_count"`
		CreatedAt   string `json:"created_at"`
	}
	var groupViews []groupView
	for _, g := range groups {
		gv := groupView{
			ID:          g.ID,
			Name:        g.Name,
			DisplayName: g.DisplayName,
			Description: g.Description,
			IsSystem:    g.IsSystem,
			CreatedAt:   g.CreatedAt,
		}
		gv.MemberCount, _ = h.DB.GroupMemberCount(g.ID)
		groupViews = append(groupViews, gv)
	}

	data := map[string]interface{}{
		"SystemUsers": sysUsers,
		"PanelUsers":  panelViews,
		"Groups":      groupViews,
	}

	if acceptsJSON(r) {
		writeJSON(w, http.StatusOK, data)
		return
	}
	h.render(w, "users.html", data)
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
		Username string  `json:"username"`
		Password string  `json:"password"`
		Email    string  `json:"email"`
		Role     string  `json:"role"`
		GroupIDs []int64 `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.Username = r.FormValue("username")
		body.Password = r.FormValue("password")
		body.Email = r.FormValue("email")
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

	// Assign groups if provided
	if len(body.GroupIDs) > 0 {
		h.DB.SetUserGroups(id, body.GroupIDs)
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
		Email    string `json:"email"`
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

// PanelUserGroups returns the groups a user belongs to.
func (h *Handler) PanelUserGroups(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	groups, err := h.DB.GetUserGroups(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

// PanelUserSetGroups replaces all group memberships for a user.
func (h *Handler) PanelUserSetGroups(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		GroupIDs []int64 `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.DB.SetUserGroups(id, body.GroupIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// PanelUserDomainAccess returns domain access entries for a user.
func (h *Handler) PanelUserDomainAccess(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	accesses, err := h.DB.GetUserDomainAccess(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, accesses)
}

// PanelUserGrantDomain grants domain access to a user.
func (h *Handler) PanelUserGrantDomain(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		SiteID        int64  `json:"site_id"`
		DomainPattern string `json:"domain_pattern"`
		AccessLevel   string `json:"access_level"`
		GrantedBy     int64  `json:"granted_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.AccessLevel == "" {
		body.AccessLevel = "read"
	}
	if body.GrantedBy == 0 {
		body.GrantedBy = 1 // default to admin user
	}
	if err := h.DB.GrantDomainAccess(id, body.SiteID, body.DomainPattern, body.AccessLevel, body.GrantedBy); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "granted"})
}

// PanelUserRevokeDomain revokes a domain access entry.
func (h *Handler) PanelUserRevokeDomain(w http.ResponseWriter, r *http.Request) {
	accessIDStr := paramStr(r, "accessId")
	accessID, err := strconv.ParseInt(accessIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid access ID")
		return
	}
	if err := h.DB.RevokeDomainAccess(accessID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// ── Group Management ────────────────────────────────────────────────

// GroupList returns all groups with member counts.
func (h *Handler) GroupList(w http.ResponseWriter, r *http.Request) {
	groups, err := h.DB.ListGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type groupView struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
		IsSystem    bool   `json:"is_system"`
		MemberCount int    `json:"member_count"`
		CreatedAt   string `json:"created_at"`
	}
	var views []groupView
	for _, g := range groups {
		gv := groupView{
			ID:          g.ID,
			Name:        g.Name,
			DisplayName: g.DisplayName,
			Description: g.Description,
			IsSystem:    g.IsSystem,
			CreatedAt:   g.CreatedAt,
		}
		gv.MemberCount, _ = h.DB.GroupMemberCount(g.ID)
		views = append(views, gv)
	}
	writeJSON(w, http.StatusOK, views)
}

// GroupCreate creates a new custom group.
func (h *Handler) GroupCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" || body.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "name and display_name are required")
		return
	}
	id, err := h.DB.CreateGroup(body.Name, body.DisplayName, body.Description, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "name": body.Name})
}

// GroupUpdate updates a group's display name and description.
func (h *Handler) GroupUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.DB.UpdateGroup(id, body.DisplayName, body.Description); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// GroupDelete deletes a custom group (refuses system groups).
func (h *Handler) GroupDelete(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.DB.DeleteGroup(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GroupPermissions returns the permissions assigned to a group.
func (h *Handler) GroupPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	perms, err := h.DB.GetGroupPermissions(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, perms)
}

// GroupSetPermissions replaces all permissions for a group.
func (h *Handler) GroupSetPermissions(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		PermissionIDs []int64 `json:"permission_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.DB.SetGroupPermissions(id, body.PermissionIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// GroupMembers returns the members of a group.
func (h *Handler) GroupMembers(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	members, err := h.DB.GetGroupMembers(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}
