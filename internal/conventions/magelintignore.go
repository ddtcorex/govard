package conventions

func LintIgnore(quick bool) []string {
	base := []string{"pub/media/**", "pub/static/**", "var/**", "generated/**", ".worktrees/**", ".git/**", ".govard/**", "artifacts/**", "node_modules/**"}
	if quick {
		return append(base, "vendor/**", "dev/tests/**", "lib/**", "m2-hotfixes/**", "setup/src/**")
	}
	return base
}

func StableVolumeKey(name string) string { return "govard-" + name }
