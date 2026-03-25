package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Adversarial tests: weird inputs, edge cases, boundary conditions.

func TestValidate_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	cfg.EnsureMaps()
	// Completely empty config should be valid.
	if err := cfg.Validate(); err != nil {
		t.Errorf("empty config should be valid: %v", err)
	}
}

func TestValidate_SkillWithEmptySource(t *testing.T) {
	cfg := &Config{
		Skills: map[string]Item{"skill": {Source: ""}},
	}
	cfg.EnsureMaps()
	err := cfg.Validate()
	if err == nil {
		t.Error("skill with empty source should fail validation")
	}
}

func TestValidate_OverrideWithUnknownScope(t *testing.T) {
	cfg := &Config{
		Scopes:    map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Overrides: map[string]ScopeSet{"item": {Enabled: []string{"nonexistent"}}},
	}
	cfg.EnsureMaps()
	err := cfg.Validate()
	if err == nil {
		t.Error("override with unknown scope should fail")
	}
}

func TestScopeNames_Empty(t *testing.T) {
	cfg := &Config{}
	cfg.EnsureMaps()
	names := cfg.ScopeNames()
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestScopeNames_OnlyGlobal(t *testing.T) {
	cfg := &Config{Scopes: map[string]Scope{"global": {Path: "/g"}}}
	cfg.EnsureMaps()
	names := cfg.ScopeNames()
	if len(names) != 1 || names[0] != "global" {
		t.Errorf("expected [global], got %v", names)
	}
}

func TestScopeRefs_Empty(t *testing.T) {
	cfg := &Config{}
	cfg.EnsureMaps()
	refs := cfg.ScopeRefs("nonexistent")
	if len(refs) != 0 {
		t.Errorf("expected empty refs, got %v", refs)
	}
}

func TestClone_WithPermissions(t *testing.T) {
	cfg := &Config{
		Permissions: Permissions{
			Allow: []PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
			Deny:  []PermissionRule{{Rule: "Write", Scopes: []string{"global"}}},
		},
	}
	cfg.EnsureMaps()
	clone := cfg.Clone()

	// Mutate original — clone should be unaffected.
	cfg.Permissions.Allow[0].Scopes = append(cfg.Permissions.Allow[0].Scopes, "personal")
	if len(clone.Permissions.Allow[0].Scopes) != 1 {
		t.Error("clone permissions were mutated")
	}
}

func TestExpandPath_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		tilde bool // whether result should differ from input
	}{
		{"~/work", true},
		{"~", true},
		{"/absolute/path", false},
		{"relative/path", false},
		{"", false},
		{"~other-user/path", false}, // should NOT expand ~other-user
	}
	for _, tt := range tests {
		result := ExpandPath(tt.input)
		if tt.tilde && result == tt.input {
			t.Errorf("ExpandPath(%q) should expand tilde", tt.input)
		}
		if !tt.tilde && result != tt.input {
			t.Errorf("ExpandPath(%q) = %q, want unchanged", tt.input, result)
		}
	}
}

func TestContractPath_EdgeCases(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home")
	}
	tests := []struct {
		input, want string
	}{
		{home, "~"},
		{home + "/work", "~/work"},
		{"/other/path", "/other/path"},
		{home + "/../" + filepath.Base(home) + "/work", "~/work"}, // Clean resolves ..
	}
	for _, tt := range tests {
		got := ContractPath(tt.input)
		if got != tt.want {
			t.Errorf("ContractPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLoadSave_PreservesItemEnabledNilVsEmpty(t *testing.T) {
	// This is the most critical round-trip invariant in the codebase.
	// nil Enabled = "inherit from default_scope", empty = "explicitly disabled".
	// A lossy round-trip silently changes item behavior.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		Scopes:  map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Sources: map[string]Source{"src": {Git: "https://example.com"}},
		Skills: map[string]Item{
			"inherited": {Source: "src"},                             // nil Enabled
			"disabled":  {Source: "src", Enabled: []string{}},       // empty Enabled
			"enabled":   {Source: "src", Enabled: []string{"work"}}, // non-empty Enabled
		},
	}
	cfg.EnsureMaps()

	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	inherited := loaded.Skills["inherited"]
	if inherited.Enabled != nil {
		t.Errorf("inherited: expected Enabled=nil (inherit), got %v", inherited.Enabled)
	}

	disabled := loaded.Skills["disabled"]
	if disabled.Enabled == nil {
		t.Error("disabled: expected Enabled=[] (disabled), got nil (would inherit)")
	} else if len(disabled.Enabled) != 0 {
		t.Errorf("disabled: expected empty Enabled, got %v", disabled.Enabled)
	}

	enabled := loaded.Skills["enabled"]
	if len(enabled.Enabled) != 1 || enabled.Enabled[0] != "work" {
		t.Errorf("enabled: expected [work], got %v", enabled.Enabled)
	}
}

func TestLoadSave_PreservesEmptySlices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		Scopes:  map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Sources: map[string]Source{"src": {Git: "https://example.com", DefaultScope: []string{}}},
	}
	cfg.EnsureMaps()

	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// YAML with empty slice: default_scope: [] should round-trip.
	// After load, DefaultScope might be nil (YAML unmarshals empty as nil).
	// This is acceptable — both nil and [] mean "no default scope".
	src := loaded.Sources["src"]
	if len(src.DefaultScope) != 0 {
		t.Errorf("expected empty DefaultScope, got %v", src.DefaultScope)
	}
}
