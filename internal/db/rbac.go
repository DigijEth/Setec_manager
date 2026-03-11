package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// ── RBAC Models ─────────────────────────────────────────────────────

type UserGroup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	IsSystem    bool   `json:"is_system"`
	CreatedAt   string `json:"created_at"`
}

type Permission struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

type DomainAccess struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"user_id"`
	SiteID        *int64 `json:"site_id,omitempty"`
	DomainPattern string `json:"domain_pattern,omitempty"`
	AccessLevel   string `json:"access_level"`
	GrantedBy     int64  `json:"granted_by"`
	GrantedAt     string `json:"granted_at"`
}

// ── Migration SQL ───────────────────────────────────────────────────

const migrateUserGroups = `CREATE TABLE IF NOT EXISTS user_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    is_system BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const migratePermissions = `CREATE TABLE IF NOT EXISTS permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    category TEXT NOT NULL,
    description TEXT
);`

const migrateGroupPermissions = `CREATE TABLE IF NOT EXISTS group_permissions (
    group_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL,
    PRIMARY KEY (group_id, permission_id),
    FOREIGN KEY (group_id) REFERENCES user_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);`

const migrateUserGroupMembership = `CREATE TABLE IF NOT EXISTS user_group_membership (
    user_id INTEGER NOT NULL,
    group_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, group_id),
    FOREIGN KEY (user_id) REFERENCES manager_users(id) ON DELETE CASCADE,
    FOREIGN KEY (group_id) REFERENCES user_groups(id) ON DELETE CASCADE
);`

const migrateUserDomainAccess = `CREATE TABLE IF NOT EXISTS user_domain_access (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    site_id INTEGER,
    domain_pattern TEXT,
    access_level TEXT NOT NULL DEFAULT 'read',
    granted_by INTEGER,
    granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES manager_users(id) ON DELETE CASCADE,
    FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE,
    FOREIGN KEY (granted_by) REFERENCES manager_users(id)
);`

// ── Group CRUD ──────────────────────────────────────────────────────

func (d *DB) CreateGroup(name, displayName, description string, isSystem bool) (int64, error) {
	result, err := d.conn.Exec(
		`INSERT INTO user_groups (name, display_name, description, is_system) VALUES (?, ?, ?, ?)`,
		name, displayName, description, isSystem,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) GetGroup(id int64) (*UserGroup, error) {
	var g UserGroup
	var desc sql.NullString
	err := d.conn.QueryRow(
		`SELECT id, name, display_name, description, is_system, created_at FROM user_groups WHERE id = ?`, id,
	).Scan(&g.ID, &g.Name, &g.DisplayName, &desc, &g.IsSystem, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.Description = desc.String
	return &g, nil
}

func (d *DB) GetGroupByName(name string) (*UserGroup, error) {
	var g UserGroup
	var desc sql.NullString
	err := d.conn.QueryRow(
		`SELECT id, name, display_name, description, is_system, created_at FROM user_groups WHERE name = ?`, name,
	).Scan(&g.ID, &g.Name, &g.DisplayName, &desc, &g.IsSystem, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.Description = desc.String
	return &g, nil
}

func (d *DB) ListGroups() ([]UserGroup, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, display_name, description, is_system, created_at FROM user_groups ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []UserGroup
	for rows.Next() {
		var g UserGroup
		var desc sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &g.DisplayName, &desc, &g.IsSystem, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.Description = desc.String
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (d *DB) UpdateGroup(id int64, displayName, description string) error {
	_, err := d.conn.Exec(
		`UPDATE user_groups SET display_name = ?, description = ? WHERE id = ?`,
		displayName, description, id,
	)
	return err
}

func (d *DB) DeleteGroup(id int64) error {
	// Refuse to delete system groups
	var isSystem bool
	err := d.conn.QueryRow(`SELECT is_system FROM user_groups WHERE id = ?`, id).Scan(&isSystem)
	if err != nil {
		return fmt.Errorf("group not found: %w", err)
	}
	if isSystem {
		return fmt.Errorf("cannot delete system group")
	}
	_, err = d.conn.Exec(`DELETE FROM user_groups WHERE id = ?`, id)
	return err
}

// ── Permission CRUD ─────────────────────────────────────────────────

func (d *DB) CreatePermission(name, displayName, category, description string) (int64, error) {
	result, err := d.conn.Exec(
		`INSERT INTO permissions (name, display_name, category, description) VALUES (?, ?, ?, ?)`,
		name, displayName, category, description,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) ListPermissions() ([]Permission, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, display_name, category, description FROM permissions ORDER BY category, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		var desc sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.DisplayName, &p.Category, &desc); err != nil {
			return nil, err
		}
		p.Description = desc.String
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func (d *DB) ListPermissionsByCategory() (map[string][]Permission, error) {
	perms, err := d.ListPermissions()
	if err != nil {
		return nil, err
	}
	result := make(map[string][]Permission)
	for _, p := range perms {
		result[p.Category] = append(result[p.Category], p)
	}
	return result, nil
}

// ── Group-Permission Assignments ────────────────────────────────────

func (d *DB) AssignPermissionToGroup(groupID, permissionID int64) error {
	_, err := d.conn.Exec(
		`INSERT OR IGNORE INTO group_permissions (group_id, permission_id) VALUES (?, ?)`,
		groupID, permissionID,
	)
	return err
}

func (d *DB) RemovePermissionFromGroup(groupID, permissionID int64) error {
	_, err := d.conn.Exec(
		`DELETE FROM group_permissions WHERE group_id = ? AND permission_id = ?`,
		groupID, permissionID,
	)
	return err
}

func (d *DB) GetGroupPermissions(groupID int64) ([]Permission, error) {
	rows, err := d.conn.Query(
		`SELECT p.id, p.name, p.display_name, p.category, p.description
		 FROM permissions p
		 JOIN group_permissions gp ON gp.permission_id = p.id
		 WHERE gp.group_id = ?
		 ORDER BY p.category, p.name`, groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		var desc sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.DisplayName, &p.Category, &desc); err != nil {
			return nil, err
		}
		p.Description = desc.String
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// ── User-Group Membership ───────────────────────────────────────────

func (d *DB) AddUserToGroup(userID, groupID int64) error {
	_, err := d.conn.Exec(
		`INSERT OR IGNORE INTO user_group_membership (user_id, group_id) VALUES (?, ?)`,
		userID, groupID,
	)
	return err
}

func (d *DB) RemoveUserFromGroup(userID, groupID int64) error {
	_, err := d.conn.Exec(
		`DELETE FROM user_group_membership WHERE user_id = ? AND group_id = ?`,
		userID, groupID,
	)
	return err
}

func (d *DB) GetUserGroups(userID int64) ([]UserGroup, error) {
	rows, err := d.conn.Query(
		`SELECT g.id, g.name, g.display_name, g.description, g.is_system, g.created_at
		 FROM user_groups g
		 JOIN user_group_membership ugm ON ugm.group_id = g.id
		 WHERE ugm.user_id = ?
		 ORDER BY g.name`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []UserGroup
	for rows.Next() {
		var g UserGroup
		var desc sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &g.DisplayName, &desc, &g.IsSystem, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.Description = desc.String
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (d *DB) GetGroupMembers(groupID int64) ([]ManagerUser, error) {
	rows, err := d.conn.Query(
		`SELECT u.id, u.username, u.password_hash, u.role, u.force_change, u.last_login, u.created_at
		 FROM manager_users u
		 JOIN user_group_membership ugm ON ugm.user_id = u.id
		 WHERE ugm.group_id = ?
		 ORDER BY u.username`, groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []ManagerUser
	for rows.Next() {
		var u ManagerUser
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role,
			&u.ForceChange, &u.LastLogin, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ── Permission Checking ─────────────────────────────────────────────

func (d *DB) UserHasPermission(userID int64, permissionName string) (bool, error) {
	var count int
	err := d.conn.QueryRow(
		`SELECT COUNT(*) FROM permissions p
		 JOIN group_permissions gp ON gp.permission_id = p.id
		 JOIN user_group_membership ugm ON ugm.group_id = gp.group_id
		 WHERE ugm.user_id = ? AND p.name = ?`,
		userID, permissionName,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *DB) GetUserPermissions(userID int64) ([]Permission, error) {
	rows, err := d.conn.Query(
		`SELECT DISTINCT p.id, p.name, p.display_name, p.category, p.description
		 FROM permissions p
		 JOIN group_permissions gp ON gp.permission_id = p.id
		 JOIN user_group_membership ugm ON ugm.group_id = gp.group_id
		 WHERE ugm.user_id = ?
		 ORDER BY p.category, p.name`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		var desc sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.DisplayName, &p.Category, &desc); err != nil {
			return nil, err
		}
		p.Description = desc.String
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// ── Domain Access ───────────────────────────────────────────────────

func (d *DB) GrantDomainAccess(userID, siteID int64, domainPattern, accessLevel string, grantedBy int64) error {
	var siteIDPtr *int64
	if siteID > 0 {
		siteIDPtr = &siteID
	}
	var patternPtr *string
	if domainPattern != "" {
		patternPtr = &domainPattern
	}

	_, err := d.conn.Exec(
		`INSERT INTO user_domain_access (user_id, site_id, domain_pattern, access_level, granted_by)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, siteIDPtr, patternPtr, accessLevel, grantedBy,
	)
	return err
}

func (d *DB) RevokeDomainAccess(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM user_domain_access WHERE id = ?`, id)
	return err
}

func (d *DB) GetUserDomainAccess(userID int64) ([]DomainAccess, error) {
	rows, err := d.conn.Query(
		`SELECT id, user_id, site_id, domain_pattern, access_level, granted_by, granted_at
		 FROM user_domain_access WHERE user_id = ? ORDER BY granted_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accesses []DomainAccess
	for rows.Next() {
		var a DomainAccess
		var pattern sql.NullString
		if err := rows.Scan(&a.ID, &a.UserID, &a.SiteID, &pattern, &a.AccessLevel, &a.GrantedBy, &a.GrantedAt); err != nil {
			return nil, err
		}
		a.DomainPattern = pattern.String
		accesses = append(accesses, a)
	}
	return accesses, rows.Err()
}

func (d *DB) UserCanAccessSite(userID, siteID int64) (bool, string, error) {
	// Check for admin group membership first — admins can access everything
	hasAdmin, err := d.UserHasPermission(userID, "sites.delete")
	if err != nil {
		return false, "", err
	}
	if hasAdmin {
		return true, "admin", nil
	}

	// Check direct site_id match
	var level string
	err = d.conn.QueryRow(
		`SELECT access_level FROM user_domain_access WHERE user_id = ? AND site_id = ? LIMIT 1`,
		userID, siteID,
	).Scan(&level)
	if err == nil {
		return true, level, nil
	}
	if err != sql.ErrNoRows {
		return false, "", err
	}

	// Check domain pattern match — fetch the site domain and compare against patterns
	var domain string
	err = d.conn.QueryRow(`SELECT domain FROM sites WHERE id = ?`, siteID).Scan(&domain)
	if err != nil {
		return false, "", err
	}

	rows, err := d.conn.Query(
		`SELECT domain_pattern, access_level FROM user_domain_access
		 WHERE user_id = ? AND domain_pattern IS NOT NULL`, userID,
	)
	if err != nil {
		return false, "", err
	}
	defer rows.Close()

	for rows.Next() {
		var pattern, accessLvl string
		if err := rows.Scan(&pattern, &accessLvl); err != nil {
			return false, "", err
		}
		if matchDomainPattern(pattern, domain) {
			return true, accessLvl, nil
		}
	}

	return false, "", rows.Err()
}

// matchDomainPattern checks if domain matches pattern.
// Supports wildcards: "*.example.com" matches "sub.example.com"
// and exact matches: "example.com" matches "example.com".
func matchDomainPattern(pattern, domain string) bool {
	pattern = strings.ToLower(pattern)
	domain = strings.ToLower(domain)

	if pattern == domain {
		return true
	}

	// Wildcard match: *.example.com
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		if strings.HasSuffix(domain, suffix) {
			// Ensure there's exactly one level before the suffix
			prefix := domain[:len(domain)-len(suffix)]
			if len(prefix) > 0 && !strings.Contains(prefix, ".") {
				return true
			}
		}
	}

	return false
}

// ── Seeding ─────────────────────────────────────────────────────────

// defaultPermissions defines all permissions to seed into the database.
var defaultPermissions = []struct {
	Name        string
	DisplayName string
	Category    string
	Description string
}{
	{"sites.view", "View Sites", "sites", "View site listings and details"},
	{"sites.create", "Create Sites", "sites", "Create new sites"},
	{"sites.edit", "Edit Sites", "sites", "Edit existing site configuration"},
	{"sites.delete", "Delete Sites", "sites", "Delete sites permanently"},
	{"sites.deploy", "Deploy Sites", "sites", "Deploy and redeploy sites"},
	{"sites.start_stop", "Start/Stop Sites", "sites", "Start and stop site services"},

	{"ssl.view", "View SSL", "ssl", "View SSL certificate status"},
	{"ssl.manage", "Manage SSL", "ssl", "Issue, renew, and configure SSL certificates"},

	{"nginx.view", "View Nginx", "nginx", "View Nginx configuration"},
	{"nginx.manage", "Manage Nginx", "nginx", "Edit and reload Nginx configuration"},

	{"firewall.view", "View Firewall", "firewall", "View firewall rules"},
	{"firewall.manage", "Manage Firewall", "firewall", "Add, edit, and delete firewall rules"},

	{"users.view", "View Users", "users", "View user listings"},
	{"users.create", "Create Users", "users", "Create new user accounts"},
	{"users.edit", "Edit Users", "users", "Edit existing user accounts"},
	{"users.delete", "Delete Users", "users", "Delete user accounts"},
	{"users.manage_groups", "Manage Groups", "users", "Manage user groups and role assignments"},

	{"backups.view", "View Backups", "backups", "View backup listings"},
	{"backups.create", "Create Backups", "backups", "Create new backups"},
	{"backups.delete", "Delete Backups", "backups", "Delete backups"},
	{"backups.download", "Download Backups", "backups", "Download backup files"},

	{"monitor.view", "View Monitoring", "monitor", "View system and site monitoring"},

	{"logs.view", "View Logs", "logs", "View system and application logs"},

	{"hosting.view", "View Hosting", "hosting", "View hosting provider configuration"},
	{"hosting.configure", "Configure Hosting", "hosting", "Configure hosting providers"},
	{"hosting.manage_dns", "Manage DNS", "hosting", "Manage DNS records"},
	{"hosting.manage_domains", "Manage Domains", "hosting", "Register and manage domains"},
	{"hosting.manage_vps", "Manage VPS", "hosting", "Manage VPS instances"},

	{"git.view", "View Git", "git", "View Git repositories and status"},
	{"git.create", "Create Git Repos", "git", "Create and clone Git repositories"},
	{"git.delete", "Delete Git Repos", "git", "Delete Git repositories"},
	{"git.manage_users", "Manage Git Users", "git", "Manage Git user access"},

	{"system.settings", "System Settings", "system", "Modify system-level settings"},
	{"system.audit_log", "Audit Log", "system", "View the audit log"},

	{"float.view", "View Float", "float", "View Float sessions"},
	{"float.manage", "Manage Float", "float", "Manage Float sessions and configuration"},
}

// SeedPermissions creates all default permission entries if they don't already exist.
func (d *DB) SeedPermissions() error {
	for _, p := range defaultPermissions {
		_, err := d.conn.Exec(
			`INSERT OR IGNORE INTO permissions (name, display_name, category, description) VALUES (?, ?, ?, ?)`,
			p.Name, p.DisplayName, p.Category, p.Description,
		)
		if err != nil {
			return fmt.Errorf("seed permission %s: %w", p.Name, err)
		}
	}
	return nil
}

// defaultGroups defines the system groups and their assigned permission names.
var defaultGroups = []struct {
	Name        string
	DisplayName string
	Description string
	Permissions []string // empty means ALL permissions (admin)
}{
	{
		Name:        "admin",
		DisplayName: "Admin",
		Description: "Full access to everything",
		Permissions: nil, // nil = all permissions
	},
	{
		Name:        "support",
		DisplayName: "Support",
		Description: "Can view all sites, manage users, view logs, but cannot delete sites or change system config",
		Permissions: []string{
			"sites.view", "ssl.view", "nginx.view", "firewall.view",
			"users.view", "users.create", "users.edit",
			"backups.view", "monitor.view", "logs.view",
			"hosting.view", "git.view", "float.view",
			"system.audit_log",
		},
	},
	{
		Name:        "power_user",
		DisplayName: "Power User",
		Description: "Can manage assigned domains/subdomains, deploy, and view logs for their sites",
		Permissions: []string{
			"sites.view", "sites.edit", "sites.deploy", "sites.start_stop",
			"ssl.view", "ssl.manage",
			"backups.view", "backups.create",
			"monitor.view", "logs.view",
			"git.view", "git.create",
		},
	},
	{
		Name:        "subscriber",
		DisplayName: "Subscriber",
		Description: "Read-only access to assigned domains/subdomains, can view status and logs",
		Permissions: []string{
			"sites.view", "ssl.view", "monitor.view", "logs.view", "git.view",
		},
	},
}

// SeedDefaultGroups creates the default system groups with their permissions if they don't already exist.
func (d *DB) SeedDefaultGroups() error {
	// Get all permissions for admin group assignment
	allPerms, err := d.ListPermissions()
	if err != nil {
		return fmt.Errorf("list permissions for seeding: %w", err)
	}

	// Build a name->ID lookup
	permByName := make(map[string]int64, len(allPerms))
	for _, p := range allPerms {
		permByName[p.Name] = p.ID
	}

	for _, gDef := range defaultGroups {
		// Check if group already exists
		existing, err := d.GetGroupByName(gDef.Name)
		if err != nil {
			return fmt.Errorf("check group %s: %w", gDef.Name, err)
		}

		var groupID int64
		if existing != nil {
			groupID = existing.ID
		} else {
			groupID, err = d.CreateGroup(gDef.Name, gDef.DisplayName, gDef.Description, true)
			if err != nil {
				return fmt.Errorf("create group %s: %w", gDef.Name, err)
			}
		}

		// Determine which permissions to assign
		var permsToAssign []int64
		if gDef.Permissions == nil {
			// Admin: all permissions
			for _, p := range allPerms {
				permsToAssign = append(permsToAssign, p.ID)
			}
		} else {
			for _, pName := range gDef.Permissions {
				if pid, ok := permByName[pName]; ok {
					permsToAssign = append(permsToAssign, pid)
				}
			}
		}

		// Assign permissions (INSERT OR IGNORE handles duplicates)
		for _, pid := range permsToAssign {
			if err := d.AssignPermissionToGroup(groupID, pid); err != nil {
				return fmt.Errorf("assign perm %d to group %s: %w", pid, gDef.Name, err)
			}
		}
	}

	return nil
}

// SetUserGroups replaces all group memberships for a user with the given group IDs.
func (d *DB) SetUserGroups(userID int64, groupIDs []int64) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM user_group_membership WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, gid := range groupIDs {
		if _, err := tx.Exec(`INSERT INTO user_group_membership (user_id, group_id) VALUES (?, ?)`, userID, gid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetGroupPermissions replaces all permissions for a group with the given permission IDs.
func (d *DB) SetGroupPermissions(groupID int64, permissionIDs []int64) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM group_permissions WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	for _, pid := range permissionIDs {
		if _, err := tx.Exec(`INSERT INTO group_permissions (group_id, permission_id) VALUES (?, ?)`, groupID, pid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GroupMemberCount returns the number of members in a group.
func (d *DB) GroupMemberCount(groupID int64) (int, error) {
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM user_group_membership WHERE group_id = ?`, groupID).Scan(&count)
	return count, err
}

// GetUserGroupNames returns just the group name strings for a user (used for JWT claims).
func (d *DB) GetUserGroupNames(userID int64) ([]string, error) {
	rows, err := d.conn.Query(
		`SELECT g.name FROM user_groups g
		 JOIN user_group_membership ugm ON ugm.group_id = g.id
		 WHERE ugm.user_id = ?
		 ORDER BY g.name`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
