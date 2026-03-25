package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLOmitEmpty_NoPermissions(t *testing.T) {
	cfg := &Config{
		Scopes: map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
	}
	cfg.EnsureMaps()
	b, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "sources:") {
		t.Error("empty sources should be omitted")
	}
	if strings.Contains(s, "plugins:") {
		t.Error("empty plugins should be omitted")
	}
	if strings.Contains(s, "skills:") {
		t.Error("empty skills should be omitted")
	}
	if strings.Contains(s, "agents:") {
		t.Error("empty agents should be omitted")
	}
	if strings.Contains(s, "overrides:") {
		t.Error("empty overrides should be omitted")
	}
}

func TestYAMLOmitEmpty_PermissionsOnlyAllow(t *testing.T) {
	cfg := &Config{
		Scopes: map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Permissions: Permissions{
			Allow: []PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()
	b, _ := yaml.Marshal(cfg)
	s := string(b)

	if !strings.Contains(s, "allow:") {
		t.Error("allow should be present")
	}
	if strings.Contains(s, "deny:") {
		t.Error("empty deny should be omitted")
	}
}

func TestYAML_ItemEnabledNilVsEmpty(t *testing.T) {
	// nil Enabled means "inherit from default_scope" — field must be omitted.
	// empty Enabled means "explicitly disabled" — field must be present as [].
	// This distinction is load-bearing; mixing them up silently changes behavior.

	nilItem := Item{Source: "src", Enabled: nil}
	emptyItem := Item{Source: "src", Enabled: []string{}}
	scopedItem := Item{Source: "src", Enabled: []string{"work"}}

	b1, _ := yaml.Marshal(nilItem)
	b2, _ := yaml.Marshal(emptyItem)
	b3, _ := yaml.Marshal(scopedItem)

	if strings.Contains(string(b1), "enabled") {
		t.Errorf("nil Enabled should omit field, got: %s", b1)
	}
	if !strings.Contains(string(b2), "enabled: []") {
		t.Errorf("empty Enabled should write 'enabled: []', got: %s", b2)
	}
	if !strings.Contains(string(b3), "enabled:") {
		t.Errorf("scoped Enabled should be present, got: %s", b3)
	}

	// Round-trip: nil stays nil, empty stays empty.
	var r1, r2, r3 Item
	yaml.Unmarshal(b1, &r1)
	yaml.Unmarshal(b2, &r2)
	yaml.Unmarshal(b3, &r3)

	if r1.Enabled != nil {
		t.Errorf("nil Enabled round-trip: got %v, want nil", r1.Enabled)
	}
	if r2.Enabled == nil || len(r2.Enabled) != 0 {
		t.Errorf("empty Enabled round-trip: got %v, want []", r2.Enabled)
	}
	if len(r3.Enabled) != 1 || r3.Enabled[0] != "work" {
		t.Errorf("scoped Enabled round-trip: got %v, want [work]", r3.Enabled)
	}
}

func TestYAML_ScopeSetEnabledNilVsEmpty(t *testing.T) {
	nilSet := ScopeSet{Enabled: nil}
	emptySet := ScopeSet{Enabled: []string{}}

	b1, _ := yaml.Marshal(nilSet)
	b2, _ := yaml.Marshal(emptySet)

	if strings.Contains(string(b1), "enabled") {
		t.Errorf("nil Enabled should omit field, got: %s", b1)
	}
	if !strings.Contains(string(b2), "enabled: []") {
		t.Errorf("empty Enabled should write 'enabled: []', got: %s", b2)
	}

	var r1, r2 ScopeSet
	yaml.Unmarshal(b1, &r1)
	yaml.Unmarshal(b2, &r2)

	if r1.Enabled != nil {
		t.Errorf("nil round-trip: got %v, want nil", r1.Enabled)
	}
	if r2.Enabled == nil {
		t.Errorf("empty round-trip: got nil, want []")
	}
}

func TestYAMLOmitEmpty_SourcesPresent(t *testing.T) {
	cfg := &Config{
		Scopes:  map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Sources: map[string]Source{"src": {Git: "https://example.com"}},
	}
	cfg.EnsureMaps()
	b, _ := yaml.Marshal(cfg)
	s := string(b)

	if !strings.Contains(s, "sources:") {
		t.Error("non-empty sources should be present")
	}
	if !strings.Contains(s, "https://example.com") {
		t.Error("source git URL should be present")
	}
}
