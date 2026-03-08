package hosted

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"sift/internal/event"
)

type fakeStore struct {
	events  []event.Record
	pingErr error
}

func (f *fakeStore) Ping(_ context.Context) error {
	return f.pingErr
}

func (f *fakeStore) ListEvents(_ context.Context) ([]event.Record, error) {
	out := make([]event.Record, 0, len(f.events))
	out = append(out, f.events...)
	return out, nil
}

func (f *fakeStore) GetEvent(_ context.Context, eventID string) (event.Record, bool, error) {
	for _, rec := range f.events {
		if rec.EventID == eventID {
			return rec, true, nil
		}
	}
	return event.Record{}, false, nil
}

type fakeValidator struct {
	validToken string
	err        error
}

func (f fakeValidator) Validate(_ context.Context, rawToken string) error {
	if f.err != nil {
		return f.err
	}
	if f.validToken == "" {
		return nil
	}
	if rawToken != f.validToken {
		return errors.New("invalid token")
	}
	return nil
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store:     &fakeStore{},
		Validator: fakeValidator{},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
}

func TestNewRejectsInvalidAllowedOrigin(t *testing.T) {
	t.Parallel()

	_, err := New(Options{
		Store:                   &fakeStore{},
		Validator:               fakeValidator{},
		OutputDir:               "output",
		AllowedWebSocketOrigins: []string{"not-a-url"},
		Now:                     fixedNow,
	})
	if err == nil {
		t.Fatal("expected New to reject invalid websocket origin")
	}
}

func TestListEventsRequiresBearer(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store:     &fakeStore{},
		Validator: fakeValidator{},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/events", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
}

func TestListEventsFiltersAndLimit(t *testing.T) {
	t.Parallel()

	records := []event.Record{
		{
			EventID:     "evt_btc",
			Category:    "crypto",
			EventType:   "policy",
			Status:      "multi_source_verified",
			Assets:      []string{"BTC"},
			Topics:      []string{"policy"},
			PublishedAt: "2026-03-08T09:00:00Z",
			Title:       "SEC updates BTC ETF guidance",
		},
		{
			EventID:     "evt_eth",
			Category:    "crypto",
			EventType:   "listing",
			Status:      "rumor",
			Assets:      []string{"ETH"},
			Topics:      []string{"exchange_listing"},
			PublishedAt: "2026-03-08T08:00:00Z",
			Title:       "Exchange listing rumor for ETH pair",
		},
	}

	srv, err := New(Options{
		Store: &fakeStore{
			events: records,
		},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/events?asset=BTC&limit=1", nil)
	req.Header.Set("Authorization", "Bearer token123")
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope eventListEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(envelope.Items) != 1 {
		t.Fatalf("unexpected item count: %d", len(envelope.Items))
	}
	if envelope.Items[0].EventID != "evt_btc" {
		t.Fatalf("unexpected event id: %s", envelope.Items[0].EventID)
	}
}

func TestGetEventNotFound(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store: &fakeStore{
			events: []event.Record{},
		},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/events/evt_missing", nil)
	req.Header.Set("Authorization", "Bearer token123")
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
}

func TestDigestEndpoint(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store: &fakeStore{
			events: []event.Record{
				{
					EventID:     "evt_btc",
					Category:    "crypto",
					Assets:      []string{"BTC"},
					Topics:      []string{"policy"},
					PublishedAt: "2026-03-08T09:00:00Z",
					Title:       "SEC updates BTC ETF guidance",
				},
			},
		},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/digests/crypto/24h", nil)
	req.Header.Set("Authorization", "Bearer token123")
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}

	var payload digestEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Scope != "crypto" || payload.Window != "24h" {
		t.Fatalf("unexpected digest identity: scope=%s window=%s", payload.Scope, payload.Window)
	}
	if payload.EventCount != 1 {
		t.Fatalf("unexpected event count: %d", payload.EventCount)
	}
	if !strings.Contains(payload.MarkdownURL, "/output/digests/crypto/24h.md") {
		t.Fatalf("unexpected markdown url: %s", payload.MarkdownURL)
	}
}

func TestDigestEndpointRejectsInvalidWindow(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store: &fakeStore{
			events: []event.Record{},
		},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/digests/crypto/bad-window", nil)
	req.Header.Set("Authorization", "Bearer token123")
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReadyzTransitions(t *testing.T) {
	t.Parallel()

	now := fixedNow()
	srv, err := New(Options{
		Store: &fakeStore{},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status before first sync: %d", recorder.Code)
	}

	srv.MarkSyncSuccess("run_1", now)
	recorder = httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status after success: %d", recorder.Code)
	}

	srv.MarkSyncFailure(now.Add(time.Minute), errors.New("boom"))
	recorder = httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status after failure: %d", recorder.Code)
	}
}

func TestWebSocketReceivesPostSyncEvents(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store: &fakeStore{},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	wsURL, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	wsURL.Scheme = "ws"
	wsURL.Path = "/v1/ws"

	header := http.Header{}
	header.Set("Authorization", "Bearer token123")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read connected message: %v", err)
	}
	var connected streamEnvelope
	if err := json.Unmarshal(message, &connected); err != nil {
		t.Fatalf("decode connected message: %v", err)
	}
	if connected.Type != "connected" {
		t.Fatalf("unexpected first message type: %s", connected.Type)
	}

	srv.MarkSyncSuccess("run_1", fixedNow())

	types := make(map[string]struct{})
	deadline := time.Now().Add(3 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	for i := 0; i < 3; i++ {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read sync message: %v", err)
		}
		var msg streamEnvelope
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatalf("decode sync message: %v", err)
		}
		types[msg.Type] = struct{}{}
	}

	if _, ok := types["event.upserted"]; !ok {
		t.Fatal("expected event.upserted message")
	}
	if _, ok := types["digest.updated"]; !ok {
		t.Fatal("expected digest.updated message")
	}
}

func TestWebSocketRejectsCrossOriginByDefault(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store: &fakeStore{},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	wsURL, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	wsURL.Scheme = "ws"
	wsURL.Path = "/v1/ws"

	header := http.Header{}
	header.Set("Authorization", "Bearer token123")
	header.Set("Origin", "https://evil.example")
	_, response, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	if err == nil {
		t.Fatal("expected websocket dial to fail for cross-origin request")
	}
	if response == nil {
		t.Fatal("expected HTTP response for failed websocket handshake")
	}
	if response.StatusCode < 400 {
		t.Fatalf("unexpected status code for failed handshake: %d", response.StatusCode)
	}
}

func TestWebSocketAllowsConfiguredOrigin(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store: &fakeStore{},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir:               "output",
		AllowedWebSocketOrigins: []string{"https://console.sift.local"},
		Now:                     fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	wsURL, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	wsURL.Scheme = "ws"
	wsURL.Path = "/v1/ws"

	header := http.Header{}
	header.Set("Authorization", "Bearer token123")
	header.Set("Origin", "https://console.sift.local")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	if err != nil {
		t.Fatalf("dial websocket with configured origin: %v", err)
	}
	defer conn.Close()

	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read connected message: %v", err)
	}

	var connected streamEnvelope
	if err := json.Unmarshal(message, &connected); err != nil {
		t.Fatalf("decode connected message: %v", err)
	}
	if connected.Type != "connected" {
		t.Fatalf("unexpected connected message type: %s", connected.Type)
	}
}

func TestWebSocketRejectsQueryTokenAuthentication(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store: &fakeStore{},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir: "output",
		Now:       fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	wsURL, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	wsURL.Scheme = "ws"
	wsURL.Path = "/v1/ws"
	query := wsURL.Query()
	query.Set("access_token", "token123")
	wsURL.RawQuery = query.Encode()

	_, response, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err == nil {
		t.Fatal("expected websocket dial to fail when token is passed in query")
	}
	if response == nil {
		t.Fatal("expected HTTP response for failed websocket handshake")
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status code for failed handshake: %d", response.StatusCode)
	}
}

func TestWebSocketRejectsMissingOriginWhenAllowlistConfigured(t *testing.T) {
	t.Parallel()

	srv, err := New(Options{
		Store: &fakeStore{},
		Validator: fakeValidator{
			validToken: "token123",
		},
		OutputDir:               "output",
		AllowedWebSocketOrigins: []string{"https://console.sift.local"},
		Now:                     fixedNow,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()

	wsURL, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	wsURL.Scheme = "ws"
	wsURL.Path = "/v1/ws"

	header := http.Header{}
	header.Set("Authorization", "Bearer token123")
	_, response, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	if err == nil {
		t.Fatal("expected websocket dial to fail without Origin header when allowlist is configured")
	}
	if response == nil {
		t.Fatal("expected HTTP response for failed websocket handshake")
	}
	if response.StatusCode < 400 {
		t.Fatalf("unexpected status code for failed handshake: %d", response.StatusCode)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
}
