package db

import (
	"database/sql"
	"time"
)

type Site struct {
	ID          int64     `json:"id"`
	Domain      string    `json:"domain"`
	Aliases     string    `json:"aliases"`
	AppType     string    `json:"app_type"`
	AppRoot     string    `json:"app_root"`
	AppPort     int       `json:"app_port"`
	AppEntry    string    `json:"app_entry"`
	GitRepo     string    `json:"git_repo"`
	GitBranch   string    `json:"git_branch"`
	SSLEnabled  bool      `json:"ssl_enabled"`
	SSLCertPath string    `json:"ssl_cert_path"`
	SSLKeyPath  string    `json:"ssl_key_path"`
	SSLAuto     bool      `json:"ssl_auto"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (d *DB) ListSites() ([]Site, error) {
	rows, err := d.conn.Query(`SELECT id, domain, aliases, app_type, app_root, app_port,
		app_entry, git_repo, git_branch, ssl_enabled, ssl_cert_path, ssl_key_path,
		ssl_auto, enabled, created_at, updated_at FROM sites ORDER BY domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var s Site
		if err := rows.Scan(&s.ID, &s.Domain, &s.Aliases, &s.AppType, &s.AppRoot,
			&s.AppPort, &s.AppEntry, &s.GitRepo, &s.GitBranch, &s.SSLEnabled,
			&s.SSLCertPath, &s.SSLKeyPath, &s.SSLAuto, &s.Enabled,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

func (d *DB) GetSite(id int64) (*Site, error) {
	var s Site
	err := d.conn.QueryRow(`SELECT id, domain, aliases, app_type, app_root, app_port,
		app_entry, git_repo, git_branch, ssl_enabled, ssl_cert_path, ssl_key_path,
		ssl_auto, enabled, created_at, updated_at FROM sites WHERE id = ?`, id).
		Scan(&s.ID, &s.Domain, &s.Aliases, &s.AppType, &s.AppRoot,
			&s.AppPort, &s.AppEntry, &s.GitRepo, &s.GitBranch, &s.SSLEnabled,
			&s.SSLCertPath, &s.SSLKeyPath, &s.SSLAuto, &s.Enabled,
			&s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (d *DB) GetSiteByDomain(domain string) (*Site, error) {
	var s Site
	err := d.conn.QueryRow(`SELECT id, domain, aliases, app_type, app_root, app_port,
		app_entry, git_repo, git_branch, ssl_enabled, ssl_cert_path, ssl_key_path,
		ssl_auto, enabled, created_at, updated_at FROM sites WHERE domain = ?`, domain).
		Scan(&s.ID, &s.Domain, &s.Aliases, &s.AppType, &s.AppRoot,
			&s.AppPort, &s.AppEntry, &s.GitRepo, &s.GitBranch, &s.SSLEnabled,
			&s.SSLCertPath, &s.SSLKeyPath, &s.SSLAuto, &s.Enabled,
			&s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (d *DB) CreateSite(s *Site) (int64, error) {
	result, err := d.conn.Exec(`INSERT INTO sites (domain, aliases, app_type, app_root, app_port,
		app_entry, git_repo, git_branch, ssl_enabled, ssl_cert_path, ssl_key_path, ssl_auto, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Domain, s.Aliases, s.AppType, s.AppRoot, s.AppPort,
		s.AppEntry, s.GitRepo, s.GitBranch, s.SSLEnabled,
		s.SSLCertPath, s.SSLKeyPath, s.SSLAuto, s.Enabled)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) UpdateSite(s *Site) error {
	_, err := d.conn.Exec(`UPDATE sites SET domain=?, aliases=?, app_type=?, app_root=?,
		app_port=?, app_entry=?, git_repo=?, git_branch=?, ssl_enabled=?,
		ssl_cert_path=?, ssl_key_path=?, ssl_auto=?, enabled=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		s.Domain, s.Aliases, s.AppType, s.AppRoot, s.AppPort,
		s.AppEntry, s.GitRepo, s.GitBranch, s.SSLEnabled,
		s.SSLCertPath, s.SSLKeyPath, s.SSLAuto, s.Enabled, s.ID)
	return err
}

func (d *DB) DeleteSite(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM sites WHERE id=?`, id)
	return err
}
