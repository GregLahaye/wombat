package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/GregLahaye/wombat/internal/config"
)

// ReadSettings reads a JSON settings file. Returns empty map if missing.
func ReadSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return make(map[string]any), nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m == nil {
		return make(map[string]any), nil
	}
	return m, nil
}

// WriteSettings writes a JSON settings file atomically (temp + rename).
func WriteSettings(path string, data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling: %w", err)
	}
	b = append(b, '\n')
	return config.AtomicWrite(path, b, 0o644)
}

// syncSettings merges plugins and permissions into each scope's settings file.
func syncSettings(cfg, prevCfg *config.Config, r *Result) error {
	for _, scopeName := range cfg.ScopeNames() {
		scope := cfg.Scopes[scopeName]
		path := filepath.Join(scope.Path, scope.SettingsFile)
		data, err := ReadSettings(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		changed := mergePlugins(data, cfg, prevCfg, scopeName)
		changed = mergePermissions(data, cfg, prevCfg, scopeName) || changed

		if changed {
			if err := WriteSettings(path, data); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			r.Updated = append(r.Updated, path)
		}
	}
	return nil
}

// CheckSettings does a dry-run merge and returns scope names with drift.
func CheckSettings(cfg *config.Config) ([]string, error) {
	var drifted []string
	for _, scopeName := range cfg.ScopeNames() {
		scope := cfg.Scopes[scopeName]
		path := filepath.Join(scope.Path, scope.SettingsFile)
		data, err := ReadSettings(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		clone := cloneMap(data)
		if clone == nil {
			clone = make(map[string]any)
		}
		if mergePlugins(clone, cfg, nil, scopeName) || mergePermissions(clone, cfg, nil, scopeName) {
			drifted = append(drifted, scopeName)
		}
	}
	return drifted, nil
}

// mergePlugins applies partial-ownership merge for enabledPlugins.
func mergePlugins(data map[string]any, cfg, prevCfg *config.Config, scope string) bool {
	plugins, _ := data["enabledPlugins"].(map[string]any)
	if plugins == nil {
		plugins = make(map[string]any)
	}

	changed := false

	for name, plugin := range cfg.Plugins {
		scopes := plugin.Enabled
		if ov, ok := cfg.Overrides[name]; ok {
			scopes = ov.Enabled
		}
		want := slices.Contains(scopes, scope)
		current, _ := plugins[name].(bool)
		if want && !current {
			plugins[name] = true
			changed = true
		} else if !want && current {
			plugins[name] = false
			changed = true
		}
	}

	// Disable plugins removed from config.
	if prevCfg != nil {
		for name := range prevCfg.Plugins {
			if _, exists := cfg.Plugins[name]; exists {
				continue
			}
			if v, _ := plugins[name].(bool); v {
				plugins[name] = false
				changed = true
			}
		}
	}

	if changed {
		data["enabledPlugins"] = plugins
	}
	return changed
}

// mergePermissions applies partial-ownership merge for permission rules.
func mergePermissions(data map[string]any, cfg, prevCfg *config.Config, scope string) bool {
	perms, _ := data["permissions"].(map[string]any)
	if perms == nil {
		perms = make(map[string]any)
	}

	changed := false
	changed = mergeRuleList(perms, "allow", cfg.Permissions.Allow, prevRules(prevCfg, "allow"), scope) || changed
	changed = mergeRuleList(perms, "deny", cfg.Permissions.Deny, prevRules(prevCfg, "deny"), scope) || changed

	if changed {
		if len(perms) > 0 {
			data["permissions"] = perms
		} else {
			delete(data, "permissions")
		}
	}
	return changed
}

func prevRules(prev *config.Config, kind string) []config.PermissionRule {
	if prev == nil {
		return nil
	}
	if kind == "allow" {
		return prev.Permissions.Allow
	}
	return prev.Permissions.Deny
}

// mergeRuleList merges a single rule list with partial ownership.
func mergeRuleList(perms map[string]any, key string, rules, prevRules []config.PermissionRule, scope string) bool {
	// Build owned set: rules managed by wombat.
	owned := make(map[string]bool)
	for _, r := range rules {
		owned[r.Rule] = true
	}
	for _, r := range prevRules {
		owned[r.Rule] = true
	}

	// Build desired set: rules that should be in this scope.
	desired := make(map[string]bool)
	for _, r := range rules {
		if slices.Contains(r.Scopes, scope) {
			desired[r.Rule] = true
		}
	}

	// Read existing rules from settings file.
	existing := ExtractStringSlice(perms, key)

	// Merge: keep unowned, add desired, remove owned-but-unwanted.
	changed := false
	var result []string
	for _, r := range existing {
		if owned[r] && !desired[r] {
			changed = true
			continue
		}
		result = append(result, r)
	}

	existingSet := make(map[string]bool)
	for _, r := range result {
		existingSet[r] = true
	}
	for rule := range desired {
		if !existingSet[rule] {
			result = append(result, rule)
			changed = true
		}
	}

	if changed {
		// Preserve non-string entries from the original array.
		raw, _ := perms[key].([]any)
		var anySlice []any
		for _, v := range raw {
			if _, ok := v.(string); !ok {
				anySlice = append(anySlice, v)
			}
		}
		for _, r := range result {
			anySlice = append(anySlice, r)
		}
		if len(anySlice) > 0 {
			perms[key] = anySlice
		} else {
			delete(perms, key)
		}
	}

	return changed
}

// ExtractStringSlice extracts a []string from a map[string]any by key.
func ExtractStringSlice(m map[string]any, key string) []string {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	slice, ok := raw.([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, v := range slice {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func cloneMap(m map[string]any) map[string]any {
	data, err := json.Marshal(m)
	if err != nil {
		panic("settings: cloneMap marshal: " + err.Error())
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		panic("settings: cloneMap unmarshal: " + err.Error())
	}
	return clone
}
