package article

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Candidate struct {
	SourceID      string
	SourceURL     string
	Title         string
	PublishedAt   time.Time
	EditorialType string
	RightsMode    string
}

type Record struct {
	ArticleID     string
	SourceID      string
	SourceURL     string
	CanonicalURL  string
	Title         string
	PublishedAt   string
	FirstSeenAt   string
	EditorialType string
	RightsMode    string
}

func BuildRecord(candidate Candidate, firstSeenAt time.Time) (Record, error) {
	if candidate.SourceID == "" {
		return Record{}, fmt.Errorf("source_id is required")
	}
	if candidate.SourceURL == "" {
		return Record{}, fmt.Errorf("source_url is required")
	}
	if candidate.Title == "" {
		return Record{}, fmt.Errorf("title is required")
	}
	if candidate.EditorialType == "" {
		return Record{}, fmt.Errorf("editorial_type is required")
	}
	if candidate.RightsMode == "" {
		return Record{}, fmt.Errorf("rights_mode is required")
	}

	canonicalURL, err := CanonicalizeURL(candidate.SourceURL)
	if err != nil {
		return Record{}, fmt.Errorf("canonicalize source_url: %w", err)
	}

	publishedAt := candidate.PublishedAt
	if publishedAt.IsZero() {
		publishedAt = firstSeenAt
	}

	return Record{
		ArticleID:     articleID(candidate.SourceID, canonicalURL),
		SourceID:      candidate.SourceID,
		SourceURL:     candidate.SourceURL,
		CanonicalURL:  canonicalURL,
		Title:         strings.TrimSpace(candidate.Title),
		PublishedAt:   publishedAt.UTC().Format(time.RFC3339),
		FirstSeenAt:   firstSeenAt.UTC().Format(time.RFC3339),
		EditorialType: candidate.EditorialType,
		RightsMode:    candidate.RightsMode,
	}, nil
}

func CanonicalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("url must include scheme and host")
	}

	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""

	if u.Path == "" {
		u.Path = "/"
	} else if u.Path != "/" {
		u.Path = strings.TrimSuffix(u.Path, "/")
		if u.Path == "" {
			u.Path = "/"
		}
	}

	filteredQuery := make(url.Values)
	rawQuery := u.Query()
	for key, values := range rawQuery {
		if isTrackingParam(key) {
			continue
		}
		filteredQuery[key] = append([]string(nil), values...)
	}

	sortedKeys := make([]string, 0, len(filteredQuery))
	for key := range filteredQuery {
		sortedKeys = append(sortedKeys, key)
		sort.Strings(filteredQuery[key])
	}
	sort.Strings(sortedKeys)

	encoded := make([]string, 0)
	for _, key := range sortedKeys {
		escapedKey := url.QueryEscape(key)
		values := filteredQuery[key]
		if len(values) == 0 {
			encoded = append(encoded, escapedKey+"=")
			continue
		}
		for _, value := range values {
			encoded = append(encoded, escapedKey+"="+url.QueryEscape(value))
		}
	}
	u.RawQuery = strings.Join(encoded, "&")

	return u.String(), nil
}

func isTrackingParam(key string) bool {
	lower := strings.ToLower(key)
	return strings.HasPrefix(lower, "utm_") || lower == "fbclid" || lower == "gclid"
}

func articleID(sourceID, canonicalURL string) string {
	sum := sha1.Sum([]byte(sourceID + "|" + canonicalURL))
	return "art_" + hex.EncodeToString(sum[:10])
}
