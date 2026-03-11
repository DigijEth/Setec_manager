package db

import (
	"database/sql"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type ManagerUser struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	ForceChange  bool       `json:"force_change"`
	LastLogin    *time.Time `json:"last_login"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (d *DB) ListManagerUsers() ([]ManagerUser, error) {
	rows, err := d.conn.Query(`SELECT id, username, password_hash, role, force_change,
		last_login, created_at FROM manager_users ORDER BY username`)
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

func (d *DB) GetManagerUser(username string) (*ManagerUser, error) {
	var u ManagerUser
	err := d.conn.QueryRow(`SELECT id, username, password_hash, role, force_change,
		last_login, created_at FROM manager_users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role,
			&u.ForceChange, &u.LastLogin, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (d *DB) GetManagerUserByID(id int64) (*ManagerUser, error) {
	var u ManagerUser
	err := d.conn.QueryRow(`SELECT id, username, password_hash, role, force_change,
		last_login, created_at FROM manager_users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role,
			&u.ForceChange, &u.LastLogin, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (d *DB) CreateManagerUser(username, password, role string) (int64, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	result, err := d.conn.Exec(`INSERT INTO manager_users (username, password_hash, role)
		VALUES (?, ?, ?)`, username, string(hash), role)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) UpdateManagerUserPassword(id int64, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = d.conn.Exec(`UPDATE manager_users SET password_hash=?, force_change=FALSE WHERE id=?`,
		string(hash), id)
	return err
}

func (d *DB) UpdateManagerUserRole(id int64, role string) error {
	_, err := d.conn.Exec(`UPDATE manager_users SET role=? WHERE id=?`, role, id)
	return err
}

func (d *DB) DeleteManagerUser(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM manager_users WHERE id=?`, id)
	return err
}

func (d *DB) UpdateLoginTimestamp(id int64) error {
	_, err := d.conn.Exec(`UPDATE manager_users SET last_login=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func (d *DB) ManagerUserCount() (int, error) {
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM manager_users`).Scan(&count)
	return count, err
}

func (d *DB) AuthenticateUser(username, password string) (*ManagerUser, error) {
	u, err := d.GetManagerUser(username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, nil
	}

	d.UpdateLoginTimestamp(u.ID)
	return u, nil
}
