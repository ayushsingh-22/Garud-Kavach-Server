// Package chat provides the LLM-backed chat engine.
// Primary provider: Google Gemini (GEMINI_API_KEY / GEMINI_API_KEY_2).
// Fallback provider: Groq (GROQ_API_KEY) — used when all Gemini attempts fail.
// The two-step pattern:
//  1. Intent extraction: send user message → ask LLM for JSON intent.
//  2. Data fetch + reply: if needsData, call the matching dataScope,
//     then send a second LLM call with data context for the natural-language reply.
package chat

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ErrGeminiRateLimit is returned when every Gemini key+model pair responded 429.
var ErrGeminiRateLimit = errors.New("gemini_rate_limited")

// ErrAllProvidersBusy is returned when Gemini AND Groq are both unavailable.
var ErrAllProvidersBusy = errors.New("all_providers_busy")

// HistoryTurn is one turn of conversation history.
type HistoryTurn struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// Intent is the structured result of step 1.
type Intent struct {
	Intent    string                 `json:"intent"`
	Params    map[string]interface{} `json:"params"`
	NeedsData bool                   `json:"needsData"`
}

// ChatAction is a structured action the frontend can act on.
type ChatAction struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// EngineResult is the full response from the engine.
type EngineResult struct {
	Reply   string       `json:"reply"`
	Actions []ChatAction `json:"actions,omitempty"`
}

// UserContext carries the authenticated user's identity. nil = guest.
type UserContext struct {
	ID   int
	Role string
}

// systemPromptForRole returns a role-aware system prompt for intent detection.
func systemPromptForRole(role string) string {
	allowed := allowedIntentsForRole(role)
	return fmt.Sprintf(`You are the Garud Kavach security-services chatbot assistant.
The current user's role is: %s.

ALLOWED INTENTS for this role: %s

Your task: analyse the user's message and return ONLY a JSON object with these fields:
{
  "intent": "<one of the allowed intents above, or 'unknown' if it does not fit>",
  "params": { "<optional key-value pairs extracted from the message, e.g. month, status, id>" },
  "needsData": <true if you need live data from the database to answer, false otherwise>
}

Rules:
- Never choose an intent outside the allowed list. Use "unknown" if the request is out of scope.
- If the message is a greeting or small talk, use intent "company_info" with needsData: false.
- For contact or booking intents, set needsData: false.
- Respond with ONLY the JSON object — no markdown, no explanation.`, role, allowed)
}

// replySystemPrompt returns a system prompt for the final reply step.
func replySystemPrompt(role, intent string, data ScopedData) string {
	dataJSON, _ := json.Marshal(data)
	return fmt.Sprintf(`You are the Garud Kavach security-services chatbot assistant.
The user's role is: %s.
Their intent is: %s.
Relevant data (do NOT expose raw internal IDs or unnecessary PII in your reply):
%s

Formatting rules — MUST follow:
- Use **bold** for labels and headings.
- Use bullet lists (- item) for enumerations, breakdowns, or lists of items.
- Use a Markdown table when presenting tabular data (e.g. user lists, financial rows).
- Use ### for section headings when the reply has multiple sections.
- Keep prose short. Prefer structured output over long paragraphs.
- Do NOT output raw JSON or internal IDs.

Answer the user in a friendly, concise manner based on this data.
If the intent is "book_service" or "contact_request", guide the user to take that action.
If the data is empty or the intent is "unknown", politely say you cannot help with that from this account.`, role, intent, string(dataJSON))
}

// guestReplySystemPrompt returns a system prompt for unauthenticated users.
func guestReplySystemPrompt(intent string, data ScopedData) string {
	dataJSON, _ := json.Marshal(data)
	return fmt.Sprintf(`You are the Garud Kavach security-services chatbot assistant.
The visitor is not logged in (guest).
Their intent is: %s.
Context data:
%s

Formatting rules — MUST follow:
- Use **bold** for labels and headings.
- Use bullet lists (- item) for enumerations or lists of services/prices.
- Use ### for section headings when the reply has multiple sections.
- Keep prose short. Prefer structured output over long paragraphs.
- Do NOT output raw JSON or internal IDs.

Help the visitor learn about Garud Kavach, its services, and pricing.
If they want to book a service, guide them to the booking form.
If they want to contact the team, tell them you can send a message on their behalf.
Be friendly and concise. Do not expose any internal or user-specific data.`, intent, string(dataJSON))
}

func allowedIntentsForRole(role string) string {
	var m map[string]bool
	switch role {
	case "customer":
		m = CustomerAllowedIntents
	case "hr":
		m = HRAllowedIntents
	case "finance":
		m = FinanceAllowedIntents
	case "manager":
		m = ManagerAllowedIntents
	case "superadmin":
		m = AdminAllowedIntents
	default:
		m = PublicAllowedIntents
	}
	var list []string
	for k := range m {
		list = append(list, k)
	}
	return strings.Join(list, ", ")
}

// Run executes the two-step chatbot pipeline.
func Run(db *sql.DB, user *UserContext, message string, history []HistoryTurn) (*EngineResult, error) {
	apiKey1 := os.Getenv("GEMINI_API_KEY")
	apiKey2 := os.Getenv("GEMINI_API_KEY_2")
	groqKey := os.Getenv("GROQ_API_KEY")
	if apiKey1 == "" && apiKey2 == "" && groqKey == "" {
		return nil, fmt.Errorf("no LLM API keys configured (GEMINI_API_KEY, GEMINI_API_KEY_2, or GROQ_API_KEY)")
	}

	var geminiKeys []string
	if apiKey1 != "" {
		geminiKeys = append(geminiKeys, apiKey1)
	}
	if apiKey2 != "" {
		geminiKeys = append(geminiKeys, apiKey2)
	}

	role := "guest"
	if user != nil {
		role = user.Role
	}

	// --- Step 1: intent extraction ---
	intentJSON, err := callLLM(geminiKeys, groqKey, systemPromptForRole(role), history, message)
	if err != nil {
		return nil, fmt.Errorf("intent extraction failed: %w", err)
	}

	var intent Intent
	intentJSON = strings.TrimSpace(intentJSON)
	// Strip possible markdown code fences
	intentJSON = strings.TrimPrefix(intentJSON, "```json")
	intentJSON = strings.TrimPrefix(intentJSON, "```")
	intentJSON = strings.TrimSuffix(intentJSON, "```")
	if err := json.Unmarshal([]byte(intentJSON), &intent); err != nil {
		// Fallback: treat as company_info
		intent = Intent{Intent: "company_info", Params: nil, NeedsData: false}
	}

	// --- Out-of-scope guard ---
	if intent.Intent == "unknown" {
		return &EngineResult{
			Reply: "I'm sorry, I can't help with that from your account. Please contact us directly at contact@rakshakservice.com if you need further assistance.",
		}, nil
	}

	// --- Step 2: data fetch (if needed) ---
	var scopedData ScopedData

	if intent.NeedsData {
		var scopeErr error
		if user == nil {
			scopedData, scopeErr = PublicScope(intent.Intent, intent.Params)
		} else {
			switch user.Role {
			case "customer":
				scopedData, scopeErr = CustomerScope(db, user.ID, intent.Intent, intent.Params)
			case "hr":
				scopedData, scopeErr = HRScope(db, user.ID, intent.Intent, intent.Params)
			case "finance":
				scopedData, scopeErr = FinanceScope(db, user.ID, intent.Intent, intent.Params)
			case "manager":
				scopedData, scopeErr = ManagerScope(db, user.ID, intent.Intent, intent.Params)
			case "superadmin":
				scopedData, scopeErr = AdminScope(db, user.ID, intent.Intent, intent.Params)
			default:
				scopedData, scopeErr = PublicScope(intent.Intent, intent.Params)
			}
		}

		if scopeErr != nil {
			if IsOutOfScope(scopeErr) {
				return &EngineResult{
					Reply: "I can't help with that from your account. Please contact us directly if you need further assistance.",
				}, nil
			}
			return nil, fmt.Errorf("data scope error: %w", scopeErr)
		}
	} else {
		// Even without DB data, fetch static context for public intents
		switch intent.Intent {
		case "service_catalog", "pricing", "company_info":
			if user == nil {
				scopedData, _ = PublicScope(intent.Intent, intent.Params)
			}
		}
	}

	// --- Step 3: build actions if applicable ---
	var actions []ChatAction
	if intent.Intent == "book_service" {
		serviceID, _ := intent.Params["service"].(string)
		actions = append(actions, ChatAction{
			Type:    "book_service",
			Payload: map[string]interface{}{"serviceId": serviceID},
		})
	}
	if intent.Intent == "contact_request" {
		// Params may be empty on first detection (user hasn't provided data yet).
		// They will be populated when the user submits the inline contact form.
		name, _ := intent.Params["name"].(string)
		email, _ := intent.Params["email"].(string)
		message, _ := intent.Params["message"].(string)
		actions = append(actions, ChatAction{
			Type: "contact_request",
			Payload: map[string]interface{}{
				"name":    name,
				"email":   email,
				"message": message,
			},
		})
	}

	// --- Step 4: generate natural-language reply ---
	var sysPrompt string
	if user == nil {
		sysPrompt = guestReplySystemPrompt(intent.Intent, scopedData)
	} else {
		sysPrompt = replySystemPrompt(user.Role, intent.Intent, scopedData)
	}

	reply, err := callLLM(geminiKeys, groqKey, sysPrompt, history, message)
	if err != nil {
		return nil, fmt.Errorf("reply generation failed: %w", err)
	}

	return &EngineResult{
		Reply:   reply,
		Actions: actions,
	}, nil
}

// ---- Provider router ---------------------------------------------------

// callLLM tries Gemini first (all keys × all models).
// If every Gemini attempt fails it falls back to Groq (all models).
// Returns ErrAllProvidersBusy only when all providers returned 429.
func callLLM(geminiKeys []string, groqKey string, systemPrompt string, history []HistoryTurn, userMessage string) (string, error) {
	var geminiErr error

	// Try Gemini first
	if len(geminiKeys) > 0 {
		text, err := callGeminiWithFallback(geminiKeys, systemPrompt, history, userMessage)
		if err == nil {
			return text, nil
		}
		geminiErr = err
	}

	// Groq fallback
	if groqKey != "" {
		text, err := callGroqWithFallback(groqKey, systemPrompt, history, userMessage)
		if err == nil {
			return text, nil
		}
		// Both providers failed — report ErrAllProvidersBusy only when both were rate-limited
		if errors.Is(err, ErrGeminiRateLimit) && errors.Is(geminiErr, ErrGeminiRateLimit) {
			return "", ErrAllProvidersBusy
		}
		return "", fmt.Errorf("all providers failed (gemini: %v, groq: %v)", geminiErr, err)
	}

	// No Groq key — surface Gemini's error directly
	if geminiErr != nil {
		return "", geminiErr
	}
	return "", fmt.Errorf("no LLM provider available")
}

// ---- Gemini HTTP client -------------------------------------------------

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// geminiModels is the ordered fallback list of models to try.
// The first model that returns a non-empty response wins.
var geminiModels = []string{
	"gemini-2.0-flash",
	"gemini-2.0-flash-lite",
	"gemini-1.5-flash",
	"gemini-2.5-flash-preview-04-17",
	"gemini-2.5-pro-preview-03-25",
}

// callGeminiWithFallback tries each API key with every model in geminiModels.
// It only returns ErrGeminiRateLimit if ALL combinations were rate-limited.
// Any other error type is treated as a transient failure and the next
// key/model pair is tried.
func callGeminiWithFallback(apiKeys []string, systemPrompt string, history []HistoryTurn, userMessage string) (string, error) {
	var lastErr error
	allRateLimited := true

	for _, apiKey := range apiKeys {
		for _, model := range geminiModels {
			text, err := callGemini(apiKey, model, systemPrompt, history, userMessage)
			if err == nil {
				return text, nil
			}
			if !errors.Is(err, ErrGeminiRateLimit) {
				allRateLimited = false
			}
			lastErr = err
		}
	}

	// Surface a clear rate-limit sentinel only when every attempt was a 429.
	if allRateLimited {
		return "", ErrGeminiRateLimit
	}
	return "", fmt.Errorf("all Gemini key/model combinations failed; last error: %w", lastErr)
}

// ---- Groq HTTP client (OpenAI-compatible) -------------------------------

// groqModels is the ordered list of Groq models to try as fallback.
var groqModels = []string{
	"llama-3.3-70b-versatile",
	"llama-3.1-8b-instant",
	"gemma2-9b-it",
	"mixtral-8x7b-32768",
}

type groqRequest struct {
	Model       string        `json:"model"`
	Messages    []groqMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// callGroqWithFallback tries every model in groqModels with the single Groq key.
// Returns ErrGeminiRateLimit (reused sentinel) when the Groq key itself is rate-limited.
func callGroqWithFallback(apiKey string, systemPrompt string, history []HistoryTurn, userMessage string) (string, error) {
	var lastErr error
	allRateLimited := true

	for _, model := range groqModels {
		text, err := callGroq(apiKey, model, systemPrompt, history, userMessage)
		if err == nil {
			return text, nil
		}
		if !errors.Is(err, ErrGeminiRateLimit) {
			allRateLimited = false
		}
		lastErr = err
	}

	if allRateLimited {
		return "", ErrGeminiRateLimit
	}
	return "", fmt.Errorf("all Groq models failed; last error: %w", lastErr)
}

// callGroq makes a single request to the Groq chat-completions endpoint.
func callGroq(apiKey, model, systemPrompt string, history []HistoryTurn, userMessage string) (string, error) {
	// Build OpenAI-format message list
	var messages []groqMessage
	messages = append(messages, groqMessage{Role: "system", Content: systemPrompt})

	start := 0
	if len(history) > 10 {
		start = len(history) - 10
	}
	for _, h := range history[start:] {
		groqRole := h.Role
		if groqRole == "assistant" {
			groqRole = "assistant"
		}
		messages = append(messages, groqMessage{Role: groqRole, Content: h.Content})
	}
	messages = append(messages, groqMessage{Role: "user", Content: userMessage})

	body, err := json.Marshal(groqRequest{
		Model:       model,
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   1024,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == 429 {
		return "", ErrGeminiRateLimit // reuse sentinel for rate-limit tracking
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("groq API error %d (model=%s): %s", resp.StatusCode, model, string(respBody))
	}

	var groqResp groqResponse
	if err := json.Unmarshal(respBody, &groqResp); err != nil {
		return "", err
	}

	if len(groqResp.Choices) == 0 || groqResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty response from Groq model %s", model)
	}

	return groqResp.Choices[0].Message.Content, nil
}

// callGemini makes a single request to the Gemini REST API.
func callGemini(apiKey, model, systemPrompt string, history []HistoryTurn, userMessage string) (string, error) {
	var contents []geminiContent

	// System instruction as the first model turn
	contents = append(contents, geminiContent{
		Role:  "model",
		Parts: []geminiPart{{Text: systemPrompt}},
	})

	// Conversation history (capped at 10 turns)
	start := 0
	if len(history) > 10 {
		start = len(history) - 10
	}
	for _, h := range history[start:] {
		gemRole := "user"
		if h.Role == "assistant" {
			gemRole = "model"
		}
		contents = append(contents, geminiContent{
			Role:  gemRole,
			Parts: []geminiPart{{Text: h.Content}},
		})
	}

	// Current user message
	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: userMessage}},
	})

	payload, err := json.Marshal(geminiRequest{Contents: contents})
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, apiKey,
	)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == 429 {
		return "", ErrGeminiRateLimit
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini API error %d (model=%s): %s", resp.StatusCode, model, string(body))
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(body, &gemResp); err != nil {
		return "", err
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from model %s", model)
	}

	return gemResp.Candidates[0].Content.Parts[0].Text, nil
}
