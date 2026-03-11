package db

import "time"

type Backup struct {
	ID         int64     `json:"id"`
	SiteID     *int64    `json:"site_id"`
	BackupType string    `json:"backup_type"`
	FilePath   string    `json:"file_path"`
	SizeBytes  int64     `json:"size_bytes"`
	CreatedAt  time.Time `json:"created_at"`
}

func (d *DB) CreateBackup(siteID *int64, backupType, filePath string, sizeBytes int64) (int64, error) {
	result, err := d.conn.Exec(`INSERT INTO backups (site_id, backup_type, file_path, size_bytes)
		VALUES (?, ?, ?, ?)`, siteID, backupType, filePath, sizeBytes)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) ListBackups() ([]Backup, error) {
	rows, err := d.conn.Query(`SELECT id, site_id, backup_type, file_path, size_bytes, created_at
		FROM backups ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.SiteID, &b.BackupType, &b.FilePath,
			&b.SizeBytes, &b.CreatedAt); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

func (d *DB) DeleteBackup(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM backups WHERE id=?`, id)
	return err
}
