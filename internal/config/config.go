// Package config defines the central data types and YAML-backed persistence
// for wombat configuration.
package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the single source of truth for wombat.
type Config struct {
	Scopes      map[string]Scope      `yaml:"scopes"`
	Sources     map[string]Source      `yaml:"sources,omitempty"`
	Plugins     map[string]ScopeSet   `yaml:"plugins,omitempty"`
	Skills      map[string]Item       `yaml:"skills,omitempty"`
	Agents      map[string]Item       `yaml:"agents,omitempty"`
	Overrides   map[string]ScopeSet   `yaml:"overrides,omitempty"`
	Permissions Permissions           `yaml:"permissions,omitempty"`
}

// Scope maps a name to a Claude Code settings directory.
type Scope struct {
	Path         string `yaml:"path"`
	SettingsFile string `yaml:"settings_file"`
}

// Source defines a git repository containing skills and/or agents.
type Source struct {
	Git          string   `yaml:"git"`
	DefaultScope []string `yaml:"default_scope,omitempty"`
	SkillPaths   []string `yaml:"skill_paths,omitempty"`
	AgentPath    string   `yaml:"agent_path,omitempty"`
}

// SkillDirs returns directories to scan for skills. Defaults to repo root.
func (s Source) SkillDirs() []string {
	if len(s.SkillPaths) > 0 {
		return s.SkillPaths
	}
	return []string{""}
}

// ScopeSet tracks which scopes something is enabled in.
type ScopeSet struct {
	Enabled []string `yaml:"enabled"`
}

// Item references a source and the scopes it is enabled in.
// Used for both skills and agents.
type Item struct {
	Source  string   `yaml:"source"`
	Enabled []string `yaml:"enabled"`
}

// Permissions holds allow and deny rules with scope targeting.
type Permissions struct {
	Allow []PermissionRule `yaml:"allow,omitempty"`
	Deny  []PermissionRule `yaml:"deny,omitempty"`
}

// PermissionRule is a single allow or deny entry.
type PermissionRule struct {
	Rule   string   `yaml:"rule"`
	Scopes []string `yaml:"scopes"`
}

// Load reads and parses config.yaml, expanding ~ in scope paths.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found at %s — run wombat init", path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	for name, scope := range cfg.Scopes {
		scope.Path = ExpandPath(scope.Path)
		cfg.Scopes[name] = scope
	}

	cfg.EnsureMaps()
	return &cfg, nil
}

// Save writes the config to YAML atomically (temp + rename).
func (c *Config) Save(path string) error {
	// Contract paths for portability before saving.
	clone := c.Clone()
	for name, scope := range clone.Scopes {
		scope.Path = ContractPath(scope.Path)
		clone.Scopes[name] = scope
	}

	data, err := yaml.Marshal(clone)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return AtomicWrite(path, data, 0o644)
}

// Validate checks referential integrity: sources exist, scopes exist,
// no duplicate permission rules.
func (c *Config) Validate() error {
	pathToScope := make(map[string]string)
	for name, scope := range c.Scopes {
		if scope.Path == "" {
			return fmt.Errorf("scope %q: empty path", name)
		}
		if scope.SettingsFile == "" {
			return fmt.Errorf("scope %q: empty settings_file", name)
		}
		cleaned := filepath.Clean(scope.Path)
		if other, ok := pathToScope[cleaned]; ok {
			a, b := other, name
			if a > b {
				a, b = b, a
			}
			return fmt.Errorf("scopes %q and %q have the same path %q", a, b, scope.Path)
		}
		pathToScope[cleaned] = name
	}
	for name, src := range c.Sources {
		if src.Git == "" {
			return fmt.Errorf("source %q: empty git URL", name)
		}
	}

	validScope := make(map[string]bool, len(c.Scopes))
	for name := range c.Scopes {
		validScope[name] = true
	}

	check := func(section, item string, scopes []string) error {
		for _, s := range scopes {
			if !validScope[s] {
				return fmt.Errorf("%s %q references unknown scope %q", section, item, s)
			}
		}
		return nil
	}

	for name, src := range c.Sources {
		if err := check("source", name, src.DefaultScope); err != nil {
			return err
		}
	}
	for name, p := range c.Plugins {
		if err := check("plugin", name, p.Enabled); err != nil {
			return err
		}
	}
	for name, sk := range c.Skills {
		if _, ok := c.Sources[sk.Source]; !ok {
			return fmt.Errorf("skill %q references unknown source %q", name, sk.Source)
		}
		if err := check("skill", name, sk.Enabled); err != nil {
			return err
		}
	}
	for name, ag := range c.Agents {
		if _, ok := c.Sources[ag.Source]; !ok {
			return fmt.Errorf("agent %q references unknown source %q", name, ag.Source)
		}
		if err := check("agent", name, ag.Enabled); err != nil {
			return err
		}
	}
	for name, ov := range c.Overrides {
		if err := check("override", name, ov.Enabled); err != nil {
			return err
		}
	}

	if err := c.validateRules("allow", c.Permissions.Allow, check); err != nil {
		return err
	}
	return c.validateRules("deny", c.Permissions.Deny, check)
}

func (c *Config) validateRules(kind string, rules []PermissionRule, check func(string, string, []string) error) error {
	seen := make(map[string]bool)
	for i, rule := range rules {
		if rule.Rule == "" {
			return fmt.Errorf("permissions.%s[%d]: empty rule", kind, i)
		}
		label := fmt.Sprintf("[%d] %s", i, rule.Rule)
		if err := check("permissions."+kind, label, rule.Scopes); err != nil {
			return err
		}
		if seen[rule.Rule] {
			return fmt.Errorf("permissions.%s has duplicate rule %q — merge into one entry", kind, rule.Rule)
		}
		seen[rule.Rule] = true
	}
	return nil
}

// Clone returns a deep copy via JSON round-trip.
// Panics on marshal failure (indicates a bug in the Config type).
func (c *Config) Clone() *Config {
	data, err := json.Marshal(c)
	if err != nil {
		panic("config: Clone marshal: " + err.Error())
	}
	var cp Config
	if err := json.Unmarshal(data, &cp); err != nil {
		panic("config: Clone unmarshal: " + err.Error())
	}
	cp.EnsureMaps()
	return &cp
}

// Equal reports whether two configs are identical.
func (c *Config) Equal(other *Config) bool {
	a, err := json.Marshal(c)
	if err != nil {
		panic("config: Equal marshal: " + err.Error())
	}
	b, err := json.Marshal(other)
	if err != nil {
		panic("config: Equal marshal: " + err.Error())
	}
	return string(a) == string(b)
}

// ScopeNames returns scope names sorted alphabetically with "global" last.
func (c *Config) ScopeNames() []string {
	names := slices.Sorted(maps.Keys(c.Scopes))
	if i := slices.Index(names, "global"); i >= 0 {
		names = append(names[:i], names[i+1:]...)
		names = append(names, "global")
	}
	return names
}

// SortedSourceNames returns source names in alphabetical order.
func (c *Config) SortedSourceNames() []string {
	return slices.Sorted(maps.Keys(c.Sources))
}

// ScopeRefs returns all config entries that reference the given scope name.
func (c *Config) ScopeRefs(scope string) []string {
	var refs []string
	for name, src := range c.Sources {
		if slices.Contains(src.DefaultScope, scope) {
			refs = append(refs, "source:"+name)
		}
	}
	for name, p := range c.Plugins {
		if slices.Contains(p.Enabled, scope) {
			refs = append(refs, "plugin:"+name)
		}
	}
	for name, sk := range c.Skills {
		if slices.Contains(sk.Enabled, scope) {
			refs = append(refs, "skill:"+name)
		}
	}
	for name, ag := range c.Agents {
		if slices.Contains(ag.Enabled, scope) {
			refs = append(refs, "agent:"+name)
		}
	}
	for name, ov := range c.Overrides {
		if slices.Contains(ov.Enabled, scope) {
			refs = append(refs, "override:"+name)
		}
	}
	for _, rule := range c.Permissions.Allow {
		if slices.Contains(rule.Scopes, scope) {
			refs = append(refs, "permission:allow:"+rule.Rule)
		}
	}
	for _, rule := range c.Permissions.Deny {
		if slices.Contains(rule.Scopes, scope) {
			refs = append(refs, "permission:deny:"+rule.Rule)
		}
	}
	slices.Sort(refs)
	return refs
}

// EnsureMaps initialises nil maps so callers never need nil checks.
func (c *Config) EnsureMaps() {
	if c.Scopes == nil {
		c.Scopes = make(map[string]Scope)
	}
	if c.Sources == nil {
		c.Sources = make(map[string]Source)
	}
	if c.Plugins == nil {
		c.Plugins = make(map[string]ScopeSet)
	}
	if c.Skills == nil {
		c.Skills = make(map[string]Item)
	}
	if c.Agents == nil {
		c.Agents = make(map[string]Item)
	}
	if c.Overrides == nil {
		c.Overrides = make(map[string]ScopeSet)
	}
}

// --- Path helpers ---

// ExpandPath replaces a leading ~ with the user's home directory.
func ExpandPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		if p == "~" {
			return home
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

// ContractPath replaces the home directory prefix with ~/ for storage.
func ContractPath(p string) string {
	p = filepath.Clean(p)
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~/" + p[len(home)+1:]
	}
	return p
}

// ConfigDir returns the wombat home directory (WOMBAT_HOME or ~/wombat).
func ConfigDir() string {
	if dir := os.Getenv("WOMBAT_HOME"); dir != "" {
		return ExpandPath(dir)
	}
	return ExpandPath("~/wombat")
}

// ConfigPath returns the full path to config.yaml.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// SourcesDir returns the directory where source repos are cloned.
func SourcesDir() string {
	return filepath.Join(ConfigDir(), "sources")
}

// AtomicWrite writes data to path via temp file + rename.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".wombat-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		// Clean up on failure.
		if tmp != "" {
			os.Remove(tmp)
		}
	}()

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	tmp = "" // Prevent deferred cleanup.
	return nil
}
