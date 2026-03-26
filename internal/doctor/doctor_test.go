package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/source"
)

func setupTestEnv(t *testing.T) (string, *config.Config, map[string][]source.Discovered) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("WOMBAT_HOME", root)

	// Create source dir with .git, a skill, and an agent.
	srcDir := filepath.Join(root, "sources", "test-source")
	os.MkdirAll(filepath.Join(srcDir, ".git"), 0o755)
	skillDir := filepath.Join(srcDir, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "my-agent.md"), []byte("---\nname: Agent\ndescription: D\n---\n"), 0o644)

	// Create scope directories in a separate subtree so source repos
	// aren't discovered as project dirs under the scope's parent.
	scopeRoot := filepath.Join(root, "scopes")
	globalScope := filepath.Join(scopeRoot, "global", ".claude")
	workScope := filepath.Join(scopeRoot, "work", ".claude")
	os.MkdirAll(globalScope, 0o755)
	os.MkdirAll(workScope, 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"global": {Path: globalScope, SettingsFile: "settings.json"},
			"work":   {Path: workScope, SettingsFile: "settings.local.json"},
		},
		Sources: map[string]config.Source{
			"test-source": {Git: "https://example.com/repo", DefaultScope: []string{"work"}},
		},
	}
	cfg.EnsureMaps()

	discovered := map[string][]source.Discovered{
		"test-source": {
			{Name: "my-skill", Path: "my-skill", Kind: "skill"},
			{Name: "my-agent", Path: "my-agent.md", Kind: "agent"},
		},
	}

	return root, cfg, discovered
}

func TestCheck_HealthyConfig(t *testing.T) {
	root, cfg, discovered := setupTestEnv(t)

	// Create the expected symlinks so everything is healthy.
	workScope := cfg.Scopes["work"]
	srcDir := filepath.Join(root, "sources", "test-source")
	for _, sub := range []struct{ dir, link, target string }{
		{filepath.Join(workScope.Path, "skills"), "my-skill", filepath.Join(srcDir, "my-skill")},
		{filepath.Join(workScope.Path, "agents"), "my-agent.md", filepath.Join(srcDir, "my-agent.md")},
	} {
		os.MkdirAll(sub.dir, 0o755)
		os.Symlink(sub.target, filepath.Join(sub.dir, sub.link))
	}

	findings := Check(cfg, discovered)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
	}
}

func TestCheck_MissingSource(t *testing.T) {
	_, cfg, discovered := setupTestEnv(t)

	cfg.Sources["missing-source"] = config.Source{Git: "https://example.com/missing"}

	findings := Check(cfg, discovered)
	found := false
	for _, f := range findings {
		if f.Severity == SevError && strings.Contains(f.Message, "missing-source") && strings.Contains(f.Message, "directory missing") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected finding about missing source, got: %v", findings)
	}
}

func TestCheck_MissingSymlink(t *testing.T) {
	_, cfg, discovered := setupTestEnv(t)

	// Don't create symlinks — they should be reported as missing.
	findings := Check(cfg, discovered)
	found := false
	for _, f := range findings {
		if f.Severity == SevError && strings.Contains(f.Message, "missing symlink") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected finding about missing symlinks, got: %v", findings)
	}
}

func TestCheck_NameCollision(t *testing.T) {
	root, cfg, _ := setupTestEnv(t)

	// Create a second source with the same skill name.
	srcDir2 := filepath.Join(root, "sources", "another-source")
	os.MkdirAll(filepath.Join(srcDir2, ".git"), 0o755)
	skillDir := filepath.Join(srcDir2, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644)

	cfg.Sources["another-source"] = config.Source{Git: "https://example.com/another"}

	discovered := map[string][]source.Discovered{
		"another-source": {{Name: "my-skill", Path: "my-skill", Kind: "skill"}},
		"test-source":    {{Name: "my-skill", Path: "my-skill", Kind: "skill"}},
	}

	findings := Check(cfg, discovered)
	found := false
	for _, f := range findings {
		if f.Severity == SevWarning && strings.Contains(f.Message, "collision") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected collision warning, got: %v", findings)
	}
}

func TestSummary_Empty(t *testing.T) {
	if s := Summary(nil); s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

func TestSummary_Categories(t *testing.T) {
	findings := []Finding{
		{Severity: SevError, Message: "missing symlink: /a"},
		{Severity: SevError, Message: "missing symlink: /b"},
		{Severity: SevWarning, Message: "scope work: settings drift"},
	}
	s := Summary(findings)
	if !strings.Contains(s, "2 missing symlinks") {
		t.Errorf("expected '2 missing symlinks' in %q", s)
	}
	if !strings.Contains(s, "1 settings drift") {
		t.Errorf("expected '1 settings drift' in %q", s)
	}
}

func TestSummary_UpdatesAvailable(t *testing.T) {
	findings := []Finding{
		{Severity: SevWarning, Message: "source foo: updates available"},
		{Severity: SevWarning, Message: "source bar: updates available"},
	}
	s := Summary(findings)
	if !strings.Contains(s, "2 updates available") {
		t.Errorf("expected '2 updates available' in %q", s)
	}
}

func TestHasErrors(t *testing.T) {
	if HasErrors(nil) {
		t.Error("nil findings should not have errors")
	}
	if HasErrors([]Finding{{Severity: SevWarning, Message: "warn"}}) {
		t.Error("warnings-only should not have errors")
	}
	if !HasErrors([]Finding{{Severity: SevError, Message: "err"}}) {
		t.Error("error finding should have errors")
	}
}
