package dashboard

import (
	json "encoding/json/v2"
	"net/http"
	"reflect"
	"testing"
)

func TestQualityRank(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  int
	}{
		{name: "frontier opus", model: "claude-opus-4.7", want: qualityFrontier},
		{name: "frontier pro", model: "gemini-3-pro", want: qualityFrontier},
		{name: "fast flash", model: "gemini-3.1-flash-preview", want: qualityFast},
		{name: "fast mini wins over line", model: "gpt-5.4-mini", want: qualityFast},
		{name: "o3-mini is fast not frontier", model: "o3-mini", want: qualityFast},
		{name: "o3 is frontier", model: "o3", want: qualityFrontier},
		{name: "sonnet is balanced", model: "claude-sonnet-4.6", want: qualityBalanced},
		{name: "unknown is balanced", model: "some-random-model", want: qualityBalanced},
		{name: "free suffix ignored", model: "deepseek-v4.1-flash:free", want: qualityFast},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qualityRank(tt.model); got != tt.want {
				t.Errorf("qualityRank(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}

func TestOrderModels(t *testing.T) {
	tests := []struct {
		name       string
		members    []string
		priorities map[string]int
		want       []string
	}{
		{
			name: "free first then frontier before fast",
			members: []string{
				"gmni/gemini-3-flash",
				"gmni/gemini-3-pro",
				"bzl/auto:free",
				"or/claude-sonnet-4.6",
			},
			priorities: map[string]int{"gmni": 2, "bzl": 1, "or": 3},
			want: []string{
				"bzl/auto:free",
				"gmni/gemini-3-pro",
				"or/claude-sonnet-4.6",
				"gmni/gemini-3-flash",
			},
		},
		{
			name: "equal quality prefers the lower provider priority",
			members: []string{
				"aaa/claude-sonnet-4.6",
				"bbb/claude-sonnet-4.6",
			},
			priorities: map[string]int{"aaa": 5, "bbb": 1},
			want:       []string{"bbb/claude-sonnet-4.6", "aaa/claude-sonnet-4.6"},
		},
		{
			name: "provider without a priority sorts last",
			members: []string{
				"zzz/claude-sonnet-4.6",
				"aaa/claude-sonnet-4.6",
			},
			priorities: map[string]int{"aaa": 9},
			want:       []string{"aaa/claude-sonnet-4.6", "zzz/claude-sonnet-4.6"},
		},
		{
			name:       "ties break on the id for a stable order",
			members:    []string{"b/x-sonnet", "a/x-sonnet"},
			priorities: map[string]int{"a": 1, "b": 1},
			want:       []string{"a/x-sonnet", "b/x-sonnet"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := orderModels(tt.members, tt.priorities)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("orderModels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleAutoOptimalCombo(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	router := setupTestRouter(repo)

	seedConnection(t, repo.RawDB(), "bazaarlink")
	seedConnection(t, repo.RawDB(), "gemini")

	rec := postJSON(t, router, "/api/combos/auto-optimal", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("auto-optimal expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := res["id"]; got != AutoOptimalComboID {
		t.Errorf("id = %v, want %q", got, AutoOptimalComboID)
	}

	master, err := repo.GetComboById(AutoOptimalComboID)
	if err != nil {
		t.Fatalf("GetComboById: %v", err)
	}
	if master == nil {
		t.Fatal("expected the master auto-optimal combo row to exist")
	}
	if master.Kind == nil || *master.Kind != autoOptimalComboKind {
		t.Fatalf("kind = %v, want %q", master.Kind, autoOptimalComboKind)
	}
	members, err := comboModels(master.Models)
	if err != nil {
		t.Fatalf("comboModels: %v", err)
	}
	if len(members) == 0 {
		t.Fatal("master combo has no models")
	}
	// The free model must lead the chain: it is the cheapest way to answer.
	if members[0] != "bzl/auto:free" {
		t.Errorf("first model = %q, want the free bzl/auto:free", members[0])
	}
	// Family combos are part of the same one-click build.
	families, _ := res["families"].([]any)
	if len(families) == 0 {
		t.Fatal("expected at least one family combo from the optimal build")
	}

	// Regenerating upserts: one master row, unchanged member count.
	rec = postJSON(t, router, "/api/combos/auto-optimal", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("second auto-optimal expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	all, err := repo.GetCombos()
	if err != nil {
		t.Fatalf("GetCombos: %v", err)
	}
	masters := 0
	for _, c := range all {
		if c.ID == AutoOptimalComboID {
			masters++
		}
	}
	if masters != 1 {
		t.Errorf("master combo rows = %d, want 1", masters)
	}
}

func TestHandleAutoOptimalComboEmptyWithoutConnections(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	router := setupTestRouter(repo)

	rec := postJSON(t, router, "/api/combos/auto-optimal", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("no connections expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}
