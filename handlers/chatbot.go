package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"regexp"
	"server/db"
	"server/services"
	chatSvc "server/services/chat"
	"strings"
	"time"
	"unicode/utf8"
)

// ---- request / response types -------------------------------------------

// ChatRequest is the incoming body for POST /api/chat.
type ChatRequest struct {
	Message string                `json:"message"`
	History []chatSvc.HistoryTurn `json:"history"`
}

// ChatResponse is the outgoing body for POST /api/chat.
// The field "reply" replaces the old "response" field to match the new spec.
type ChatResponse struct {
	Reply   string               `json:"reply"`
	Actions []chatSvc.ChatAction `json:"actions,omitempty"`
}

// contactRequestPayload matches the contact_request action payload.
type contactRequestPayload struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
}

// ---- constants ----------------------------------------------------------

const (
	maxMessageRunes = 2000
	maxHistoryTurns = 10
)

var contactEmailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ---- handler ------------------------------------------------------------

// ChatHandler handles POST /api/chat.
// Auth is optional: if a valid JWT cookie is present, user context is populated;
// otherwise the request proceeds as a guest session.
func ChatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	start := time.Now()

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// --- Decode body ---
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	// --- Input validation & sanitisation ---
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "message is required"})
		return
	}
	if utf8.RuneCountInString(req.Message) > maxMessageRunes {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "message too long (max 2000 characters)"})
		return
	}

	// Cap history to last N turns
	if len(req.History) > maxHistoryTurns {
		req.History = req.History[len(req.History)-maxHistoryTurns:]
	}

	// Escape HTML for safe logging and downstream use
	sanitisedMessage := html.EscapeString(req.Message)

	// --- Optional auth: populate user context if a valid token cookie exists ---
	var userCtx *chatSvc.UserContext
	claims, authErr := parseTokenClaimsFromRequest(r)
	if authErr == nil {
		uid, _ := claims["user_id"].(float64)
		role, _ := claims["role"].(string)
		if role != "" {
			userCtx = &chatSvc.UserContext{
				ID:   int(uid),
				Role: role,
			}
		}
	}

	userLabel := "guest"
	roleLabel := "guest"
	if userCtx != nil {
		userLabel = fmt.Sprintf("u:%d", userCtx.ID)
		roleLabel = userCtx.Role
	}

	// --- Run the chat engine ---
	result, err := chatSvc.Run(db.DB, userCtx, sanitisedMessage, req.History)
	latency := time.Since(start)
	if err != nil {
		if errors.Is(err, chatSvc.ErrGeminiRateLimit) || errors.Is(err, chatSvc.ErrAllProvidersBusy) {
			log.Printf("[chat] ts=%s user=%s role=%s latency=%dms ALL_PROVIDERS_BUSY",
				time.Now().UTC().Format(time.RFC3339), userLabel, roleLabel, latency.Milliseconds())
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "AI assistant is busy, please try again in a moment"})
			return
		}
		log.Printf("[chat] ts=%s user=%s role=%s latency=%dms ERROR=%v",
			time.Now().UTC().Format(time.RFC3339), userLabel, roleLabel, latency.Milliseconds(), err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "chat service unavailable"})
		return
	}

	// --- Handle contact_request actions (server-side email dispatch) ---
	result.Actions = handleContactActions(result.Actions)

	// --- Structured log: timestamp, userId, role, intent, latency — no message body ---
	intent := actionsLabel(result.Actions)
	log.Printf("[chat] ts=%s user=%s role=%s intent=%s latency=%dms",
		time.Now().UTC().Format(time.RFC3339), userLabel, roleLabel, intent, latency.Milliseconds())

	_ = json.NewEncoder(w).Encode(ChatResponse{
		Reply:   result.Reply,
		Actions: result.Actions,
	})
}

// handleContactActions iterates the action list and, for any contact_request
// with a complete, valid payload, fires the existing email pipeline and removes
// the action from the response (it has been handled server-side).
// Actions with incomplete payloads are kept so the frontend can collect missing fields.
func handleContactActions(actions []chatSvc.ChatAction) []chatSvc.ChatAction {
	kept := make([]chatSvc.ChatAction, 0, len(actions))
	for _, a := range actions {
		if a.Type != "contact_request" {
			kept = append(kept, a)
			continue
		}

		payloadBytes, err := json.Marshal(a.Payload)
		if err != nil {
			kept = append(kept, a)
			continue
		}
		var p contactRequestPayload
		if err := json.Unmarshal(payloadBytes, &p); err != nil {
			kept = append(kept, a)
			continue
		}

		p.Name = strings.TrimSpace(p.Name)
		p.Email = strings.TrimSpace(strings.ToLower(p.Email))
		p.Message = strings.TrimSpace(p.Message)

		// If payload is incomplete, return the action so the frontend can collect fields.
		if p.Name == "" || !contactEmailRegex.MatchString(p.Email) || p.Message == "" {
			kept = append(kept, a)
			continue
		}

		// Acknowledge the enquirer using the existing email pipeline.
		services.EnqueueEmail(
			p.Email,
			p.Name,
			"We received your enquiry — Garud Kavach",
			services.EmailTemplate(
				"Thank you, "+html.EscapeString(p.Name)+"!",
				"<p>We have received your enquiry and our team will respond shortly.</p>"+
					`<div style="margin:16px 0;padding:12px 16px;background-color:#f8fafc;border-left:3px solid #ea580c;border-radius:4px;">`+
					"<p style=\"margin:0;color:#475569;font-size:14px;\"><strong>Your message:</strong></p>"+
					"<p style=\"margin:8px 0 0;color:#334155;\">"+html.EscapeString(p.Message)+"</p></div>",
				"Our team typically responds within 24 hours.",
			),
		)

		// Forward the enquiry to the company inbox using the same pipeline.
		services.EnqueueEmail(
			"contact@rakshakservice.com",
			"Garud Kavach Team",
			"Chatbot Enquiry from "+html.EscapeString(p.Name),
			services.EmailTemplate(
				"New Chatbot Enquiry",
				`<table style="margin:12px 0;border-collapse:collapse;width:100%;">`+
					`<tr><td style="padding:8px 16px 8px 0;color:#64748b;font-size:14px;vertical-align:top;">Name:</td><td style="padding:8px 0;font-weight:600;">`+html.EscapeString(p.Name)+`</td></tr>`+
					`<tr><td style="padding:8px 16px 8px 0;color:#64748b;font-size:14px;vertical-align:top;">Email:</td><td style="padding:8px 0;">`+html.EscapeString(p.Email)+`</td></tr>`+
					`<tr><td style="padding:8px 16px 8px 0;color:#64748b;font-size:14px;vertical-align:top;">Message:</td><td style="padding:8px 0;">`+html.EscapeString(p.Message)+`</td></tr>`+
					`</table>`,
				"Respond to the customer within 24 hours.",
			),
		)

		log.Printf("[chat] contact_request dispatched via email pipeline for email=%s", p.Email)
		// Do not keep this action — it was fully handled.
	}
	return kept
}

// actionsLabel returns a short string describing the actions list for logging.
func actionsLabel(actions []chatSvc.ChatAction) string {
	if len(actions) == 0 {
		return "reply"
	}
	types := make([]string, 0, len(actions))
	for _, a := range actions {
		types = append(types, a.Type)
	}
	return strings.Join(types, ",")
}
