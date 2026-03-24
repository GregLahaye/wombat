package source

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Discovered represents a skill or agent found in a source repository.
type Discovered struct {
	Name string // directory name (skill) or filename without .md (agent)
	Path string // relative path within the source repo
	Kind string // "skill" or "agent"
}

// skipFiles are markdown files that should never be treated as agents.
var skipFiles = map[string]bool{
	"readme.md": true, "changelog.md": true,
	"license.md": true, "contributing.md": true,
}

// DiscoverSkills scans dir/subpath one level deep for directories with SKILL.md.
func DiscoverSkills(repoDir, subpath string) ([]Discovered, error) {
	dir := filepath.Join(repoDir, subpath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var items []Discovered
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err != nil {
			continue
		}
		items = append(items, Discovered{
			Name: e.Name(),
			Path: filepath.Join(subpath, e.Name()),
			Kind: "skill",
		})
	}
	return items, nil
}

// DiscoverAgents scans for .md files with YAML frontmatter (name + description).
func DiscoverAgents(repoDir, subpath string) ([]Discovered, error) {
	dir := filepath.Join(repoDir, subpath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var items []Discovered
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		if skipFiles[strings.ToLower(e.Name())] {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if hasName, hasDesc := parseFrontmatter(path); hasName && hasDesc {
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			rel := e.Name()
			if subpath != "" {
				rel = filepath.Join(subpath, e.Name())
			}
			items = append(items, Discovered{Name: name, Path: rel, Kind: "agent"})
		}
	}
	return items, nil
}

// parseFrontmatter checks if file has --- delimited YAML with name: and description:.
func parseFrontmatter(path string) (hasName, hasDesc bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return false, false
	}
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > 50 {
			break // Limit frontmatter scan to prevent reading huge files.
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			hasName = strings.TrimSpace(val) != ""
		case "description":
			hasDesc = strings.TrimSpace(val) != ""
		}
		if hasName && hasDesc {
			return true, true
		}
	}
	return hasName, hasDesc
}
