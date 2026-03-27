// Package tidy scans project-level Claude Code settings files and
// recommends consolidating repeated permissions into scopes.
package tidy

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
)

// Recommendation describes a permission that can be consolidated.
type Recommendation struct {
	Rule        string
	Type        string   // "allow" or "deny"
	FoundIn     []string // project settings paths
	TargetScope string
	Reason      string
}

// ScanResult holds the output of a tidy scan.
type ScanResult struct {
	Scanned         int
	Recommendations []Recommendation
}

// Scan examines scope-level and project-level settings files and returns
// recommendations for adopting unmanaged permissions into wombat config.
func Scan(cfg *config.Config) (*ScanResult, error) {
	result := &ScanResult{}

	// Build set of rules already managed by wombat per scope.
	managedRules := make(map[string]map[string]bool)
	for _, scopeName := range cfg.ScopeNames() {
		m := make(map[string]bool)
		for _, r := range cfg.Permissions.Allow {
			if slices.Contains(r.Scopes, scopeName) {
				m["allow\x00"+r.Rule] = true
			}
		}
		for _, r := range cfg.Permissions.Deny {
			if slices.Contains(r.Scopes, scopeName) {
				m["deny\x00"+r.Rule] = true
			}
		}
		managedRules[scopeName] = m
	}

	// Step 0: Scope-level settings → adopt into config.
	for _, scopeName := range cfg.ScopeNames() {
		scope := cfg.Scopes[scopeName]
		path := filepath.Join(scope.Path, scope.SettingsFile)
		data, err := apply.ReadSettings(path)
		if err != nil || len(data) == 0 {
			continue
		}
		managed := managedRules[scopeName]
		for _, kind := range []string{"allow", "deny"} {
			for _, rule := range extractRules(data, kind) {
				if managed[kind+"\x00"+rule] {
					continue
				}
				result.Recommendations = append(result.Recommendations, Recommendation{
					Rule:        rule,
					Type:        kind,
					FoundIn:     []string{path},
					TargetScope: scopeName,
					Reason:      fmt.Sprintf("Found in %s scope settings, not managed by wombat", scopeName),
				})
			}
		}
	}

	// Collect project permissions per scope.
	scopeProjects := make(map[string][]projectPerms)

	for _, scopeName := range cfg.ScopeNames() {
		if scopeName == "global" {
			continue
		}
		scope := cfg.Scopes[scopeName]
		// Scan one level deep: parent/<project>/.claude/<settings_file>
		parent := filepath.Dir(scope.Path)
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(parent, entry.Name(), ".claude", scope.SettingsFile)
			data, err := apply.ReadSettings(path)
			if err != nil || len(data) == 0 {
				continue
			}
			result.Scanned++
			pp := projectPerms{Path: path}
			pp.Allow = extractRules(data, "allow")
			pp.Deny = extractRules(data, "deny")
			if len(pp.Allow) > 0 || len(pp.Deny) > 0 {
				scopeProjects[scopeName] = append(scopeProjects[scopeName], pp)
			}
		}
	}

	// Step 1: Any project-level permission -> scope level.
	type ruleKey struct{ rule, kind string }
	for scopeName, projects := range scopeProjects {
		managed := managedRules[scopeName]
		for _, kind := range []string{"allow", "deny"} {
			counts := countRules(projects, kind)
			for rule, paths := range counts {
				if managed[kind+"\x00"+rule] {
					continue
				}
				result.Recommendations = append(result.Recommendations, Recommendation{
					Rule:        rule,
					Type:        kind,
					FoundIn:     paths,
					TargetScope: scopeName,
					Reason:      fmt.Sprintf("Found in %d project(s) under %s", len(paths), scopeName),
				})
			}
		}
	}

	// Deduplicate: merge scope + project recommendations for same rule/type/scope.
	{
		seen := make(map[string]int) // key → index in deduped
		var deduped []Recommendation
		for _, rec := range result.Recommendations {
			key := rec.Type + "\x00" + rec.Rule + "\x00" + rec.TargetScope
			if idx, ok := seen[key]; ok {
				deduped[idx].FoundIn = append(deduped[idx].FoundIn, rec.FoundIn...)
				deduped[idx].Reason = fmt.Sprintf("Found in %d location(s) under %s", len(deduped[idx].FoundIn), rec.TargetScope)
			} else {
				seen[key] = len(deduped)
				deduped = append(deduped, rec)
			}
		}
		result.Recommendations = deduped
	}

	// Step 2: Rules in ALL non-global scopes -> promote to global.
	// Only if a global scope exists, otherwise there's nothing to promote to.
	_, hasGlobal := cfg.Scopes["global"]
	nonGlobalCount := len(cfg.Scopes)
	if hasGlobal {
		nonGlobalCount--
	}
	if hasGlobal && nonGlobalCount >= 2 && len(scopeProjects) >= 2 {
		type sr struct{ kind, rule string }
		ruleScopes := make(map[sr][]string)
		for scopeName, projects := range scopeProjects {
			managed := managedRules[scopeName]
			for _, kind := range []string{"allow", "deny"} {
				for rule := range countRules(projects, kind) {
					if managed[kind+"\x00"+rule] {
						continue
					}
					key := sr{kind, rule}
					ruleScopes[key] = append(ruleScopes[key], scopeName)
				}
			}
		}

		step1Rules := make(map[ruleKey]bool)
		for _, rec := range result.Recommendations {
			step1Rules[ruleKey{rec.Rule, rec.Type}] = true
		}

		for key, scopes := range ruleScopes {
			if len(scopes) != nonGlobalCount {
				continue
			}
			var allPaths []string
			for _, s := range scopes {
				allPaths = append(allPaths, countRules(scopeProjects[s], key.kind)[key.rule]...)
			}
			slices.Sort(allPaths)

			if step1Rules[ruleKey{key.rule, key.kind}] {
				result.Recommendations = slices.DeleteFunc(result.Recommendations, func(r Recommendation) bool {
					return r.Rule == key.rule && r.Type == key.kind
				})
			}
			result.Recommendations = append(result.Recommendations, Recommendation{
				Rule:        key.rule,
				Type:        key.kind,
				FoundIn:     allPaths,
				TargetScope: "global",
				Reason:      fmt.Sprintf("Found across all %d scopes — consolidate to global", len(scopes)),
			})
		}
	}

	// Sort: global first, then by type and rule.
	slices.SortFunc(result.Recommendations, func(a, b Recommendation) int {
		if a.TargetScope != b.TargetScope {
			if a.TargetScope == "global" {
				return -1
			}
			if b.TargetScope == "global" {
				return 1
			}
			return cmp.Compare(a.TargetScope, b.TargetScope)
		}
		if c := cmp.Compare(a.Type, b.Type); c != 0 {
			return c
		}
		return cmp.Compare(a.Rule, b.Rule)
	})

	return result, nil
}

// ApplyRecommendations adds recommended permissions to config and removes
// them from project-level settings files.
func ApplyRecommendations(cfg *config.Config, recs []Recommendation) error {
	for _, rec := range recs {
		addToConfig(cfg, rec)
		for _, path := range rec.FoundIn {
			if err := removeFromFile(path, rec.Rule, rec.Type); err != nil {
				return fmt.Errorf("updating %s: %w", path, err)
			}
		}
	}
	return nil
}

func addToConfig(cfg *config.Config, rec Recommendation) {
	var rules *[]config.PermissionRule
	if rec.Type == "allow" {
		rules = &cfg.Permissions.Allow
	} else {
		rules = &cfg.Permissions.Deny
	}

	for i, existing := range *rules {
		if existing.Rule == rec.Rule {
			if !slices.Contains(existing.Scopes, rec.TargetScope) {
				(*rules)[i].Scopes = append((*rules)[i].Scopes, rec.TargetScope)
			}
			return
		}
	}
	*rules = append(*rules, config.PermissionRule{
		Rule: rec.Rule, Scopes: []string{rec.TargetScope},
	})
}

func removeFromFile(path, rule, kind string) error {
	data, err := apply.ReadSettings(path)
	if err != nil {
		return err
	}
	perms, ok := data["permissions"].(map[string]any)
	if !ok {
		return nil
	}
	if list, ok := perms[kind].([]any); ok {
		var filtered []any
		for _, v := range list {
			if s, ok := v.(string); ok && s == rule {
				continue
			}
			filtered = append(filtered, v)
		}
		if len(filtered) > 0 {
			perms[kind] = filtered
		} else {
			delete(perms, kind)
		}
	}
	if len(perms) == 0 {
		delete(data, "permissions")
	} else {
		data["permissions"] = perms
	}
	if len(data) == 0 {
		return os.Remove(path)
	}
	return apply.WriteSettings(path, data)
}

func countRules(projects []projectPerms, kind string) map[string][]string {
	counts := make(map[string][]string)
	for _, p := range projects {
		var rules []string
		if kind == "allow" {
			rules = p.Allow
		} else {
			rules = p.Deny
		}
		for _, rule := range rules {
			counts[rule] = append(counts[rule], p.Path)
		}
	}
	return counts
}

type projectPerms struct {
	Path  string
	Allow []string
	Deny  []string
}

func extractRules(data map[string]any, key string) []string {
	perms, ok := data["permissions"].(map[string]any)
	if !ok {
		return nil
	}
	return apply.ExtractStringSlice(perms, key)
}
