package resolve

import (
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/source"
)

func TestEffectiveScopes_Override(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()
	cfg.Overrides["my-skill"] = config.ScopeSet{Enabled: []string{"work"}}
	cfg.Skills["my-skill"] = config.Item{Source: "src", Enabled: []string{"personal"}}

	scopes, inherited := EffectiveScopes(cfg, "my-skill", "src", []string{"default"}, "skill")

	if inherited {
		t.Error("expected inherited=false for override")
	}
	if len(scopes) != 1 || scopes[0] != "work" {
		t.Errorf("expected [work], got %v", scopes)
	}
}

func TestEffectiveScopes_ExplicitEntry(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()
	cfg.Skills["my-skill"] = config.Item{Source: "src", Enabled: []string{"work", "personal"}}

	scopes, inherited := EffectiveScopes(cfg, "my-skill", "src", []string{"default"}, "skill")

	if inherited {
		t.Error("expected inherited=false for explicit entry")
	}
	if len(scopes) != 2 {
		t.Errorf("expected 2 scopes, got %v", scopes)
	}
}

func TestEffectiveScopes_EmptyEnabledDisables(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()
	// Empty Enabled = explicitly disabled, should NOT fall through to default_scope.
	cfg.Skills["my-skill"] = config.Item{Source: "src", Enabled: []string{}}

	scopes, inherited := EffectiveScopes(cfg, "my-skill", "src", []string{"default"}, "skill")

	if inherited {
		t.Error("expected inherited=false for empty Enabled")
	}
	if len(scopes) != 0 {
		t.Errorf("expected empty scopes for disabled item, got %v", scopes)
	}
}

func TestEffectiveScopes_NilEnabledFallsThrough(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()
	// nil Enabled = absent, should fall through to default_scope.
	cfg.Skills["my-skill"] = config.Item{Source: "src"}

	scopes, inherited := EffectiveScopes(cfg, "my-skill", "src", []string{"work"}, "skill")

	if !inherited {
		t.Error("expected inherited=true for nil Enabled fall-through")
	}
	if len(scopes) != 1 || scopes[0] != "work" {
		t.Errorf("expected [work], got %v", scopes)
	}
}

func TestEffectiveScopes_NoMatch(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()

	scopes, inherited := EffectiveScopes(cfg, "unknown", "src", nil, "skill")

	if inherited {
		t.Error("expected inherited=false for unknown item")
	}
	if scopes != nil {
		t.Errorf("expected nil scopes, got %v", scopes)
	}
}

func TestEffectiveScopes_Plugin(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()
	cfg.Plugins["my-plugin"] = config.ScopeSet{Enabled: []string{"global"}}

	scopes, inherited := EffectiveScopes(cfg, "my-plugin", "", nil, "plugin")

	if inherited {
		t.Error("expected inherited=false for plugin")
	}
	if len(scopes) != 1 || scopes[0] != "global" {
		t.Errorf("expected [global], got %v", scopes)
	}
}

func TestEffectiveScopes_Agent(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()
	cfg.Agents["my-agent"] = config.Item{Source: "src", Enabled: []string{"work"}}

	scopes, inherited := EffectiveScopes(cfg, "my-agent", "src", []string{"default"}, "agent")

	if inherited {
		t.Error("expected inherited=false for agent")
	}
	if len(scopes) != 1 || scopes[0] != "work" {
		t.Errorf("expected [work], got %v", scopes)
	}
}

func TestEffectiveScopes_DefaultScope(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()

	scopes, inherited := EffectiveScopes(cfg, "my-skill", "src", []string{"work", "personal"}, "skill")

	if !inherited {
		t.Error("expected inherited=true for default_scope")
	}
	if len(scopes) != 2 {
		t.Errorf("expected 2 scopes, got %v", scopes)
	}
}

func TestItems_NameCollision(t *testing.T) {
	cfg := &config.Config{
		Sources: map[string]config.Source{
			"alpha-repo": {Git: "https://example.com/alpha", DefaultScope: []string{"work"}},
			"beta-repo":  {Git: "https://example.com/beta", DefaultScope: []string{"work"}},
		},
	}
	cfg.EnsureMaps()

	// Both sources discover a skill with the same name.
	discovered := map[string][]source.Discovered{
		"alpha-repo": {{Name: "shared-skill", Path: "shared-skill", Kind: "skill"}},
		"beta-repo":  {{Name: "shared-skill", Path: "shared-skill", Kind: "skill"}},
	}

	skills, _ := Items(cfg, discovered, false)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (deduped), got %d", len(skills))
	}
	// First alphabetically wins.
	if skills[0].SourceName != "alpha-repo" {
		t.Errorf("expected alpha-repo (first alphabetically), got %q", skills[0].SourceName)
	}
}

func TestItems_SkillAndAgentSameName(t *testing.T) {
	cfg := &config.Config{
		Sources: map[string]config.Source{
			"my-source": {Git: "https://example.com/repo", DefaultScope: []string{"work"}},
		},
	}
	cfg.EnsureMaps()

	// A skill and agent with the same name should both be resolved.
	discovered := map[string][]source.Discovered{
		"my-source": {
			{Name: "helper", Path: "helper", Kind: "skill"},
			{Name: "helper", Path: "agents/helper.md", Kind: "agent"},
		},
	}

	skills, agents := Items(cfg, discovered, false)
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

func TestItems_ExplicitEntryNotDiscovered(t *testing.T) {
	cfg := &config.Config{
		Sources: map[string]config.Source{
			"my-source": {Git: "https://example.com/repo"},
		},
		Skills: map[string]config.Item{
			"manual-skill": {Source: "my-source", Enabled: []string{"work"}},
		},
	}
	cfg.EnsureMaps()

	// Source discovers nothing.
	discovered := map[string][]source.Discovered{
		"my-source": {},
	}

	skills, _ := Items(cfg, discovered, false)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (explicit entry), got %d", len(skills))
	}
	if skills[0].Name != "manual-skill" {
		t.Errorf("expected manual-skill, got %q", skills[0].Name)
	}
}

func TestItems_IncludeAll(t *testing.T) {
	cfg := &config.Config{
		Sources: map[string]config.Source{
			"my-source": {Git: "https://example.com/repo"},
		},
	}
	cfg.EnsureMaps()

	discovered := map[string][]source.Discovered{
		"my-source": {{Name: "no-scope-skill", Path: "no-scope-skill", Kind: "skill"}},
	}

	// Without includeAll, items with no scopes are excluded.
	skills, _ := Items(cfg, discovered, false)
	if len(skills) != 0 {
		t.Errorf("expected 0 skills without includeAll, got %d", len(skills))
	}

	// With includeAll, they're included.
	skills, _ = Items(cfg, discovered, true)
	if len(skills) != 1 {
		t.Errorf("expected 1 skill with includeAll, got %d", len(skills))
	}
}
