package apply

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/resolve"
)

// syncSymlinks creates and removes symlinks for resolved items.
// projDirs maps scope names to project .claude directories for propagation.
func syncSymlinks(cfg *config.Config, items []resolve.ResolvedItem, subdir string, r *Result, projDirs map[string][]string) {
	sourcesDir := config.SourcesDir()

	// Build desired state: .claude path -> link filename -> target path.
	desired := make(map[string]map[string]string)
	addDesired := func(claudePath, linkName, target string) {
		if desired[claudePath] == nil {
			desired[claudePath] = make(map[string]string)
		}
		desired[claudePath][linkName] = target
	}

	for _, item := range items {
		if item.SourcePath == "" {
			continue
		}
		absPath := filepath.Join(sourcesDir, item.SourceName, item.SourcePath)
		scopes := item.Scopes
		if slices.Contains(scopes, "global") {
			scopes = []string{"global"}
		}
		ln := item.LinkName()
		for _, scopeName := range scopes {
			scope, ok := cfg.Scopes[scopeName]
			if !ok {
				continue
			}
			addDesired(scope.Path, ln, absPath)
			for _, projDir := range projDirs[scopeName] {
				addDesired(projDir, ln, absPath)
			}
		}
	}

	// Create/update desired symlinks.
	for claudePath, items := range desired {
		dir := filepath.Join(claudePath, subdir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("creating %s: %w", dir, err))
			continue
		}
		for name, target := range items {
			link := filepath.Join(dir, name)
			created, err := ensureSymlink(link, target)
			if err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("symlink %s: %w", link, err))
				continue
			}
			if created {
				r.Created = append(r.Created, link)
			}
		}
	}

	// Remove stale symlinks (only those pointing into sources/).
	sourcesPrefix := filepath.Clean(sourcesDir) + string(filepath.Separator)
	cleanStale := func(claudePath string) {
		dir := filepath.Join(claudePath, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			link := filepath.Join(dir, entry.Name())
			target, err := os.Readlink(link)
			if err != nil {
				continue
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			target = filepath.Clean(target)
			if !strings.HasPrefix(target, sourcesPrefix) {
				continue
			}
			if d, ok := desired[claudePath]; ok {
				if _, wanted := d[entry.Name()]; wanted {
					continue
				}
			}
			if err := os.Remove(link); err == nil {
				r.Removed = append(r.Removed, link)
			}
		}
	}

	for _, scope := range cfg.Scopes {
		cleanStale(scope.Path)
	}
	for _, dirs := range projDirs {
		for _, d := range dirs {
			cleanStale(d)
		}
	}
}

// ensureSymlink creates or updates a symlink. Returns true if created/changed.
func ensureSymlink(link, target string) (bool, error) {
	existing, err := os.Readlink(link)
	if err == nil {
		if existing == target {
			return false, nil // Already correct.
		}
		if err := os.Remove(link); err != nil {
			return false, fmt.Errorf("removing old symlink: %w", err)
		}
	} else if _, statErr := os.Lstat(link); statErr == nil {
		// Path exists but is not a symlink (regular file or directory).
		return false, fmt.Errorf("non-symlink file exists at %s (remove manually)", link)
	}

	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return false, err
	}
	return true, os.Symlink(target, link)
}
