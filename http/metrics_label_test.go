package http

import "testing"

func TestNormalizeMetricDimension(t *testing.T) {
	if got := normalizeMetricDimension("model-123"); got != "MODEL-123" {
		t.Fatalf("normalizeMetricDimension(model-123) = %q", got)
	}
	if got := normalizeMetricDimension("model abc"); got != "MODEL ABC" {
		t.Fatalf("normalizeMetricDimension(model abc) = %q", got)
	}
	if got := normalizeMetricDimension("bad/value"); got != "others" {
		t.Fatalf("normalizeMetricDimension(bad/value) = %q", got)
	}
	if got := normalizeMetricDimension(""); got != "null" {
		t.Fatalf("normalizeMetricDimension(empty) = %q", got)
	}
}
