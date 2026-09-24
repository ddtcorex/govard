package desktop

import "sort"

type trayProject struct {
	Project string
	Name    string
	URL     string
	Running bool
}

// buildTrayProjects lists every environment for the tray menu: running ones
// first, then by project name.
func buildTrayProjects(d Dashboard) []trayProject {
	out := make([]trayProject, 0, len(d.Environments))
	for _, env := range d.Environments {
		running := env.Status == "running" || env.Status == "syncing"
		url := ""
		if env.Domain != "" {
			url = "https://" + env.Domain
		}
		out = append(out, trayProject{Project: env.Project, Name: env.Name, URL: url, Running: running})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Running != out[j].Running {
			return out[i].Running
		}
		return out[i].Project < out[j].Project
	})
	return out
}
