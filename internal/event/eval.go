package event

import (
	"fmt"
	"strings"
)

const (
	MergeSimilarityThreshold         = 0.82
	HighSimilarityEventTypeThreshold = 0.90
)

type EvalLabel string

const (
	EvalLabelSameEvent      EvalLabel = "same_event"
	EvalLabelDifferentEvent EvalLabel = "different_event"
)

type EvalPair struct {
	ID     string
	TitleA string
	TitleB string
	Label  EvalLabel
}

type EvalMetrics struct {
	PairsTotal     int
	SameEventPairs int
	DifferentPairs int
	TruePositives  int
	FalsePositives int
	TrueNegatives  int
	FalseNegatives int
	Precision      float64
	Threshold      float64
}

func SimilarityForTitles(titleA, titleB string) float64 {
	return titleSimilarity(
		normalizeForSimilarity(titleA),
		normalizeForSimilarity(titleB),
	)
}

func EvaluatePairs(pairs []EvalPair, threshold float64) (EvalMetrics, error) {
	metrics := EvalMetrics{
		PairsTotal: len(pairs),
		Threshold:  threshold,
	}

	for i, pair := range pairs {
		label := EvalLabel(strings.ToLower(strings.TrimSpace(string(pair.Label))))
		if label != EvalLabelSameEvent && label != EvalLabelDifferentEvent {
			return EvalMetrics{}, fmt.Errorf("pair %d has unsupported label %q", i, pair.Label)
		}

		similarity := SimilarityForTitles(pair.TitleA, pair.TitleB)
		predictSame := similarity >= threshold

		switch label {
		case EvalLabelSameEvent:
			metrics.SameEventPairs++
			if predictSame {
				metrics.TruePositives++
			} else {
				metrics.FalseNegatives++
			}
		case EvalLabelDifferentEvent:
			metrics.DifferentPairs++
			if predictSame {
				metrics.FalsePositives++
			} else {
				metrics.TrueNegatives++
			}
		}
	}

	denominator := metrics.TruePositives + metrics.FalsePositives
	if denominator > 0 {
		metrics.Precision = float64(metrics.TruePositives) / float64(denominator)
	}

	return metrics, nil
}
