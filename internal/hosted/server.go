package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"sift/internal/digest"
	"sift/internal/event"
	"sift/internal/zitadel"
)

const (
	defaultListLimit = 20
	maxListLimit     = 100
)

type EventStore interface {
	Ping(ctx context.Context) error
	ListEvents(ctx context.Context) ([]event.Record, error)
	GetEvent(ctx context.Context, eventID string) (event.Record, bool, error)
}

type Options struct {
	Store                   EventStore
	Validator               zitadel.Validator
	OutputDir               string
	AllowedWebSocketOrigins []string
	Now                     func() time.Time
}

type Server struct {
	store     EventStore
	validator zitadel.Validator
	outputDir string
	now       func() time.Time

	syncMu        sync.RWMutex
	lastRunID     string
	lastRunAt     time.Time
	lastSuccessAt time.Time
	lastError     string

	wsMu     sync.RWMutex
	clients  map[*wsClient]struct{}
	upgrader websocket.Upgrader

	allowedWebSocketOrigins map[string]struct{}
}

type wsClient struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	closeOnce sync.Once
}

type eventListEnvelope struct {
	Items      []event.Record `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

type digestEnvelope struct {
	Scope       string `json:"scope"`
	Window      string `json:"window"`
	MarkdownURL string `json:"markdown_url"`
	EventCount  int    `json:"event_count"`
	GeneratedAt string `json:"generated_at"`
}

type streamEnvelope struct {
	Type        string         `json:"type"`
	GeneratedAt string         `json:"generated_at"`
	Payload     map[string]any `json:"payload,omitempty"`
}

func New(options Options) (*Server, error) {
	if options.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if options.Validator == nil {
		return nil, fmt.Errorf("validator is required")
	}

	outputDir := strings.TrimSpace(options.OutputDir)
	if outputDir == "" {
		outputDir = "output"
	}

	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	allowedOrigins := make(map[string]struct{}, len(options.AllowedWebSocketOrigins))
	for _, rawOrigin := range options.AllowedWebSocketOrigins {
		origin, err := normalizeOrigin(rawOrigin)
		if err != nil {
			return nil, fmt.Errorf("invalid websocket allowed origin %q: %w", rawOrigin, err)
		}
		allowedOrigins[origin] = struct{}{}
	}

	server := &Server{
		store:                   options.Store,
		validator:               options.Validator,
		outputDir:               outputDir,
		now:                     now,
		clients:                 make(map[*wsClient]struct{}),
		allowedWebSocketOrigins: allowedOrigins,
	}
	server.upgrader = websocket.Upgrader{
		CheckOrigin: server.checkWebSocketOrigin,
	}

	return server, nil
}

func normalizeOrigin(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("origin value is empty")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse origin: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("origin must use http or https scheme")
	}
	if parsed.Host == "" {
		return "", errors.New("origin host is required")
	}

	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func (s *Server) checkWebSocketOrigin(r *http.Request) bool {
	originHeader := strings.TrimSpace(r.Header.Get("Origin"))
	if originHeader == "" {
		// If an explicit allowlist is configured, require Origin header presence.
		return len(s.allowedWebSocketOrigins) == 0
	}

	origin, err := normalizeOrigin(originHeader)
	if err != nil {
		return false
	}

	if len(s.allowedWebSocketOrigins) > 0 {
		_, ok := s.allowedWebSocketOrigins[origin]
		return ok
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}

	return strings.EqualFold(originURL.Host, r.Host)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/v1/events", s.requireAuth(s.handleListEvents))
	mux.HandleFunc("/v1/events/", s.requireAuth(s.handleGetEvent))
	mux.HandleFunc("/v1/digests/", s.requireAuth(s.handleGetDigest))
	mux.HandleFunc("/v1/ws", s.requireAuth(s.handleWebSocket))
	return mux
}

func (s *Server) MarkSyncSuccess(runID string, at time.Time) {
	s.syncMu.Lock()
	s.lastRunID = runID
	s.lastRunAt = at.UTC()
	s.lastSuccessAt = at.UTC()
	s.lastError = ""
	s.syncMu.Unlock()

	generatedAt := at.UTC().Format(time.RFC3339)
	s.broadcast(streamEnvelope{
		Type:        "event.upserted",
		GeneratedAt: generatedAt,
		Payload: map[string]any{
			"run_id": runID,
		},
	})
	for _, window := range []string{"24h", "7d"} {
		s.broadcast(streamEnvelope{
			Type:        "digest.updated",
			GeneratedAt: generatedAt,
			Payload: map[string]any{
				"scope":  "crypto",
				"window": window,
			},
		})
	}
}

func (s *Server) MarkSyncFailure(at time.Time, err error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	s.lastRunAt = at.UTC()
	if err != nil {
		s.lastError = err.Error()
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.store.Ping(pingCtx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"detail": "store is not reachable",
		})
		return
	}

	s.syncMu.RLock()
	lastRunID := s.lastRunID
	lastRunAt := s.lastRunAt
	lastSuccessAt := s.lastSuccessAt
	lastError := s.lastError
	s.syncMu.RUnlock()

	if lastSuccessAt.IsZero() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "starting",
			"detail": "waiting for first successful sync",
		})
		return
	}

	if lastError != "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"detail": "last sync failed",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":          "ready",
		"last_run_id":     lastRunID,
		"last_run_at":     lastRunAt.Format(time.RFC3339),
		"last_success_at": lastSuccessAt.Format(time.RFC3339),
	})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query()
	if strings.TrimSpace(query.Get("cursor")) != "" {
		writeJSONError(w, http.StatusBadRequest, "cursor is not supported in this build")
		return
	}

	limit, err := parseLimit(query.Get("limit"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	records, err := s.store.ListEvents(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list events failed")
		return
	}

	filtered, err := filterEvents(records, query)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	writeJSON(w, http.StatusOK, eventListEnvelope{
		Items:      filtered,
		NextCursor: nil,
	})
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	eventID := strings.TrimPrefix(r.URL.Path, "/v1/events/")
	if eventID == "" || strings.Contains(eventID, "/") {
		writeJSONError(w, http.StatusNotFound, "event not found")
		return
	}

	rec, found, err := s.store.GetEvent(r.Context(), eventID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "get event failed")
		return
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "event not found")
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleGetDigest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/digests/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		writeJSONError(w, http.StatusNotFound, "digest not found")
		return
	}

	scope := parts[0]
	window := parts[1]

	records, err := s.store.ListEvents(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list events failed")
		return
	}

	projection, err := digest.BuildProjection(records, s.outputDir, scope, window, s.now())
	if err != nil {
		if errors.Is(err, digest.ErrInvalidWindowValue) || errors.Is(err, digest.ErrWindowValueRequired) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "build digest failed")
		return
	}

	writeJSON(w, http.StatusOK, digestEnvelope{
		Scope:       projection.Envelope.Scope,
		Window:      projection.Envelope.Window,
		MarkdownURL: fmt.Sprintf("/output/digests/%s/%s.md", projection.Envelope.Scope, projection.Envelope.Window),
		EventCount:  len(projection.Envelope.EventIDs),
		GeneratedAt: projection.Envelope.GeneratedAt,
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &wsClient{conn: conn}
	s.registerClient(client)
	defer s.unregisterClient(client)

	s.writeToClient(client, streamEnvelope{
		Type:        "connected",
		GeneratedAt: s.now().UTC().Format(time.RFC3339),
	})

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := tokenFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}

		if err := s.validator.Validate(r.Context(), token); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}

		next(w, r)
	}
}

func tokenFromRequest(r *http.Request) (string, error) {
	if token, err := zitadel.ExtractBearerToken(r.Header.Get("Authorization")); err == nil {
		return token, nil
	}

	return "", fmt.Errorf("missing bearer token")
}

func (s *Server) registerClient(client *wsClient) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	s.clients[client] = struct{}{}
}

func (s *Server) unregisterClient(client *wsClient) {
	s.wsMu.Lock()
	delete(s.clients, client)
	s.wsMu.Unlock()
	client.closeOnce.Do(func() {
		_ = client.conn.Close()
	})
}

func (s *Server) broadcast(message streamEnvelope) {
	payload, err := json.Marshal(message)
	if err != nil {
		return
	}

	s.wsMu.RLock()
	clients := make([]*wsClient, 0, len(s.clients))
	for client := range s.clients {
		clients = append(clients, client)
	}
	s.wsMu.RUnlock()

	for _, client := range clients {
		if err := s.writePayload(client, payload); err != nil {
			s.unregisterClient(client)
		}
	}
}

func (s *Server) writeToClient(client *wsClient, message streamEnvelope) {
	payload, err := json.Marshal(message)
	if err != nil {
		return
	}
	_ = s.writePayload(client, payload)
}

func (s *Server) writePayload(client *wsClient, payload []byte) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	if err := client.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if err := client.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return err
	}
	return nil
}

func filterEvents(records []event.Record, query url.Values) ([]event.Record, error) {
	category := strings.ToLower(strings.TrimSpace(query.Get("category")))
	eventTypes := querySet(query["event_type"], strings.ToLower)
	statuses := querySet(query["status"], strings.ToLower)
	assets := querySet(query["asset"], strings.ToUpper)
	topics := querySet(query["topic"], strings.ToLower)

	var since *time.Time
	if sinceRaw := strings.TrimSpace(query.Get("since")); sinceRaw != "" {
		parsed, err := time.Parse(time.RFC3339, sinceRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid since value, expected RFC3339")
		}
		parsed = parsed.UTC()
		since = &parsed
	}

	filtered := make([]event.Record, 0, len(records))
	for _, rec := range records {
		if category != "" && strings.ToLower(rec.Category) != category {
			continue
		}
		if len(eventTypes) > 0 {
			if _, ok := eventTypes[strings.ToLower(rec.EventType)]; !ok {
				continue
			}
		}
		if len(statuses) > 0 {
			if _, ok := statuses[strings.ToLower(rec.Status)]; !ok {
				continue
			}
		}
		if len(assets) > 0 && !matchesAny(rec.Assets, assets, strings.ToUpper) {
			continue
		}
		if len(topics) > 0 && !matchesAny(rec.Topics, topics, strings.ToLower) {
			continue
		}
		if since != nil {
			publishedAt, err := time.Parse(time.RFC3339, rec.PublishedAt)
			if err != nil {
				return nil, fmt.Errorf("invalid event timestamp in store")
			}
			if publishedAt.Before(*since) {
				continue
			}
		}

		filtered = append(filtered, rec)
	}

	return filtered, nil
}

func matchesAny(values []string, set map[string]struct{}, normalize func(string) string) bool {
	for _, value := range values {
		if _, ok := set[normalize(value)]; ok {
			return true
		}
	}
	return false
}

func querySet(values []string, normalize func(string) string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[normalize(trimmed)] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func parseLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultListLimit, nil
	}

	limit, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit < 1 || limit > maxListLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxListLimit)
	}
	return limit, nil
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(payload); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buffer.Bytes())
}
