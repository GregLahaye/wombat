package tidy

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
)

func writeProjectSettings(t *testing.T, dir, settingsFile string, data map[string]any) {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0o755)
	if err := apply.WriteSettings(filepath.Join(claudeDir, settingsFile), data); err != nil {
		t.Fatal(err)
	}
}

func TestScan_FindsProjectPermissions(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")
	os.MkdirAll(scopePath, 0o755)

	// Create a project under root with permissions.
	writeProjectSettings(t, filepath.Join(root, "project-a"), "settings.local.json", map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read", "Write"},
		},
	})

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: scopePath, SettingsFile: "settings.local.json"},
			"global": {Path: filepath.Join(t.TempDir(), ".claude"), SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.Scanned != 1 {
		t.Errorf("expected 1 scanned, got %d", result.Scanned)
	}
	if len(result.Recommendations) != 2 {
		t.Fatalf("expected 2 recommendations, got %d", len(result.Recommendations))
	}

	rules := make(map[string]string)
	for _, rec := range result.Recommendations {
		rules[rec.Rule] = rec.TargetScope
	}
	if rules["Read"] != "work" {
		t.Errorf("expected Read -> work, got %q", rules["Read"])
	}
	if rules["Write"] != "work" {
		t.Errorf("expected Write -> work, got %q", rules["Write"])
	}
}

func TestScan_PromotesToGlobal(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()
	scopePath1 := filepath.Join(root1, ".claude")
	scopePath2 := filepath.Join(root2, ".claude")
	os.MkdirAll(scopePath1, 0o755)
	os.MkdirAll(scopePath2, 0o755)

	// Same rule in projects under both scopes.
	writeProjectSettings(t, filepath.Join(root1, "proj"), "settings.local.json", map[string]any{
		"permissions": map[string]any{"allow": []any{"Read"}},
	})
	writeProjectSettings(t, filepath.Join(root2, "proj"), "settings.local.json", map[string]any{
		"permissions": map[string]any{"allow": []any{"Read"}},
	})

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":     {Path: scopePath1, SettingsFile: "settings.local.json"},
			"personal": {Path: scopePath2, SettingsFile: "settings.local.json"},
			"global":   {Path: filepath.Join(t.TempDir(), ".claude"), SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Should be promoted to global since it's in all non-global scopes.
	if len(result.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation (global), got %d", len(result.Recommendations))
	}
	rec := result.Recommendations[0]
	if rec.TargetScope != "global" {
		t.Errorf("expected global target, got %q", rec.TargetScope)
	}
	if rec.Rule != "Read" {
		t.Errorf("expected Read rule, got %q", rec.Rule)
	}
}

func TestScan_NoRecommendations(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")
	os.MkdirAll(scopePath, 0o755)

	// Project with no permissions.
	writeProjectSettings(t, filepath.Join(root, "project"), "settings.local.json", map[string]any{
		"enabledPlugins": map[string]any{"foo": true},
	})

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: scopePath, SettingsFile: "settings.local.json"},
			"global": {Path: filepath.Join(t.TempDir(), ".claude"), SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(result.Recommendations) != 0 {
		t.Errorf("expected 0 recommendations, got %d", len(result.Recommendations))
	}
}

func TestApplyRecommendations(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "project")
	settingsPath := filepath.Join(projDir, ".claude", "settings.local.json")

	writeProjectSettings(t, projDir, "settings.local.json", map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read", "Write"},
		},
	})

	cfg := &config.Config{}
	cfg.EnsureMaps()

	recs := []Recommendation{
		{
			Rule:        "Read",
			Type:        "allow",
			FoundIn:     []string{settingsPath},
			TargetScope: "work",
		},
	}

	if err := ApplyRecommendations(cfg, recs); err != nil {
		t.Fatalf("ApplyRecommendations: %v", err)
	}

	// Config should have the rule.
	if len(cfg.Permissions.Allow) != 1 || cfg.Permissions.Allow[0].Rule != "Read" {
		t.Errorf("expected Read rule in config, got %v", cfg.Permissions.Allow)
	}
	if !slices.Contains(cfg.Permissions.Allow[0].Scopes, "work") {
		t.Errorf("expected work scope, got %v", cfg.Permissions.Allow[0].Scopes)
	}

	// Settings file should have Read removed but Write preserved.
	data, err := apply.ReadSettings(settingsPath)
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	perms, _ := data["permissions"].(map[string]any)
	allow := apply.ExtractStringSlice(perms, "allow")
	if len(allow) != 1 || allow[0] != "Write" {
		t.Errorf("expected [Write] in settings, got %v", allow)
	}
}

func TestScan_NoGlobalScope_NoPromotion(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()
	scopePath1 := filepath.Join(root1, ".claude")
	scopePath2 := filepath.Join(root2, ".claude")
	os.MkdirAll(scopePath1, 0o755)
	os.MkdirAll(scopePath2, 0o755)

	// Same rule in projects under both scopes.
	writeProjectSettings(t, filepath.Join(root1, "proj"), "settings.local.json", map[string]any{
		"permissions": map[string]any{"allow": []any{"Read"}},
	})
	writeProjectSettings(t, filepath.Join(root2, "proj"), "settings.local.json", map[string]any{
		"permissions": map[string]any{"allow": []any{"Read"}},
	})

	// No global scope — should NOT try to promote to global.
	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":     {Path: scopePath1, SettingsFile: "settings.local.json"},
			"personal": {Path: scopePath2, SettingsFile: "settings.local.json"},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Should have per-scope recommendations, NOT a global promotion.
	for _, rec := range result.Recommendations {
		if rec.TargetScope == "global" {
			t.Errorf("should not promote to global when no global scope exists, got: %+v", rec)
		}
	}
	if len(result.Recommendations) != 2 {
		t.Errorf("expected 2 per-scope recommendations, got %d", len(result.Recommendations))
	}
}

func TestApplyRecommendations_RemovesEmptyPermissions(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "project")
	settingsPath := filepath.Join(projDir, ".claude", "settings.local.json")

	writeProjectSettings(t, projDir, "settings.local.json", map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read"},
		},
	})

	cfg := &config.Config{}
	cfg.EnsureMaps()

	recs := []Recommendation{
		{Rule: "Read", Type: "allow", FoundIn: []string{settingsPath}, TargetScope: "work"},
	}

	if err := ApplyRecommendations(cfg, recs); err != nil {
		t.Fatal(err)
	}

	// Settings file should be deleted since it's now empty.
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("expected settings file to be removed when empty")
	}
}
