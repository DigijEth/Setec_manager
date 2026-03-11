package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(1) // SQLite single-writer

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) Conn() *sql.DB {
	return d.conn
}

func (d *DB) migrate() error {
	migrations := []string{
		migrateSites,
		migrateSystemUsers,
		migrateManagerUsers,
		migrateDeployments,
		migrateCronJobs,
		migrateFirewallRules,
		migrateFloatSessions,
		migrateBackups,
		migrateAuditLog,
	}

	for _, m := range migrations {
		if _, err := d.conn.Exec(m); err != nil {
			return err
		}
	}
	return nil
}

const migrateSites = `CREATE TABLE IF NOT EXISTS sites (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    domain        TEXT NOT NULL UNIQUE,
    aliases       TEXT DEFAULT '',
    app_type      TEXT NOT NULL DEFAULT 'static',
    app_root      TEXT NOT NULL,
    app_port      INTEGER DEFAULT 0,
    app_entry     TEXT DEFAULT '',
    git_repo      TEXT DEFAULT '',
    git_branch    TEXT DEFAULT 'main',
    ssl_enabled   BOOLEAN DEFAULT FALSE,
    ssl_cert_path TEXT DEFAULT '',
    ssl_key_path  TEXT DEFAULT '',
    ssl_auto      BOOLEAN DEFAULT TRUE,
    enabled       BOOLEAN DEFAULT TRUE,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const migrateSystemUsers = `CREATE TABLE IF NOT EXISTS system_users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    uid           INTEGER,
    home_dir      TEXT,
    shell         TEXT DEFAULT '/bin/bash',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const migrateManagerUsers = `CREATE TABLE IF NOT EXISTS manager_users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT DEFAULT 'admin',
    force_change  BOOLEAN DEFAULT FALSE,
    last_login    DATETIME,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const migrateDeployments = `CREATE TABLE IF NOT EXISTS deployments (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id       INTEGER REFERENCES sites(id),
    action        TEXT NOT NULL,
    status        TEXT DEFAULT 'pending',
    output        TEXT DEFAULT '',
    started_at    DATETIME,
    finished_at   DATETIME
);`

const migrateCronJobs = `CREATE TABLE IF NOT EXISTS cron_jobs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id       INTEGER REFERENCES sites(id),
    job_type      TEXT NOT NULL,
    schedule      TEXT NOT NULL,
    enabled       BOOLEAN DEFAULT TRUE,
    last_run      DATETIME,
    next_run      DATETIME
);`

const migrateFirewallRules = `CREATE TABLE IF NOT EXISTS firewall_rules (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    direction     TEXT DEFAULT 'in',
    protocol      TEXT DEFAULT 'tcp',
    port          TEXT NOT NULL,
    source        TEXT DEFAULT 'any',
    action        TEXT DEFAULT 'allow',
    comment       TEXT DEFAULT '',
    enabled       BOOLEAN DEFAULT TRUE
);`

const migrateFloatSessions = `CREATE TABLE IF NOT EXISTS float_sessions (
    id            TEXT PRIMARY KEY,
    user_id       INTEGER REFERENCES manager_users(id),
    client_ip     TEXT,
    client_agent  TEXT,
    usb_bridge    BOOLEAN DEFAULT FALSE,
    connected_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_ping     DATETIME,
    expires_at    DATETIME
);`

const migrateAuditLog = `CREATE TABLE IF NOT EXISTS audit_log (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL,
    ip            TEXT,
    action        TEXT NOT NULL,
    detail        TEXT DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const migrateBackups = `CREATE TABLE IF NOT EXISTS backups (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    site_id       INTEGER REFERENCES sites(id),
    backup_type   TEXT DEFAULT 'site',
    file_path     TEXT NOT NULL,
    size_bytes    INTEGER DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);`
