package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GregLahaye/wombat/internal/apply"
	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/doctor"
	"github.com/GregLahaye/wombat/internal/source"
	"github.com/spf13/cobra"
)

func DoctorCmd() *cobra.Command {
	var verbose, offline bool
	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   "Check configuration health",
		GroupID: "maintain",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			noColor := os.Getenv("NO_COLOR") != ""
			errors, warnings := 0, 0

			report := func(level string, f *doctor.Finding) {
				msg := ""
				if f != nil {
					msg = f.Message
				}
				switch level {
				case "error":
					errors++
					if noColor {
						fmt.Printf("[ERROR] %s\n", msg)
					} else {
						fmt.Printf("✗ %s\n", msg)
					}
				case "warning":
					warnings++
					if noColor {
						fmt.Printf("[WARN]  %s\n", msg)
					} else {
						fmt.Printf("⚠ %s\n", msg)
					}
				default:
					if verbose {
						if noColor {
							fmt.Printf("[OK]    %s\n", msg)
						} else {
							fmt.Printf("✓ %s\n", msg)
						}
					}
					return
				}
				if f != nil {
					for _, detail := range f.Details {
						if noColor {
							fmt.Printf("        - %s\n", detail)
						} else {
							fmt.Printf("  - %s\n", detail)
						}
					}
					if f.Hint != "" {
						if noColor {
							fmt.Printf("        → %s\n", f.Hint)
						} else {
							fmt.Printf("  → %s\n", f.Hint)
						}
					}
				}
			}

			// Run shared local checks.
			findings := doctor.Check(cfg, nil)
			for _, f := range findings {
				switch f.Severity {
				case doctor.SevError:
					report("error", &f)
				case doctor.SevWarning:
					report("warning", &f)
				}
			}

			// Verbose: report healthy sources and discovered projects.
			if verbose {
				sourcesDir := config.SourcesDir()
				for _, name := range cfg.SortedSourceNames() {
					dir := filepath.Join(sourcesDir, name)
					if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
						hasIssue := false
						for _, f := range findings {
							if strings.HasPrefix(f.Message, "source "+name+":") {
								hasIssue = true
								break
							}
						}
						if !hasIssue {
							report("info", &doctor.Finding{Message: fmt.Sprintf("source %q: ok", name)})
						}
					}
				}

				projDirs := apply.DiscoverAllProjectDirs(cfg)
				for _, scopeName := range cfg.ScopeNames() {
					if scopeName == "global" {
						continue
					}
					if dirs := projDirs[scopeName]; len(dirs) > 0 {
						report("info", &doctor.Finding{Message: fmt.Sprintf("scope %q: %d project(s) discovered", scopeName, len(dirs))})
					}
				}
			}

			// Remote update checks (CLI-only, requires network).
			if !offline {
				sourcesDir := config.SourcesDir()
				for _, name := range cfg.SortedSourceNames() {
					dir := filepath.Join(sourcesDir, name)
					if _, err := os.Stat(dir); err != nil {
						continue
					}
					hasUpdates, err := source.HasUpdates(dir)
					if err != nil {
						report("warning", &doctor.Finding{Message: fmt.Sprintf("source %q: could not check for updates: %v", name, err)})
					} else if hasUpdates {
						report("warning", &doctor.Finding{Message: fmt.Sprintf("source %q: updates available", name), Hint: "run wombat pull"})
					}
				}
			}

			if errors > 0 {
				return fmt.Errorf("%d errors, %d warnings", errors, warnings)
			}
			if warnings > 0 {
				fmt.Printf("\n%d warnings\n", warnings)
			} else {
				fmt.Println("\nAll checks passed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show passing checks")
	cmd.Flags().BoolVar(&offline, "offline", false, "Skip network checks (remote updates)")
	return cmd
}
