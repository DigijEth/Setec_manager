package float

import (
	"log"
	"net/http"
	"sync"
	"time"

	"setec-manager/internal/db"

	"github.com/gorilla/websocket"
)

// Bridge manages WebSocket connections for USB passthrough in Float Mode.
type Bridge struct {
	db       *db.DB
	sessions map[string]*bridgeConn
	mu       sync.RWMutex
	upgrader websocket.Upgrader
}

// bridgeConn tracks a single active WebSocket connection and its associated session.
type bridgeConn struct {
	sessionID string
	conn      *websocket.Conn
	devices   []USBDevice
	mu        sync.Mutex
	done      chan struct{}
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingInterval   = 30 * time.Second
	maxMessageSize = 64 * 1024 // 64 KB max frame payload
)

// NewBridge creates a new Bridge with the given database reference.
func NewBridge(database *db.DB) *Bridge {
	return &Bridge{
		db:       database,
		sessions: make(map[string]*bridgeConn),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true // Accept all origins; auth is handled via session token
			},
		},
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket and manages the
// binary frame protocol for USB passthrough. The session ID must be provided
// as a "session" query parameter.
func (b *Bridge) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "missing session parameter", http.StatusBadRequest)
		return
	}

	// Validate session exists and is not expired
	sess, err := b.db.GetFloatSession(sessionID)
	if err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	if time.Now().After(sess.ExpiresAt) {
		http.Error(w, "session expired", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[float/bridge] upgrade failed for session %s: %v", sessionID, err)
		return
	}

	bc := &bridgeConn{
		sessionID: sessionID,
		conn:      conn,
		done:      make(chan struct{}),
	}

	// Register active connection
	b.mu.Lock()
	// Close any existing connection for this session
	if existing, ok := b.sessions[sessionID]; ok {
		close(existing.done)
		existing.conn.Close()
	}
	b.sessions[sessionID] = bc
	b.mu.Unlock()

	log.Printf("[float/bridge] session %s connected from %s", sessionID, r.RemoteAddr)

	// Start read/write loops
	go b.writePump(bc)
	b.readPump(bc)
}

// readPump reads binary frames from the WebSocket and dispatches them.
func (b *Bridge) readPump(bc *bridgeConn) {
	defer b.cleanup(bc)

	bc.conn.SetReadLimit(maxMessageSize)
	bc.conn.SetReadDeadline(time.Now().Add(pongWait))
	bc.conn.SetPongHandler(func(string) error {
		bc.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		messageType, data, err := bc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[float/bridge] session %s read error: %v", bc.sessionID, err)
			}
			return
		}

		if messageType != websocket.BinaryMessage {
			b.sendError(bc, 0x0001, "expected binary message")
			continue
		}

		frameType, payload, err := DecodeFrame(data)
		if err != nil {
			b.sendError(bc, 0x0002, "malformed frame: "+err.Error())
			continue
		}

		// Update session ping in DB
		b.db.PingFloatSession(bc.sessionID)

		switch frameType {
		case FrameEnumerate:
			b.handleEnumerate(bc)
		case FrameOpen:
			b.handleOpen(bc, payload)
		case FrameClose:
			b.handleClose(bc, payload)
		case FrameTransferOut:
			b.handleTransfer(bc, payload)
		case FrameInterrupt:
			b.handleInterrupt(bc, payload)
		case FramePong:
			// Client responded to our ping; no action needed
		default:
			b.sendError(bc, 0x0003, "unknown frame type")
		}
	}
}

// writePump sends periodic pings to keep the connection alive.
func (b *Bridge) writePump(bc *bridgeConn) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bc.mu.Lock()
			bc.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := bc.conn.WriteMessage(websocket.BinaryMessage, EncodeFrame(FramePing, nil))
			bc.mu.Unlock()
			if err != nil {
				return
			}
		case <-bc.done:
			return
		}
	}
}

// handleEnumerate responds with the current list of USB devices known to this
// session. In a full implementation, this would forward the enumerate request
// to the client-side USB agent and await its response. Here we return the
// cached device list.
func (b *Bridge) handleEnumerate(bc *bridgeConn) {
	bc.mu.Lock()
	devices := bc.devices
	bc.mu.Unlock()

	if devices == nil {
		devices = []USBDevice{}
	}

	payload := EncodeDeviceList(devices)
	b.sendFrame(bc, FrameEnumResult, payload)
}

// handleOpen processes a device open request. The payload contains
// [deviceID:2] identifying which device to claim.
func (b *Bridge) handleOpen(bc *bridgeConn, payload []byte) {
	if len(payload) < 2 {
		b.sendError(bc, 0x0010, "open: payload too short")
		return
	}

	deviceID := uint16(payload[0])<<8 | uint16(payload[1])

	// Verify the device exists in our known list
	bc.mu.Lock()
	found := false
	for _, dev := range bc.devices {
		if dev.DeviceID == deviceID {
			found = true
			break
		}
	}
	bc.mu.Unlock()

	if !found {
		b.sendError(bc, 0x0011, "open: device not found")
		return
	}

	// In a real implementation, this would claim the USB device via the host agent.
	// For now, acknowledge the open request.
	result := make([]byte, 3)
	result[0] = payload[0]
	result[1] = payload[1]
	result[2] = 0x00 // success
	b.sendFrame(bc, FrameOpenResult, result)

	log.Printf("[float/bridge] session %s opened device 0x%04X", bc.sessionID, deviceID)
}

// handleClose processes a device close request. Payload: [deviceID:2].
func (b *Bridge) handleClose(bc *bridgeConn, payload []byte) {
	if len(payload) < 2 {
		b.sendError(bc, 0x0020, "close: payload too short")
		return
	}

	deviceID := uint16(payload[0])<<8 | uint16(payload[1])

	// Acknowledge close
	result := make([]byte, 3)
	result[0] = payload[0]
	result[1] = payload[1]
	result[2] = 0x00 // success
	b.sendFrame(bc, FrameCloseResult, result)

	log.Printf("[float/bridge] session %s closed device 0x%04X", bc.sessionID, deviceID)
}

// handleTransfer forwards a bulk/interrupt OUT transfer to the USB device.
func (b *Bridge) handleTransfer(bc *bridgeConn, payload []byte) {
	deviceID, endpoint, transferData, err := DecodeTransfer(payload)
	if err != nil {
		b.sendError(bc, 0x0030, "transfer: "+err.Error())
		return
	}

	// In a real implementation, the transfer data would be sent to the USB device
	// via the host agent, and the response would be sent back. Here we acknowledge
	// receipt of the transfer request.
	log.Printf("[float/bridge] session %s transfer to device 0x%04X endpoint 0x%02X: %d bytes",
		bc.sessionID, deviceID, endpoint, len(transferData))

	// Build transfer result: [deviceID:2][endpoint:1][status:1]
	result := make([]byte, 4)
	result[0] = byte(deviceID >> 8)
	result[1] = byte(deviceID)
	result[2] = endpoint
	result[3] = 0x00 // success
	b.sendFrame(bc, FrameTransferResult, result)
}

// handleInterrupt processes an interrupt transfer request.
func (b *Bridge) handleInterrupt(bc *bridgeConn, payload []byte) {
	if len(payload) < 3 {
		b.sendError(bc, 0x0040, "interrupt: payload too short")
		return
	}

	deviceID := uint16(payload[0])<<8 | uint16(payload[1])
	endpoint := payload[2]

	log.Printf("[float/bridge] session %s interrupt on device 0x%04X endpoint 0x%02X",
		bc.sessionID, deviceID, endpoint)

	// Acknowledge interrupt request
	result := make([]byte, 4)
	result[0] = payload[0]
	result[1] = payload[1]
	result[2] = endpoint
	result[3] = 0x00 // success
	b.sendFrame(bc, FrameInterruptResult, result)
}

// sendFrame writes a binary frame to the WebSocket connection.
func (b *Bridge) sendFrame(bc *bridgeConn, frameType byte, payload []byte) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := bc.conn.WriteMessage(websocket.BinaryMessage, EncodeFrame(frameType, payload)); err != nil {
		log.Printf("[float/bridge] session %s write error: %v", bc.sessionID, err)
	}
}

// sendError writes an error frame to the WebSocket connection.
func (b *Bridge) sendError(bc *bridgeConn, code uint16, message string) {
	b.sendFrame(bc, FrameError, EncodeError(code, message))
}

// cleanup removes a connection from the active sessions and cleans up resources.
func (b *Bridge) cleanup(bc *bridgeConn) {
	b.mu.Lock()
	if current, ok := b.sessions[bc.sessionID]; ok && current == bc {
		delete(b.sessions, bc.sessionID)
	}
	b.mu.Unlock()

	close(bc.done)
	bc.conn.Close()

	log.Printf("[float/bridge] session %s disconnected", bc.sessionID)
}

// ActiveSessions returns the number of currently connected WebSocket sessions.
func (b *Bridge) ActiveSessions() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.sessions)
}

// DisconnectSession forcibly closes the WebSocket connection for a given session.
func (b *Bridge) DisconnectSession(sessionID string) {
	b.mu.Lock()
	bc, ok := b.sessions[sessionID]
	if ok {
		delete(b.sessions, sessionID)
	}
	b.mu.Unlock()

	if ok {
		close(bc.done)
		bc.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session terminated"),
			time.Now().Add(writeWait),
		)
		bc.conn.Close()
		log.Printf("[float/bridge] session %s forcibly disconnected", sessionID)
	}
}

// UpdateDeviceList sets the known device list for a session (called when the
// client-side USB agent reports its attached devices).
func (b *Bridge) UpdateDeviceList(sessionID string, devices []USBDevice) {
	b.mu.RLock()
	bc, ok := b.sessions[sessionID]
	b.mu.RUnlock()

	if ok {
		bc.mu.Lock()
		bc.devices = devices
		bc.mu.Unlock()
	}
}
