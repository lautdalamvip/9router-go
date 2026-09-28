package dashboard

import (
	"testing"
)

func TestModelFamily(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "claude sonnet versions collapse",
			input: "claude-sonnet-4.6",
			want:  "claude-sonnet",
		},
		{
			name:  "claude opus single-digit version",
			input: "claude-opus-4.7",
			want:  "claude-opus",
		},
		{
			name:  "gpt mini variants keep size tier",
			input: "gpt-5.4-mini",
			want:  "gpt-mini",
		},
		{
			name:  "gpt codex across generations",
			input: "gpt-5.2-codex",
			want:  "gpt-codex",
		},
		{
			name:  "gemini preview flag dropped",
			input: "gemini-3.1-flash-preview",
			want:  "gemini-flash",
		},
		{
			name:  "gemini generations meet",
			input: "gemini-3-flash-preview",
			want:  "gemini-flash",
		},
		{
			name:  "deepseek flash keeps tier",
			input: "deepseek-v4.1-flash",
			want:  "deepseek-flash",
		},
		{
			name:  "kimi v-prefixed version",
			input: "kimi-k2.6",
			want:  "kimi",
		},
		{
			name:  "free marker dropped",
			input: "deepseek-v4.1-flash:free",
			want:  "deepseek-flash",
		},
		{
			name:  "grok plain version",
			input: "grok-4.3",
			want:  "grok",
		},
		{
			name:  "registry model with owner path",
			input: "nvidia/nemotron-3-super-120b-a12b:free",
			want:  "nvidia/nemotron-super-120b-a12b",
		},
		{
			name:  "kimi mid-name generation",
			input: "kimi-k2.6",
			want:  "kimi",
		},
		{
			name:  "kimi generation suffix",
			input: "kimi-k2.7-code",
			want:  "kimi-code",
		},
		{
			name:  "minimax generations merge",
			input: "minimax-m3",
			want:  "minimax",
		},
		{
			name:  "openai o-series line kept",
			input: "o1",
			want:  "o1",
		},
		{
			name:  "openai o-series mini kept",
			input: "o3-mini",
			want:  "o3-mini",
		},
		{
			name:  "plain version only falls back to original",
			input: "4.6",
			want:  "4.6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modelFamily(tt.input); got != tt.want {
				t.Errorf("modelFamily(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestModelFamily_MergesAcrossProvidersAndVersions(t *testing.T) {
	models := []string{
		"cc/claude-sonnet-4.6",
		"claude/claude-sonnet-4.5",
		"cc/claude-opus-4.7",
	}
	families := make(map[string][]string)
	for _, m := range models {
		_, id := splitAliasModel(m)
		f := modelFamily(id)
		families[f] = appendUnique(families[f], m)
	}

	if len(families) != 2 {
		t.Fatalf("expected 2 families, got %d: %v", len(families), families)
	}
	if got := len(families["claude-sonnet"]); got != 2 {
		t.Errorf("claude-sonnet family = %d members, want 2: %v", got, families["claude-sonnet"])
	}
}

func TestAutoFamilyComboID_StableAndSafe(t *testing.T) {
	if got, want := autoFamilyComboID("claude-sonnet"), "auto-family-claude-sonnet"; got != want {
		t.Errorf("autoFamilyComboID(claude-sonnet) = %q, want %q", got, want)
	}
	// Same family must always map to the same id so regeneration upserts.
	if autoFamilyComboID("gpt-mini") != autoFamilyComboID("gpt-mini") {
		t.Error("autoFamilyComboID is not stable")
	}
}

func TestIsEmbeddingModel(t *testing.T) {
	cases := map[string]bool{
		"text-embedding-3-small": true,
		"gemini-embedding-001":   true,
		"claude-sonnet-4.6":      false,
	}
	for model, want := range cases {
		if got := isEmbeddingModel(model); got != want {
			t.Errorf("isEmbeddingModel(%q) = %v, want %v", model, got, want)
		}
	}
}
