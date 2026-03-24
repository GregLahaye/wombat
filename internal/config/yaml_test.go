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
