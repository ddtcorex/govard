package engine

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"govard/internal/conventions"
)

// RequiredRuntimeImages returns all Docker images needed by the current runtime config.
func RequiredRuntimeImages(config Config, root string) []string {
	NormalizeConfig(&config, root)

	imageRepo := strings.TrimSpace(os.Getenv("GOVARD_IMAGE_REPOSITORY"))
	if imageRepo == "" {
		imageRepo = conventions.DefaultImageRepoPrefix
	}

	images := make([]string, 0, 8)
	push := func(image string) {
		image = strings.TrimSpace(image)
		if image != "" {
			images = append(images, image)
		}
	}

	if FrameworkUsesNodeRuntime(config.Framework) {
		if config.Framework == "emdash" || config.Framework == "nextjs" {
			push(fmt.Sprintf("node:%s", config.Stack.NodeVersion))
		} else {
			push(fmt.Sprintf("node:%s-alpine", config.Stack.NodeVersion))
		}
	} else if FrameworkUsesPythonRuntime(config.Framework) {
		push(fmt.Sprintf("python:%s-slim", config.Stack.PythonVersion))
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
		switch config.Framework {
		case "magento2", "mageos":
			push(fmt.Sprintf("%sphp-magento2:%s", imageRepo, config.Stack.PHPVersion))
		case "magento1", "openmage":
			push(fmt.Sprintf("%sphp-magento1:%s", imageRepo, config.Stack.PHPVersion))
		default:
			push(fmt.Sprintf("%sphp:%s", imageRepo, config.Stack.PHPVersion))
		}
	}

	if config.Stack.Services.DB != "" && config.Stack.Services.DB != "none" && !FrameworkUsesNodeRuntime(config.Framework) {
		push(fmt.Sprintf("%s%s:%s", imageRepo, config.Stack.Services.DB, config.Stack.DBVersion))
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
