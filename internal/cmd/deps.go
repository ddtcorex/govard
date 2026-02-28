package cmd

import (
	"fmt"
	"govard/internal/engine"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var fixDepsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Check and report required system dependencies",
	RunE: func(cmd *cobra.Command, args []string) error {
		missing, warnings := missingDependenciesWithWarnings()
		for _, warning := range warnings {
			pterm.Warning.Println(warning)
		}

		if len(missing) == 0 {
			pterm.Success.Println("All required dependencies are available.")
			return nil
		}

		pterm.Error.Printf("Missing dependencies: %s\n", strings.Join(missing, ", "))
		pterm.Info.Println("Install missing tools, then run `govard doctor fix-deps` again.")
		return fmt.Errorf("missing dependencies: %s", strings.Join(missing, ", "))
	},
}

func missingDependenciesWithWarnings() ([]string, []string) {
	missing := missingSystemDependencies()
	if len(missing) > 0 {
		return missing, nil
	}

	missingImages, warnings := missingRuntimeImagesForCurrentProject()
	for _, image := range missingImages {
		missing = append(missing, "docker image "+image)
	}
	return missing, warnings
}

func missingSystemDependencies() []string {
	missing := make([]string, 0, 4)

	if _, err := exec.LookPath("docker"); err != nil {
		missing = append(missing, "docker")
	} else if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		missing = append(missing, "docker compose plugin")
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		missing = append(missing, "ssh")
	}

	if _, err := exec.LookPath("rsync"); err != nil {
		missing = append(missing, "rsync")
	}

	return missing
}

func missingRuntimeImagesForCurrentProject() ([]string, []string) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, []string{fmt.Sprintf("Skipped runtime image checks: could not read working directory (%v)", err)}
	}

	config, _, err := engine.LoadConfigFromDir(cwd, true)
	if err != nil {
		if strings.Contains(err.Error(), engine.BaseConfigFile+" not found") {
			return nil, nil
		}
		return nil, []string{fmt.Sprintf("Skipped runtime image checks: %v", err)}
	}

	required := RequiredRuntimeImages(config)
	if len(required) == 0 {
		return nil, nil
	}

	missing := make([]string, 0, len(required))
	for _, image := range required {
		if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
			missing = append(missing, image)
		}
	}
	return missing, nil
}

// RequiredRuntimeImages returns all Docker images needed by the current runtime config.
func RequiredRuntimeImages(config engine.Config) []string {
	engine.NormalizeConfig(&config)

	imageRepo := strings.TrimSpace(os.Getenv("GOVARD_IMAGE_REPOSITORY"))
	if imageRepo == "" {
		imageRepo = "ddtcorex/govard-"
	}

	images := make([]string, 0, 8)
	push := func(image string) {
		image = strings.TrimSpace(image)
		if image != "" {
			images = append(images, image)
		}
	}

	if config.Framework == "nextjs" {
		push(fmt.Sprintf("node:%s-alpine", config.Stack.NodeVersion))
	} else {
		switch strings.ToLower(config.Stack.Services.WebServer) {
		case "apache":
			push(fmt.Sprintf("%sapache:%s", imageRepo, config.Stack.ApacheVersion))
		case "hybrid":
			push(fmt.Sprintf("%snginx:%s", imageRepo, config.Stack.NginxVersion))
			push(fmt.Sprintf("%sapache:%s", imageRepo, config.Stack.ApacheVersion))
		default:
			push(fmt.Sprintf("%snginx:%s", imageRepo, config.Stack.NginxVersion))
		}
		if config.Framework == "magento2" {
			push(fmt.Sprintf("%sphp-magento2:%s", imageRepo, config.Stack.PHPVersion))
		} else {
			push(fmt.Sprintf("%sphp:%s", imageRepo, config.Stack.PHPVersion))
		}
	}

	if config.Stack.DBType != "" && config.Stack.DBType != "none" && config.Framework != "nextjs" {
		push(fmt.Sprintf("%s%s:%s", imageRepo, config.Stack.DBType, config.Stack.DBVersion))
	}

	switch config.Stack.Services.Cache {
	case "redis":
		push(fmt.Sprintf("%sredis:%s", imageRepo, config.Stack.CacheVersion))
	case "valkey":
		push(fmt.Sprintf("%svalkey:%s", imageRepo, config.Stack.CacheVersion))
	}

	switch config.Stack.Services.Search {
	case "elasticsearch":
		push(fmt.Sprintf("%selasticsearch:%s", imageRepo, config.Stack.SearchVersion))
	case "opensearch":
		push(fmt.Sprintf("%sopensearch:%s", imageRepo, config.Stack.SearchVersion))
	}

	if config.Stack.Services.Queue == "rabbitmq" {
		push(fmt.Sprintf("%srabbitmq:%s", imageRepo, config.Stack.QueueVersion))
	}

	if config.Stack.Features.Varnish {
		push(fmt.Sprintf("%svarnish:%s", imageRepo, config.Stack.VarnishVersion))
	}

	seen := make(map[string]struct{}, len(images))
	uniq := make([]string, 0, len(images))
	for _, image := range images {
		if _, exists := seen[image]; exists {
			continue
		}
		seen[image] = struct{}{}
		uniq = append(uniq, image)
	}
	sort.Strings(uniq)
	return uniq
}

// FixDepsCommand exposes the deps command for tests.
func FixDepsCommand() *cobra.Command {
	return fixDepsCmd
}
