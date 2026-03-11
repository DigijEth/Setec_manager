package db

import "time"

type FloatSession struct {
	ID          string     `json:"id"`
	UserID      int64      `json:"user_id"`
	ClientIP    string     `json:"client_ip"`
	ClientAgent string     `json:"client_agent"`
	USBBridge   bool       `json:"usb_bridge"`
	ConnectedAt time.Time  `json:"connected_at"`
	LastPing    *time.Time `json:"last_ping"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

func (d *DB) CreateFloatSession(id string, userID int64, clientIP, agent string, expiresAt time.Time) error {
	_, err := d.conn.Exec(`INSERT INTO float_sessions (id, user_id, client_ip, client_agent, expires_at)
		VALUES (?, ?, ?, ?, ?)`, id, userID, clientIP, agent, expiresAt)
	return err
}

func (d *DB) GetFloatSession(id string) (*FloatSession, error) {
	var s FloatSession
	err := d.conn.QueryRow(`SELECT id, user_id, client_ip, client_agent, usb_bridge,
		connected_at, last_ping, expires_at FROM float_sessions WHERE id=?`, id).
		Scan(&s.ID, &s.UserID, &s.ClientIP, &s.ClientAgent, &s.USBBridge,
			&s.ConnectedAt, &s.LastPing, &s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (d *DB) ListFloatSessions() ([]FloatSession, error) {
	rows, err := d.conn.Query(`SELECT id, user_id, client_ip, client_agent, usb_bridge,
		connected_at, last_ping, expires_at FROM float_sessions ORDER BY connected_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []FloatSession
	for rows.Next() {
		var s FloatSession
		if err := rows.Scan(&s.ID, &s.UserID, &s.ClientIP, &s.ClientAgent, &s.USBBridge,
			&s.ConnectedAt, &s.LastPing, &s.ExpiresAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (d *DB) DeleteFloatSession(id string) error {
	_, err := d.conn.Exec(`DELETE FROM float_sessions WHERE id=?`, id)
	return err
}

func (d *DB) PingFloatSession(id string) error {
	_, err := d.conn.Exec(`UPDATE float_sessions SET last_ping=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func (d *DB) CleanExpiredFloatSessions() (int64, error) {
	result, err := d.conn.Exec(`DELETE FROM float_sessions WHERE expires_at < CURRENT_TIMESTAMP`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
