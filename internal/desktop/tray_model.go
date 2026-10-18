package desktop

import (
	"fmt"
	"sort"
)

type trayProject struct {
	Project string
	Name    string
	URL     string
	Running bool
}

// trayTitle renders the tray label with the running-op count: plain "Govard"
// when idle, with a "(N op/N ops)" badge while operations run. The refresh
// path (see tray_wails.go) applies it to the label and tooltip on every
// rebuild so the count tracks ListOps.
func trayTitle(runningOps int) string {
	if runningOps <= 0 {
		return trayName
	}
	if runningOps == 1 {
		return trayName + " (1 op)"
	}
	return fmt.Sprintf("%s (%d ops)", trayName, runningOps)
}

// buildTrayProjects lists every environment for the tray menu: running ones
// first, then by project name.
func buildTrayProjects(d Dashboard) []trayProject {
	out := make([]trayProject, 0, len(d.Environments))
	for _, env := range d.Environments {
		// Degraded and warning are still running: the tray must offer
		// Stop, not Start-and-recreate.
		running := env.Status == "running" || env.Status == "syncing" ||
			env.Status == "degraded" || env.Status == "warning"
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
