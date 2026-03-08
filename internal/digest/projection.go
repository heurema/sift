package digest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sift/internal/event"
)

type Envelope struct {
	Scope        string   `json:"scope"`
	Window       string   `json:"window"`
	GeneratedAt  string   `json:"generated_at"`
	EventIDs     []string `json:"event_ids"`
	MarkdownPath string   `json:"markdown_path"`
}

type Projection struct {
	Envelope     Envelope
	Markdown     string
	JSONPayload  []byte
	JSONPath     string
	MarkdownPath string
}

var defaultWindows = []string{"24h", "7d"}

var (
	ErrWindowValueRequired = errors.New("window value is required")
	ErrInvalidWindowValue  = errors.New("invalid window value")
)

func PublishDefault(records []event.Record, outputDir string, generatedAt time.Time) error {
	for _, window := range defaultWindows {
		projection, err := BuildProjection(records, outputDir, "crypto", window, generatedAt)
		if err != nil {
			return fmt.Errorf("build digest projection for window %s: %w", window, err)
		}
		if err := PublishProjection(projection); err != nil {
			return fmt.Errorf("publish digest projection for window %s: %w", window, err)
		}
	}

	return nil
}

func BuildProjection(records []event.Record, outputDir, scope, window string, generatedAt time.Time) (Projection, error) {
	windowDuration, err := parseWindowDuration(window)
	if err != nil {
		return Projection{}, err
	}

	since := generatedAt.Add(-windowDuration)
	selected, err := selectEvents(records, scope, since)
	if err != nil {
		return Projection{}, err
	}

	eventIDs := make([]string, 0, len(selected))
	for _, rec := range selected {
		eventIDs = append(eventIDs, rec.EventID)
	}

	digestDir := filepath.Join(outputDir, "digests", scope)
	markdownPath := filepath.Join(digestDir, window+".md")
	jsonPath := filepath.Join(digestDir, window+".json")

	envelope := Envelope{
		Scope:        scope,
		Window:       window,
		GeneratedAt:  generatedAt.Format(time.RFC3339),
		EventIDs:     eventIDs,
		MarkdownPath: filepath.ToSlash(markdownPath),
	}

	jsonPayload, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return Projection{}, fmt.Errorf("marshal digest json: %w", err)
	}
	jsonPayload = append(jsonPayload, '\n')

	return Projection{
		Envelope:     envelope,
		Markdown:     renderMarkdown(envelope, selected),
		JSONPayload:  jsonPayload,
		JSONPath:     jsonPath,
		MarkdownPath: markdownPath,
	}, nil
}

func PublishProjection(projection Projection) error {
	if err := writeFileAtomic(projection.JSONPath, projection.JSONPayload, 0o644); err != nil {
		return fmt.Errorf("write digest json projection: %w", err)
	}
	if err := writeFileAtomic(projection.MarkdownPath, []byte(projection.Markdown), 0o644); err != nil {
		return fmt.Errorf("write digest markdown projection: %w", err)
	}
	return nil
}

func parseWindowDuration(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return 0, fmt.Errorf("%w", ErrWindowValueRequired)
	}

	if strings.HasSuffix(trimmed, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(trimmed, "d"))
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("%w: %q, expected relative window like 24h, 7d, or 30d", ErrInvalidWindowValue, raw)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	if strings.HasSuffix(trimmed, "h") {
		window, err := time.ParseDuration(trimmed)
		if err != nil || window <= 0 {
			return 0, fmt.Errorf("%w: %q, expected relative window like 24h, 7d, or 30d", ErrInvalidWindowValue, raw)
		}
		return window, nil
	}

	return 0, fmt.Errorf("%w: %q, expected relative window like 24h, 7d, or 30d", ErrInvalidWindowValue, raw)
}

func selectEvents(records []event.Record, scope string, since time.Time) ([]event.Record, error) {
	filtered := make([]event.Record, 0, len(records))
	for _, rec := range records {
		publishedAt, err := time.Parse(time.RFC3339, rec.PublishedAt)
		if err != nil {
			return nil, fmt.Errorf("parse event %s published_at %q: %w", rec.EventID, rec.PublishedAt, err)
		}

		if publishedAt.Before(since) {
			continue
		}

		if !eventMatchesScope(rec, scope) {
			continue
		}

		filtered = append(filtered, rec)
	}

	return filtered, nil
}

func eventMatchesScope(rec event.Record, scope string) bool {
	target := strings.ToLower(strings.TrimSpace(scope))
	if target == "" {
		return false
	}

	if target == "crypto" && strings.EqualFold(rec.Category, "crypto") {
		return true
	}

	for _, asset := range rec.Assets {
		if strings.EqualFold(asset, target) {
			return true
		}
	}

	for _, topic := range rec.Topics {
		if strings.EqualFold(topic, target) {
			return true
		}
	}

	if strings.EqualFold(rec.EventType, target) {
		return true
	}

	return false
}

func renderMarkdown(envelope Envelope, records []event.Record) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# Digest: %s (%s)\n\n", envelope.Scope, envelope.Window))
	builder.WriteString(fmt.Sprintf("Generated at: `%s`\n\n", envelope.GeneratedAt))
	builder.WriteString(fmt.Sprintf("Event count: `%d`\n\n", len(envelope.EventIDs)))

	if len(records) == 0 {
		builder.WriteString("No events found for this scope and window.\n")
		return builder.String()
	}

	for _, rec := range records {
		builder.WriteString(fmt.Sprintf("- `%s` %s\n", rec.EventID, rec.Title))
		builder.WriteString(fmt.Sprintf("  published: `%s`, importance: `%.2f`, confidence: `%.2f`\n", rec.PublishedAt, rec.ImportanceScore, rec.ConfidenceScore))
	}

	return builder.String()
}

func writeFileAtomic(path string, payload []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	closeWithError := func(stage string, stageErr error) error {
		if closeErr := tmpFile.Close(); closeErr != nil {
			return fmt.Errorf("%s: %w (close temp file: %v)", stage, stageErr, closeErr)
		}
		return fmt.Errorf("%s: %w", stage, stageErr)
	}

	if _, err := tmpFile.Write(payload); err != nil {
		return closeWithError("write temp file", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return closeWithError("sync temp file", err)
	}

	if err := tmpFile.Chmod(mode); err != nil {
		return closeWithError("chmod temp file", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
