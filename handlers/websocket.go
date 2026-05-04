package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"server/db"
	"server/helpers"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// ─── Upgrader ──────────────────────────────────────────────────────────────────

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow connections from frontend origins
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		allowed := map[string]bool{
			"http://localhost:3000":              true,
			"http://localhost:5174":              true, // Guard PWA dev port
			"https://rakshak-service.vercel.app": true,
		}
		return allowed[origin]
	},
}

// ─── Message types ─────────────────────────────────────────────────────────────

type IncomingMsg struct {
	Type      string  `json:"type"` // "location" | "clockin" | "clockout" | "sos"
	GuardID   int     `json:"guardId"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Timestamp string  `json:"timestamp"`
}

type OutgoingMsg struct {
	Type      string  `json:"type"` // "guard_update" | "guard_disconnect" | "guard_list"
	GuardID   int     `json:"guardId"`
	GuardName string  `json:"guardName"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	ClockedIn bool    `json:"clockedIn"`
	Severity  string  `json:"severity,omitempty"` // for SOS
	Timestamp string  `json:"timestamp"`
	// InGeofence is nil when there is no active shift with a geofence defined.
	// true = inside the geofence, false = outside (amber alert on map).
	InGeofence *bool `json:"inGeofence,omitempty"`
}

// ─── Client structs ────────────────────────────────────────────────────────────

type GuardClient struct {
	conn      *websocket.Conn
	guardID   int
	guardName string
	send      chan []byte
}

type AdminClient struct {
	conn *websocket.Conn
	send chan []byte
}

// ─── Hub ───────────────────────────────────────────────────────────────────────

type Hub struct {
	mu           sync.RWMutex
	guardClients map[int]*GuardClient // guardID → client
	adminClients map[*AdminClient]bool
	guardStates  map[int]OutgoingMsg // last known state per guard
}

var globalHub = &Hub{
	guardClients: make(map[int]*GuardClient),
	adminClients: make(map[*AdminClient]bool),
	guardStates:  make(map[int]OutgoingMsg),
}

// broadcast sends a JSON message to all connected admin clients.
func (h *Hub) broadcastToAdmins(msg OutgoingMsg) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.adminClients {
		select {
		case client.send <- data:
		default:
			// Client send buffer full — skip; reader will clean up
		}
	}
}

// ─── Guard WebSocket Handler ───────────────────────────────────────────────────

// ServeGuardWS handles guard app WebSocket connections.
// URL: GET /ws/guard?token={guard_token}
func ServeGuardWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	// Validate token and fetch guard info
	var guardID int
	var guardName string
	err := db.DB.QueryRow(
		`SELECT id, name FROM guards WHERE guard_token = $1 AND deleted_at IS NULL`,
		token,
	).Scan(&guardID, &guardName)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] guard upgrade error: %v", err)
		return
	}

	client := &GuardClient{
		conn:      conn,
		guardID:   guardID,
		guardName: guardName,
		send:      make(chan []byte, 64),
	}

	globalHub.mu.Lock()
	globalHub.guardClients[guardID] = client
	globalHub.mu.Unlock()

	log.Printf("[ws] guard %d (%s) connected", guardID, guardName)

	go client.writePump()
	client.readPump()

	// Cleanup on disconnect
	globalHub.mu.Lock()
	delete(globalHub.guardClients, guardID)
	globalHub.mu.Unlock()

	globalHub.broadcastToAdmins(OutgoingMsg{
		Type:      "guard_disconnect",
		GuardID:   guardID,
		GuardName: guardName,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	log.Printf("[ws] guard %d (%s) disconnected", guardID, guardName)
}

func (c *GuardClient) readPump() {
	defer c.conn.Close()
	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))

		var msg IncomingMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		msg.GuardID = c.guardID

		switch msg.Type {
		case "location":
			c.handleLocation(msg)
		case "clockin":
			c.handleClockIn(msg)
		case "clockout":
			c.handleClockOut(msg)
		case "sos":
			c.handleSOS(msg)
		}
	}
}

func (c *GuardClient) writePump() {
	ticker := time.NewTicker(45 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *GuardClient) handleLocation(msg IncomingMsg) {
	// Persist location to DB
	_, _ = db.DB.Exec(
		`INSERT INTO guard_locations(guard_id, lat, lng) VALUES($1,$2,$3)`,
		c.guardID, msg.Lat, msg.Lng,
	)

	update := OutgoingMsg{
		Type:      "guard_update",
		GuardID:   c.guardID,
		GuardName: c.guardName,
		Lat:       msg.Lat,
		Lng:       msg.Lng,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	globalHub.mu.Lock()
	if prev, ok := globalHub.guardStates[c.guardID]; ok {
		update.ClockedIn = prev.ClockedIn
		update.Severity = prev.Severity // preserve SOS / other severity
	}
	globalHub.mu.Unlock()

	// ── Geofence check (Phase 9.5) ──────────────────────────────────────────
	// Find the guard's current active shift and its query geofence.
	var fenceLat, fenceLng float64
	var fenceRadiusM int
	err := db.DB.QueryRow(`
		SELECT q.geofence_lat, q.geofence_lng, q.geofence_radius_m
		FROM   shifts s
		JOIN   queries q ON q.id = s.query_id
		WHERE  s.guard_id      = $1
		  AND  s.start_time   <= NOW()
		  AND  s.end_time     >= NOW()
		  AND  s.deleted_at   IS NULL
		  AND  q.deleted_at   IS NULL
		  AND  q.geofence_lat IS NOT NULL
		LIMIT 1`, c.guardID,
	).Scan(&fenceLat, &fenceLng, &fenceRadiusM)

	if err == nil && fenceRadiusM > 0 {
		// Guard has an active shift with a geofence — compute distance.
		distM := helpers.HaversineM(msg.Lat, msg.Lng, fenceLat, fenceLng)
		inside := distM <= float64(fenceRadiusM)
		update.InGeofence = &inside
	}
	// err == sql.ErrNoRows → no active geofenced shift, leave InGeofence nil.

	globalHub.mu.Lock()
	globalHub.guardStates[c.guardID] = update
	globalHub.mu.Unlock()

	globalHub.broadcastToAdmins(update)
}

func (c *GuardClient) handleClockIn(msg IncomingMsg) {
	now := time.Now().UTC()
	_, _ = db.DB.Exec(
		`UPDATE guards SET clocked_in=TRUE, clocked_in_at=$1 WHERE id=$2`,
		now, c.guardID,
	)
	update := OutgoingMsg{
		Type:      "guard_update",
		GuardID:   c.guardID,
		GuardName: c.guardName,
		Lat:       msg.Lat,
		Lng:       msg.Lng,
		ClockedIn: true,
		Timestamp: now.Format(time.RFC3339),
	}
	globalHub.mu.Lock()
	// Preserve active SOS severity even after clock-in
	if prev, ok := globalHub.guardStates[c.guardID]; ok {
		update.Severity = prev.Severity
	}
	globalHub.guardStates[c.guardID] = update
	globalHub.mu.Unlock()
	globalHub.broadcastToAdmins(update)
}

func (c *GuardClient) handleClockOut(msg IncomingMsg) {
	now := time.Now().UTC()
	_, _ = db.DB.Exec(
		`UPDATE guards SET clocked_in=FALSE, clocked_in_at=NULL WHERE id=$1`,
		c.guardID,
	)
	update := OutgoingMsg{
		Type:      "guard_update",
		GuardID:   c.guardID,
		GuardName: c.guardName,
		Lat:       msg.Lat,
		Lng:       msg.Lng,
		ClockedIn: false,
		Timestamp: now.Format(time.RFC3339),
	}
	globalHub.mu.Lock()
	globalHub.guardStates[c.guardID] = update
	globalHub.mu.Unlock()
	globalHub.broadcastToAdmins(update)
}

func (c *GuardClient) handleSOS(msg IncomingMsg) {
	// Insert SOS incident
	_, _ = db.DB.Exec(
		`INSERT INTO incidents(guard_id, title, severity, lat, lng)
		 VALUES($1, $2, 'sos', $3, $4)`,
		c.guardID, "SOS Alert", msg.Lat, msg.Lng,
	)

	now := time.Now().UTC()

	// Phase-2: Create SOS notification for admins/managers
	go func() {
		notifMsg := fmt.Sprintf("🆘 SOS Alert from %s! Immediate attention required.", c.guardName)
		helpers.NotifyUsersByRole(db.DB, []string{"superadmin", "manager"}, notifMsg, "sos")
	}()

	// Preserve previous ClockedIn state and fall back to last known location
	// if the guard has no GPS fix at time of SOS.
	globalHub.mu.Lock()
	prev := globalHub.guardStates[c.guardID]
	lat, lng := msg.Lat, msg.Lng
	if lat == 0 && lng == 0 {
		lat, lng = prev.Lat, prev.Lng
	}
	update := OutgoingMsg{
		Type:      "guard_update",
		GuardID:   c.guardID,
		GuardName: c.guardName,
		Lat:       lat,
		Lng:       lng,
		ClockedIn: prev.ClockedIn,
		Severity:  "sos",
		Timestamp: now.Format(time.RFC3339),
	}
	// Persist SOS state in hub so reconnecting admins see it
	globalHub.guardStates[c.guardID] = update
	globalHub.mu.Unlock()

	globalHub.broadcastToAdmins(update)
}

// ─── Admin WebSocket Handler ───────────────────────────────────────────────────

// ServeAdminWS handles admin/manager WebSocket connections.
// URL: GET /ws/admin  (requires JWT cookie with superadmin/manager role)
func ServeAdminWS(w http.ResponseWriter, r *http.Request) {
	claims, err := parseTokenClaimsFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	role, _ := claims["role"].(string)
	if role != "superadmin" && role != "manager" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] admin upgrade error: %v", err)
		return
	}

	client := &AdminClient{
		conn: conn,
		send: make(chan []byte, 128),
	}

	globalHub.mu.Lock()
	globalHub.adminClients[client] = true
	// Send current guard state snapshot on connect
	snapshot := make([]OutgoingMsg, 0, len(globalHub.guardStates))
	for _, state := range globalHub.guardStates {
		snapshot = append(snapshot, state)
	}
	globalHub.mu.Unlock()

	// Send the initial snapshot
	if len(snapshot) > 0 {
		data, _ := json.Marshal(map[string]any{
			"type":   "guard_list",
			"guards": snapshot,
		})
		client.send <- data
	}

	log.Printf("[ws] admin connected (role=%s)", role)

	go client.writePump()
	client.readPump() // blocks until disconnect

	globalHub.mu.Lock()
	delete(globalHub.adminClients, client)
	globalHub.mu.Unlock()
	log.Printf("[ws] admin disconnected")
}

func (c *AdminClient) readPump() {
	defer c.conn.Close()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		return nil
	})
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	}
}

func (c *AdminClient) writePump() {
	ticker := time.NewTicker(45 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ─── SOS Acknowledge ──────────────────────────────────────────────────────────

// AcknowledgeSOS clears the active SOS severity for a guard in the hub and
// broadcasts the updated state to all connected admin clients.
// POST /api/guards/{id}/sos/clear
func AcknowledgeSOS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	guardID, err := strconv.Atoi(vars["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid guard id"})
		return
	}

	var update OutgoingMsg
	globalHub.mu.Lock()
	state, ok := globalHub.guardStates[guardID]
	if ok {
		state.Severity = ""
		state.Timestamp = time.Now().UTC().Format(time.RFC3339)
		globalHub.guardStates[guardID] = state
		update = state
	}
	globalHub.mu.Unlock()

	if ok {
		globalHub.broadcastToAdmins(update)
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

// ─── Guard Token REST Endpoints ────────────────────────────────────────────────

// GetGuardToken returns the guard's WebSocket token (admin only).
// GET /api/guards/{id}/token
func GetGuardToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]

	var token string
	var name string
	err := db.DB.QueryRow(
		`SELECT guard_token, name FROM guards WHERE id=$1 AND deleted_at IS NULL`,
		id,
	).Scan(&token, &name)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "guard not found"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{
		"guardId":    id,
		"guardName":  name,
		"guardToken": token,
	})
}

// GetConnectedGuards returns live status of all WebSocket-connected guards.
// GET /api/guards/live
func GetConnectedGuards(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	globalHub.mu.RLock()
	defer globalHub.mu.RUnlock()

	guards := make([]OutgoingMsg, 0, len(globalHub.guardStates))
	for _, state := range globalHub.guardStates {
		guards = append(guards, state)
	}
	_ = json.NewEncoder(w).Encode(guards)
}

// GetIncidents returns incident reports, optionally filtered by guard.
// GET /api/incidents?guard_id=42
func GetIncidents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	guardID := r.URL.Query().Get("guard_id")

	type Incident struct {
		ID          int      `json:"id"`
		GuardID     *int     `json:"guardId"`
		Title       string   `json:"title"`
		Description *string  `json:"description"`
		PhotoURL    *string  `json:"photoUrl"`
		Severity    string   `json:"severity"`
		Lat         *float64 `json:"lat"`
		Lng         *float64 `json:"lng"`
		CreatedAt   string   `json:"createdAt"`
	}

	incidents := make([]Incident, 0)

	const baseQ = `SELECT id, guard_id, title, description, photo_url, severity, lat, lng, created_at
	               FROM incidents WHERE deleted_at IS NULL`

	var (
		rows *sql.Rows
		err  error
	)
	if guardID != "" {
		rows, err = db.DB.Query(baseQ+` AND guard_id=$1 ORDER BY created_at DESC LIMIT 100`, guardID)
	} else {
		rows, err = db.DB.Query(baseQ + ` ORDER BY created_at DESC LIMIT 100`)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(&inc.ID, &inc.GuardID, &inc.Title, &inc.Description, &inc.PhotoURL, &inc.Severity, &inc.Lat, &inc.Lng, &inc.CreatedAt); err != nil {
			continue
		}
		incidents = append(incidents, inc)
	}
	_ = json.NewEncoder(w).Encode(incidents)
}

// GuardCreateIncident handles POST /api/guard/incidents
// Authenticated via X-Guard-Token header (no JWT required — for Guard PWA).
func GuardCreateIncident(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token := r.Header.Get("X-Guard-Token")
	if token == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing guard token"})
		return
	}

	var guardID int
	err := db.DB.QueryRow(
		`SELECT id FROM guards WHERE guard_token = $1 AND deleted_at IS NULL`, token,
	).Scan(&guardID)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		// Try plain form as fallback
		_ = r.ParseForm()
	}

	title := r.FormValue("title")
	desc := r.FormValue("description")
	severity := r.FormValue("severity")
	latStr := r.FormValue("lat")
	lngStr := r.FormValue("lng")

	if title == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "title required"})
		return
	}
	if severity == "" {
		severity = "medium"
	}

	var lat, lng *float64
	if v, err2 := strconv.ParseFloat(latStr, 64); err2 == nil && v != 0 {
		lat = &v
	}
	if v, err2 := strconv.ParseFloat(lngStr, 64); err2 == nil && v != 0 {
		lng = &v
	}

	// Phase-2: Upload photo to Cloudinary if provided
	var photoURL *string
	file, header, fileErr := r.FormFile("photo")
	if fileErr == nil {
		defer file.Close()
		url, uploadErr := uploadGuardPhoto(file, header)
		if uploadErr != nil {
			log.Printf("GuardCreateIncident photo upload warning: %v", uploadErr)
			// Non-fatal — continue without photo
		} else {
			photoURL = url
		}
	}

	var incidentID int
	err = db.DB.QueryRow(
		`INSERT INTO incidents (guard_id, title, description, severity, lat, lng, photo_url)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		guardID, title, desc, severity, lat, lng, photoURL,
	).Scan(&incidentID)
	if err != nil {
		log.Printf("GuardCreateIncident insert error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}

	// Phase-2: Create notification for admins/managers on any incident (especially SOS)
	go func() {
		guardNameForNotif := ""
		var gn string
		if qErr := db.DB.QueryRow(`SELECT name FROM guards WHERE id=$1`, guardID).Scan(&gn); qErr == nil {
			guardNameForNotif = gn
		}
		notifType := "warning"
		if severity == "sos" {
			notifType = "sos"
		}
		msg := fmt.Sprintf("🚨 Incident (%s) reported by %s: %s", severity, guardNameForNotif, title)
		helpers.NotifyUsersByRole(db.DB, []string{"superadmin", "manager"}, msg, notifType)
	}()

	// Broadcast to admin clients so live tracking panel updates in real-time
	globalHub.mu.RLock()
	gState, hasState := globalHub.guardStates[guardID]
	globalHub.mu.RUnlock()
	guardName := ""
	if hasState {
		guardName = gState.GuardName
	} else {
		// Fetch guard name from DB if not in hub
		var gn string
		if qErr := db.DB.QueryRow(`SELECT name FROM guards WHERE id=$1`, guardID).Scan(&gn); qErr == nil {
			guardName = gn
		}
	}

	// Phase-3: If severity is SOS, update hub guardStates so live map shows SOS marker
	if severity == "sos" {
		globalHub.mu.Lock()
		prev := globalHub.guardStates[guardID]
		sosUpdate := OutgoingMsg{
			Type:      "guard_update",
			GuardID:   guardID,
			GuardName: guardName,
			Lat:       prev.Lat,
			Lng:       prev.Lng,
			ClockedIn: prev.ClockedIn,
			Severity:  "sos",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		if lat != nil {
			sosUpdate.Lat = *lat
		}
		if lng != nil {
			sosUpdate.Lng = *lng
		}
		globalHub.guardStates[guardID] = sosUpdate
		globalHub.mu.Unlock()
		// Broadcast as guard_update so live map picks it up
		globalHub.broadcastToAdmins(sosUpdate)
	} else {
		// Non-SOS incident: broadcast as "incident" type for feed panel
		broadcastMsg := OutgoingMsg{
			Type:      "incident",
			GuardID:   guardID,
			GuardName: guardName,
			Severity:  severity,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		if lat != nil {
			broadcastMsg.Lat = *lat
		}
		if lng != nil {
			broadcastMsg.Lng = *lng
		}
		globalHub.broadcastToAdmins(broadcastMsg)
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": incidentID, "ok": true})
}
