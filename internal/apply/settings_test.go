package apply

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
)

func TestReadSettings_Missing(t *testing.T) {
	data, err := ReadSettings("/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty map, got %v", data)
	}
}

func TestReadSettings_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte(""), 0o644)

	data, err := ReadSettings(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty map, got %v", data)
	}
}

func TestReadWriteSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	original := map[string]any{
		"enabledPlugins": map[string]any{"plugin1": true},
	}
	if err := WriteSettings(path, original); err != nil {
		t.Fatalf("WriteSettings: %v", err)
	}

	data, err := ReadSettings(path)
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	plugins, ok := data["enabledPlugins"].(map[string]any)
	if !ok {
		t.Fatal("missing enabledPlugins")
	}
	if v, _ := plugins["plugin1"].(bool); !v {
		t.Error("expected plugin1=true")
	}
}

func TestExtractStringSlice(t *testing.T) {
	m := map[string]any{
		"allow": []any{"Read", "Write", 42}, // 42 should be skipped
	}
	result := ExtractStringSlice(m, "allow")
	if len(result) != 2 || result[0] != "Read" || result[1] != "Write" {
		t.Errorf("expected [Read, Write], got %v", result)
	}

	result = ExtractStringSlice(m, "deny")
	if result != nil {
		t.Errorf("expected nil for missing key, got %v", result)
	}
}

func TestMergePlugins(t *testing.T) {
	data := map[string]any{}
	cfg := &config.Config{
		Plugins: map[string]config.ScopeSet{
			"plugin1": {Enabled: []string{"work"}},
		},
	}
	cfg.EnsureMaps()

	changed := mergePlugins(data, cfg, nil, "work")
	if !changed {
		t.Error("expected change")
	}

	plugins := data["enabledPlugins"].(map[string]any)
	if v, _ := plugins["plugin1"].(bool); !v {
		t.Error("expected plugin1=true in work scope")
	}

	// Not in personal scope.
	data2 := map[string]any{}
	changed2 := mergePlugins(data2, cfg, nil, "personal")
	if changed2 {
		t.Error("expected no change for personal scope")
	}
}

func TestMergePermissions_AddRule(t *testing.T) {
	data := map[string]any{}
	cfg := &config.Config{
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{
				{Rule: "Read", Scopes: []string{"work"}},
			},
		},
	}
	cfg.EnsureMaps()

	changed := mergePermissions(data, cfg, nil, "work")
	if !changed {
		t.Error("expected change")
	}

	perms := data["permissions"].(map[string]any)
	allow := perms["allow"].([]any)
	if len(allow) != 1 || allow[0] != "Read" {
		t.Errorf("expected [Read], got %v", allow)
	}
}

func TestMergePermissions_RemoveOwnedRule(t *testing.T) {
	data := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read", "Write"},
		},
	}
	cfg := &config.Config{
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{
				{Rule: "Read", Scopes: []string{"work"}},
				// Write is owned but not in work scope anymore.
			},
		},
	}
	cfg.EnsureMaps()

	prevCfg := &config.Config{
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{
				{Rule: "Read", Scopes: []string{"work"}},
				{Rule: "Write", Scopes: []string{"work"}},
			},
		},
	}
	prevCfg.EnsureMaps()

	changed := mergePermissions(data, cfg, prevCfg, "work")
	if !changed {
		t.Error("expected change")
	}

	perms := data["permissions"].(map[string]any)
	allow := perms["allow"].([]any)
	if len(allow) != 1 || allow[0] != "Read" {
		t.Errorf("expected [Read], got %v", allow)
	}
}

func TestMergePermissions_PreservesUnowned(t *testing.T) {
	data := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"UserRule", "Read"},
		},
	}
	cfg := &config.Config{
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{
				{Rule: "Read", Scopes: []string{"work"}},
			},
		},
	}
	cfg.EnsureMaps()

	changed := mergePermissions(data, cfg, nil, "work")
	// "UserRule" is unowned, should be preserved. "Read" already exists.
	if changed {
		t.Error("expected no change since Read already exists")
	}
}
