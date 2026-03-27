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

func TestScan_SkipsWombatManagedRules(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")
	os.MkdirAll(scopePath, 0o755)

	writeProjectSettings(t, filepath.Join(root, "project-a"), "settings.local.json", map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read", "CustomRule"},
		},
	})

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: scopePath, SettingsFile: "settings.local.json"},
			"global": {Path: filepath.Join(t.TempDir(), ".claude"), SettingsFile: "settings.json"},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	for _, rec := range result.Recommendations {
		if rec.Rule == "Read" {
			t.Errorf("should not recommend 'Read' — already managed by wombat")
		}
	}
	if len(result.Recommendations) != 1 {
		t.Errorf("expected 1 recommendation (CustomRule only), got %d", len(result.Recommendations))
	}
}

func TestScan_SkipsManagedRulesInGlobalPromotion(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()
	scopePath1 := filepath.Join(root1, ".claude")
	scopePath2 := filepath.Join(root2, ".claude")
	os.MkdirAll(scopePath1, 0o755)
	os.MkdirAll(scopePath2, 0o755)

	// "Read" is in projects under BOTH scopes — normally promoted to global.
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
		Permissions: config.Permissions{
			// "Read" is already managed by wombat for both scopes.
			Allow: []config.PermissionRule{{Rule: "Read", Scopes: []string{"work", "personal"}}},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// "Read" should NOT be recommended for global promotion — already managed.
	for _, rec := range result.Recommendations {
		if rec.Rule == "Read" {
			t.Errorf("should not recommend 'Read' for %s — already managed by wombat", rec.TargetScope)
		}
	}
	if len(result.Recommendations) != 0 {
		t.Errorf("expected 0 recommendations, got %d: %v", len(result.Recommendations), result.Recommendations)
	}
}

func TestScan_FindsScopePermissions(t *testing.T) {
	dir := t.TempDir()

	scopePath := filepath.Join(dir, "work", ".claude")
	os.MkdirAll(scopePath, 0o755)

	// Write permissions directly to scope settings file.
	if err := apply.WriteSettings(filepath.Join(scopePath, "settings.json"), map[string]any{
		"permissions": map[string]any{
			"allow": []any{"mcp__playwright__browser_type", "Read"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(result.Recommendations) != 2 {
		t.Fatalf("expected 2 recommendations, got %d: %v", len(result.Recommendations), result.Recommendations)
	}
	for _, rec := range result.Recommendations {
		if rec.TargetScope != "work" {
			t.Errorf("expected target scope 'work', got %q", rec.TargetScope)
		}
		if rec.Type != "allow" {
			t.Errorf("expected type 'allow', got %q", rec.Type)
		}
	}
}

func TestScan_SkipsManagedRulesInScopeSettings(t *testing.T) {
	dir := t.TempDir()

	scopePath := filepath.Join(dir, "work", ".claude")
	os.MkdirAll(scopePath, 0o755)

	if err := apply.WriteSettings(filepath.Join(scopePath, "settings.json"), map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read", "Write"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{
				{Rule: "Read", Scopes: []string{"work"}},
			},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Only "Write" should be recommended, "Read" is already managed.
	if len(result.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d: %v", len(result.Recommendations), result.Recommendations)
	}
	if result.Recommendations[0].Rule != "Write" {
		t.Errorf("expected rule 'Write', got %q", result.Recommendations[0].Rule)
	}
}

func TestScan_FindsGlobalScopePermissions(t *testing.T) {
	dir := t.TempDir()

	globalPath := filepath.Join(dir, ".claude")
	os.MkdirAll(globalPath, 0o755)

	if err := apply.WriteSettings(filepath.Join(globalPath, "settings.json"), map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(git:*)"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"global": {Path: globalPath, SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(result.Recommendations) != 1 {
		t.Fatalf("expected 1 recommendation, got %d", len(result.Recommendations))
	}
	if result.Recommendations[0].TargetScope != "global" {
		t.Errorf("expected target scope 'global', got %q", result.Recommendations[0].TargetScope)
	}
}

func TestScan_DeduplicatesScopeAndProjectRules(t *testing.T) {
	dir := t.TempDir()

	scopePath := filepath.Join(dir, "work", ".claude")
	os.MkdirAll(scopePath, 0o755)

	// Same rule in scope settings and a project under the scope.
	perms := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read"},
		},
	}
	if err := apply.WriteSettings(filepath.Join(scopePath, "settings.local.json"), perms); err != nil {
		t.Fatal(err)
	}
	writeProjectSettings(t, filepath.Join(dir, "work", "proj-a"), "settings.local.json", perms)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.local.json"},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Should be one recommendation with both paths in FoundIn.
	recs := filterRecs(result.Recommendations, "Read", "allow")
	if len(recs) != 1 {
		t.Fatalf("expected 1 recommendation for Read, got %d", len(recs))
	}
	if len(recs[0].FoundIn) != 2 {
		t.Errorf("expected 2 FoundIn paths, got %d: %v", len(recs[0].FoundIn), recs[0].FoundIn)
	}
}

func filterRecs(recs []Recommendation, rule, kind string) []Recommendation {
	var out []Recommendation
	for _, r := range recs {
		if r.Rule == rule && r.Type == kind {
			out = append(out, r)
		}
	}
	return out
}

func TestApplyRecommendations_RemovesFromScopeSettings(t *testing.T) {
	dir := t.TempDir()

	scopePath := filepath.Join(dir, "work", ".claude")
	os.MkdirAll(scopePath, 0o755)
	settingsPath := filepath.Join(scopePath, "settings.json")

	if err := apply.WriteSettings(settingsPath, map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read", "Write"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.EnsureMaps()

	recs := []Recommendation{{
		Rule:        "Read",
		Type:        "allow",
		FoundIn:     []string{settingsPath},
		TargetScope: "work",
	}}

	if err := ApplyRecommendations(cfg, recs); err != nil {
		t.Fatalf("ApplyRecommendations: %v", err)
	}

	// "Read" should be in config.
	if len(cfg.Permissions.Allow) != 1 || cfg.Permissions.Allow[0].Rule != "Read" {
		t.Errorf("expected Read in config, got %v", cfg.Permissions.Allow)
	}

	// "Write" should remain in settings file, "Read" removed.
	data, _ := apply.ReadSettings(settingsPath)
	perms, _ := data["permissions"].(map[string]any)
	remaining := apply.ExtractStringSlice(perms, "allow")
	if len(remaining) != 1 || remaining[0] != "Write" {
		t.Errorf("expected [Write] remaining, got %v", remaining)
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
