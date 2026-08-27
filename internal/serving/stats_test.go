package serving

import (
	"path/filepath"
	"testing"
)

func TestStatsWithoutActiveBundle(t *testing.T) {
	registry, err := NewRegistry(filepath.Join(t.TempDir(), "registry"))
	if err != nil {
		t.Fatal(err)
	}
	stats, err := registry.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Active || stats.Repositories != 0 || stats.RecommendationEdges != 0 {
		t.Fatalf("unexpected empty stats: %#v", stats)
	}
}
