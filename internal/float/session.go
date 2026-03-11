package float

import (
	"fmt"
	"log"
	"sync"
	"time"

	"setec-manager/internal/db"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Session represents an active Float Mode session, combining database state
// with the live WebSocket connection reference.
type Session struct {
	ID          string         `json:"id"`
	UserID      int64          `json:"user_id"`
	ClientIP    string         `json:"client_ip"`
	ClientAgent string         `json:"client_agent"`
	USBBridge   bool           `json:"usb_bridge"`
	ConnectedAt time.Time      `json:"connected_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
	LastPing    *time.Time     `json:"last_ping,omitempty"`
	conn        *websocket.Conn
}

// SessionManager provides in-memory + database-backed session lifecycle
// management for Float Mode connections.
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	db       *db.DB
}

// NewSessionManager creates a new SessionManager backed by the given database.
func NewSessionManager(database *db.DB) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		db:       database,
	}
}

// Create generates a new Float session with a random UUID, storing it in both
// the in-memory map and the database.
func (sm *SessionManager) Create(userID int64, clientIP, agent string, ttl time.Duration) (string, error) {
	id := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(ttl)

	session := &Session{
		ID:          id,
		UserID:      userID,
		ClientIP:    clientIP,
		ClientAgent: agent,
		ConnectedAt: now,
		ExpiresAt:   expiresAt,
	}

	// Persist to database first
	if err := sm.db.CreateFloatSession(id, userID, clientIP, agent, expiresAt); err != nil {
		return "", fmt.Errorf("create session: db insert: %w", err)
	}

	// Store in memory
	sm.mu.Lock()
	sm.sessions[id] = session
	sm.mu.Unlock()

	log.Printf("[float/session] created session %s for user %d from %s (expires %s)",
		id, userID, clientIP, expiresAt.Format(time.RFC3339))

	return id, nil
}

// Get retrieves a session by ID, checking the in-memory cache first, then
// falling back to the database. Returns nil and an error if not found.
func (sm *SessionManager) Get(id string) (*Session, error) {
	// Check memory first
	sm.mu.RLock()
	if sess, ok := sm.sessions[id]; ok {
		sm.mu.RUnlock()
		// Check if expired
		if time.Now().After(sess.ExpiresAt) {
			sm.Delete(id)
			return nil, fmt.Errorf("session %s has expired", id)
		}
		return sess, nil
	}
	sm.mu.RUnlock()

	// Fall back to database
	dbSess, err := sm.db.GetFloatSession(id)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Check if expired
	if time.Now().After(dbSess.ExpiresAt) {
		sm.db.DeleteFloatSession(id)
		return nil, fmt.Errorf("session %s has expired", id)
	}

	// Hydrate into memory
	session := &Session{
		ID:          dbSess.ID,
		UserID:      dbSess.UserID,
		ClientIP:    dbSess.ClientIP,
		ClientAgent: dbSess.ClientAgent,
		USBBridge:   dbSess.USBBridge,
		ConnectedAt: dbSess.ConnectedAt,
		ExpiresAt:   dbSess.ExpiresAt,
		LastPing:    dbSess.LastPing,
	}

	sm.mu.Lock()
	sm.sessions[id] = session
	sm.mu.Unlock()

	return session, nil
}

// Delete removes a session from both the in-memory map and the database.
func (sm *SessionManager) Delete(id string) error {
	sm.mu.Lock()
	sess, ok := sm.sessions[id]
	if ok {
		// Close the WebSocket connection if it exists
		if sess.conn != nil {
			sess.conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session deleted"),
				time.Now().Add(5*time.Second),
			)
			sess.conn.Close()
		}
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()

	if err := sm.db.DeleteFloatSession(id); err != nil {
		return fmt.Errorf("delete session: db delete: %w", err)
	}

	log.Printf("[float/session] deleted session %s", id)
	return nil
}

// Ping updates the last-ping timestamp for a session in both memory and DB.
func (sm *SessionManager) Ping(id string) error {
	now := time.Now()

	sm.mu.Lock()
	if sess, ok := sm.sessions[id]; ok {
		sess.LastPing = &now
	}
	sm.mu.Unlock()

	if err := sm.db.PingFloatSession(id); err != nil {
		return fmt.Errorf("ping session: %w", err)
	}
	return nil
}

// CleanExpired removes all sessions that have passed their expiry time.
// Returns the number of sessions removed.
func (sm *SessionManager) CleanExpired() (int, error) {
	now := time.Now()

	// Clean from memory
	sm.mu.Lock()
	var expiredIDs []string
	for id, sess := range sm.sessions {
		if now.After(sess.ExpiresAt) {
			expiredIDs = append(expiredIDs, id)
			if sess.conn != nil {
				sess.conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session expired"),
					now.Add(5*time.Second),
				)
				sess.conn.Close()
			}
		}
	}
	for _, id := range expiredIDs {
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()

	// Clean from database
	count, err := sm.db.CleanExpiredFloatSessions()
	if err != nil {
		return len(expiredIDs), fmt.Errorf("clean expired: db: %w", err)
	}

	total := int(count)
	if total > 0 {
		log.Printf("[float/session] cleaned %d expired sessions", total)
	}

	return total, nil
}

// ActiveCount returns the number of sessions currently in the in-memory map.
func (sm *SessionManager) ActiveCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// SetConn associates a WebSocket connection with a session.
func (sm *SessionManager) SetConn(id string, conn *websocket.Conn) {
	sm.mu.Lock()
	if sess, ok := sm.sessions[id]; ok {
		sess.conn = conn
		sess.USBBridge = true
	}
	sm.mu.Unlock()
}

// List returns all active (non-expired) sessions from the database.
func (sm *SessionManager) List() ([]Session, error) {
	dbSessions, err := sm.db.ListFloatSessions()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	sessions := make([]Session, 0, len(dbSessions))
	for _, dbs := range dbSessions {
		if time.Now().After(dbs.ExpiresAt) {
			continue
		}
		sessions = append(sessions, Session{
			ID:          dbs.ID,
			UserID:      dbs.UserID,
			ClientIP:    dbs.ClientIP,
			ClientAgent: dbs.ClientAgent,
			USBBridge:   dbs.USBBridge,
			ConnectedAt: dbs.ConnectedAt,
			ExpiresAt:   dbs.ExpiresAt,
			LastPing:    dbs.LastPing,
		})
	}

	return sessions, nil
}
