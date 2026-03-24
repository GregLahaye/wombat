package apply

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
)

func TestApply_NoSources(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WOMBAT_HOME", dir)

	scopePath := filepath.Join(dir, "scope")
	os.MkdirAll(scopePath, 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	r, err := Apply(cfg, nil)
	if err != nil {
		t.Fatalf("Apply with no sources should succeed: %v", err)
	}
	if len(r.Errors) != 0 {
		t.Errorf("unexpected errors: %v", r.Errors)
	}
}

func TestApply_NoScopes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WOMBAT_HOME", dir)

	cfg := &config.Config{}
	cfg.EnsureMaps()

	r, err := Apply(cfg, nil)
	if err != nil {
		t.Fatalf("Apply with no scopes should succeed: %v", err)
	}
	if len(r.Errors) != 0 {
		t.Errorf("unexpected errors: %v", r.Errors)
	}
}

func TestApply_MissingSourceDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WOMBAT_HOME", dir)

	scopePath := filepath.Join(dir, "scope")
	os.MkdirAll(scopePath, 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
		Sources: map[string]config.Source{
			"missing": {Git: "https://example.com", DefaultScope: []string{"work"}},
		},
	}
	cfg.EnsureMaps()

	// Apply should succeed even if source dir doesn't exist — discovery just finds nothing.
	r, err := Apply(cfg, nil)
	if err != nil {
		t.Fatalf("Apply with missing source should succeed: %v", err)
	}
	if len(r.Errors) != 0 {
		t.Errorf("unexpected errors: %v", r.Errors)
	}
}

func TestSyncSettings_CreatesDirectoryIfNeeded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WOMBAT_HOME", dir)

	// Scope path doesn't exist yet.
	scopePath := filepath.Join(dir, "nonexistent", "scope")

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	r := &Result{}
	err := syncSettings(cfg, nil, r)
	if err != nil {
		t.Fatalf("syncSettings should create dirs: %v", err)
	}

	// Settings file should exist.
	settingsPath := filepath.Join(scopePath, "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Errorf("expected settings file at %s", settingsPath)
	}
}

func TestReadSettings_NullJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte("null"), 0o644)

	data, err := ReadSettings(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data == nil {
		t.Fatal("ReadSettings should never return nil map")
	}
	// Should be safe to write to.
	data["test"] = true
}

func TestMergePlugins_DisabledPlugin(t *testing.T) {
	// Plugin explicitly set to false in settings — wombat shouldn't touch it
	// if it's not in wombat config.
	data := map[string]any{
		"enabledPlugins": map[string]any{"user-plugin": false},
	}
	cfg := &config.Config{}
	cfg.EnsureMaps()

	changed := mergePlugins(data, cfg, nil, "work")
	if changed {
		t.Error("should not touch plugins not in wombat config")
	}

	plugins := data["enabledPlugins"].(map[string]any)
	if v, _ := plugins["user-plugin"].(bool); v {
		t.Error("user plugin should remain false")
	}
}

func TestMergePermissions_MixedTypes(t *testing.T) {
	// Settings file has a non-string entry in permissions (edge case).
	data := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read", 42, "Write"}, // 42 is not a string
		},
	}
	cfg := &config.Config{
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{{Rule: "Bash", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	changed := mergePermissions(data, cfg, nil, "work")
	if !changed {
		t.Error("expected change (adding Bash)")
	}

	// The non-string entry should be preserved as-is, Bash should be added.
	perms := data["permissions"].(map[string]any)
	allow := perms["allow"].([]any)

	hasRead := false
	hasBash := false
	for _, v := range allow {
		if s, ok := v.(string); ok {
			if s == "Read" {
				hasRead = true
			}
			if s == "Bash" {
				hasBash = true
			}
		}
	}
	if !hasRead || !hasBash {
		t.Errorf("expected Read and Bash in allow, got %v", allow)
	}
}
