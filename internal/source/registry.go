package source

import (
	"encoding/json"
	"fmt"
	"os"
)

type Registry struct {
	Version   int      `json:"version"`
	Category  string   `json:"category"`
	UpdatedAt string   `json:"updated_at"`
	Sources   []Source `json:"sources"`
}

type Source struct {
	SourceID             string  `json:"source_id"`
	SourceName           string  `json:"source_name"`
	SourceClass          string  `json:"source_class"`
	AccessMethod         string  `json:"access_method"`
	URL                  string  `json:"url"`
	SourceWeight         float64 `json:"source_weight"`
	RightsMode           string  `json:"rights_mode"`
	ExcerptAllowed       bool    `json:"excerpt_allowed"`
	SummaryAllowed       bool    `json:"summary_allowed"`
	DefaultEditorialType string  `json:"default_editorial_type"`
	ReviewedAt           string  `json:"reviewed_at"`
	Notes                string  `json:"notes"`
}

func LoadSeed(path string) (Registry, error) {
	file, err := os.Open(path)
	if err != nil {
		return Registry{}, fmt.Errorf("open source registry: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	var registry Registry
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, fmt.Errorf("decode source registry: %w", err)
	}

	if err := registry.Validate(); err != nil {
		return Registry{}, err
	}

	return registry, nil
}

func (r Registry) Validate() error {
	if r.Version <= 0 {
		return fmt.Errorf("source registry version must be positive")
	}
	if r.Category == "" {
		return fmt.Errorf("source registry category is required")
	}
	if r.UpdatedAt == "" {
		return fmt.Errorf("source registry updated_at is required")
	}
	if len(r.Sources) == 0 {
		return fmt.Errorf("source registry must contain at least one source")
	}

	seenIDs := make(map[string]struct{}, len(r.Sources))
	for _, src := range r.Sources {
		if src.SourceID == "" {
			return fmt.Errorf("source_id is required for every source")
		}
		if _, exists := seenIDs[src.SourceID]; exists {
			return fmt.Errorf("duplicate source_id: %s", src.SourceID)
		}
		seenIDs[src.SourceID] = struct{}{}

		if src.SourceName == "" {
			return fmt.Errorf("source_name is required for %s", src.SourceID)
		}
		if src.SourceClass == "" {
			return fmt.Errorf("source_class is required for %s", src.SourceID)
		}
		if src.AccessMethod == "" {
			return fmt.Errorf("access_method is required for %s", src.SourceID)
		}
		if src.URL == "" {
			return fmt.Errorf("url is required for %s", src.SourceID)
		}
		if src.RightsMode == "" {
			return fmt.Errorf("rights_mode is required for %s", src.SourceID)
		}
		if src.DefaultEditorialType == "" {
			return fmt.Errorf("default_editorial_type is required for %s", src.SourceID)
		}
		if src.ReviewedAt == "" {
			return fmt.Errorf("reviewed_at is required for %s", src.SourceID)
		}
	}

	return nil
}
