package db

import "time"

type Deployment struct {
	ID         int64      `json:"id"`
	SiteID     *int64     `json:"site_id"`
	Action     string     `json:"action"`
	Status     string     `json:"status"`
	Output     string     `json:"output"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

func (d *DB) CreateDeployment(siteID *int64, action string) (int64, error) {
	result, err := d.conn.Exec(`INSERT INTO deployments (site_id, action, status, started_at)
		VALUES (?, ?, 'running', CURRENT_TIMESTAMP)`, siteID, action)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) FinishDeployment(id int64, status, output string) error {
	_, err := d.conn.Exec(`UPDATE deployments SET status=?, output=?, finished_at=CURRENT_TIMESTAMP
		WHERE id=?`, status, output, id)
	return err
}

func (d *DB) ListDeployments(siteID *int64, limit int) ([]Deployment, error) {
	var rows_query string
	var args []interface{}

	if siteID != nil {
		rows_query = `SELECT id, site_id, action, status, output, started_at, finished_at
			FROM deployments WHERE site_id=? ORDER BY id DESC LIMIT ?`
		args = []interface{}{*siteID, limit}
	} else {
		rows_query = `SELECT id, site_id, action, status, output, started_at, finished_at
			FROM deployments ORDER BY id DESC LIMIT ?`
		args = []interface{}{limit}
	}

	rows, err := d.conn.Query(rows_query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []Deployment
	for rows.Next() {
		var dep Deployment
		if err := rows.Scan(&dep.ID, &dep.SiteID, &dep.Action, &dep.Status,
			&dep.Output, &dep.StartedAt, &dep.FinishedAt); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, rows.Err()
}
