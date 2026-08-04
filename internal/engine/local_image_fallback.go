package engine

import (
	"fmt"
	"govard/internal/conventions"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	dockerassets "govard/docker"
)

const (
	defaultGovardImageRepository = conventions.DefaultImageRepoPrefix
)

var (
	embeddedDockerAssetsOnce sync.Once
	embeddedDockerAssetsDir  string
	embeddedDockerAssetsErr  error
	majorMinorVersionPattern = regexp.MustCompile(`^\d+\.\d+$`)
)

type localImageBuildArg struct {
	Name  string
	Value string
}

type localImageBuildSpec struct {
	ContextRel    string
	DockerfileRel string
	BuildArgs     []localImageBuildArg
	Dependencies  []string
}

type localImageInspection struct {
	Architecture string
}

type localImageBuildOptions struct {
	BuildMissing      bool
	BuildIncompatible bool
}

// LocalBuildSpecForTest exposes resolved local build spec details for tests.
type LocalBuildSpecForTest struct {
	ContextRel    string
	DockerfileRel string
	BuildArgs     map[string]string
	Dependencies  []string
}

// FallbackBuildMissingGovardImagesFromCompose builds Govard-managed images locally if
// pulling fails or if an existing local Govard image should be rebuilt for the host.
func FallbackBuildMissingGovardImagesFromCompose(composePath string, out io.Writer, errOut io.Writer) ([]string, error) {
	return buildGovardImagesFromCompose(composePath, out, errOut, localImageBuildOptions{
		BuildMissing:      true,
		BuildIncompatible: true,
	})
}

// RebuildIncompatibleGovardImagesFromCompose rebuilds existing Govard-managed
// images that are incompatible with the current host architecture.
func RebuildIncompatibleGovardImagesFromCompose(composePath string, out io.Writer, errOut io.Writer) ([]string, error) {
	return buildGovardImagesFromCompose(composePath, out, errOut, localImageBuildOptions{
		BuildIncompatible: true,
	})
}

func buildGovardImagesFromCompose(composePath string, out io.Writer, errOut io.Writer, options localImageBuildOptions) ([]string, error) {
	out = normalizeWriter(out)
	errOut = normalizeWriter(errOut)

	serviceImages, err := ReadServiceImagesFromCompose(composePath)
	if err != nil {
		return nil, fmt.Errorf("read compose service images: %w", err)
	}

	uniqueImages := make([]string, 0, len(serviceImages))
	seen := make(map[string]struct{}, len(serviceImages))
	for _, image := range serviceImages {
		trimmed := strings.TrimSpace(image)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		uniqueImages = append(uniqueImages, trimmed)
	}

	queue := append([]string{}, uniqueImages...)
	buildSpecs := make(map[string]localImageBuildSpec)
	nonBuildableMissing := make([]string, 0)
	resolved := make(map[string]struct{})
	dependencyImages := make(map[string]struct{})

	for len(queue) > 0 {
		image := queue[0]
		queue = queue[1:]

		if _, exists := resolved[image]; exists {
			continue
		}

		inspection, inspectErr := inspectLocalImage(image)
		imageExists := inspectErr == nil

		repoPrefix, service, tag, ok := parseGovardImageReference(image)
		if !ok {
			_, isDependency := dependencyImages[image]
			if (options.BuildMissing || isDependency) && !imageExists {
				nonBuildableMissing = append(nonBuildableMissing, image)
			}
			resolved[image] = struct{}{}
			continue
		}

		_, isDependency := dependencyImages[image]
		if imageExists {
			if !options.BuildIncompatible || !shouldRebuildGovardImageForHost(inspection.Architecture) {
				resolved[image] = struct{}{}
				continue
			}
			fmt.Fprintf(
				errOut,
				"WARNING: rebuilding %s locally because the existing image architecture is %s on %s/%s\n",
				image,
				inspection.Architecture,
				runtime.GOOS,
				runtime.GOARCH,
			)
		} else if !options.BuildMissing && !isDependency {
			resolved[image] = struct{}{}
			continue
		}

		spec, specErr := localBuildSpecForGovardService(service, tag, repoPrefix)
		if specErr != nil {
			nonBuildableMissing = append(nonBuildableMissing, image)
			resolved[image] = struct{}{}
			continue
		}

		buildSpecs[image] = spec
		for _, dependency := range spec.Dependencies {
			dependencyImages[dependency] = struct{}{}
			queue = append(queue, dependency)
		}
		resolved[image] = struct{}{}
	}

	if len(buildSpecs) == 0 {
		return nil, nil
	}

	dockerRoot, err := ensureDockerAssetsRoot(".")
	if err != nil {
		return nil, fmt.Errorf("resolve docker build contexts: %w", err)
	}

	// Simple greedy build: keep building images whose dependencies are met until none left or no progress
	built := make([]string, 0, len(buildSpecs))
	remaining := make(map[string]localImageBuildSpec)
	for k, v := range buildSpecs {
		remaining[k] = v
	}

	for len(remaining) > 0 {
		progress := false
		for image, spec := range remaining {
			depsMet := true
			for _, dep := range spec.Dependencies {
				if _, isMissing := remaining[dep]; isMissing {
					depsMet = false
					break
				}
			}

			if depsMet {
				if err := buildGovardImageLocally(image, dockerRoot, out, errOut); err != nil {
					return built, err
				}
				built = append(built, image)
				delete(remaining, image)
				progress = true
				break // Start over to check for newly unlocked dependencies
			}
		}

		if !progress {
			// Circular dependency or unbuildable dependency
			unmetStrings := make([]string, 0, len(remaining))
			for image := range remaining {
				unmetStrings = append(unmetStrings, image)
			}
			return built, fmt.Errorf("failed to resolve build order for images: %s", strings.Join(unmetStrings, ", "))
		}
	}

	if len(nonBuildableMissing) > 0 {
		if len(built) == 0 {
			return nil, fmt.Errorf(
				"missing images are not eligible for Govard local fallback build: %s",
				strings.Join(nonBuildableMissing, ", "),
			)
		}
		fmt.Fprintf(errOut, "WARNING: missing images are not eligible for Govard local fallback build: %s\n", strings.Join(nonBuildableMissing, ", "))
	}

	return built, nil
}

func buildGovardImageLocally(image string, dockerRoot string, out io.Writer, errOut io.Writer) error {
	repoPrefix, service, tag, ok := parseGovardImageReference(image)
	if !ok {
		return fmt.Errorf("image %q is not a Govard-managed image", image)
	}

	spec, err := localBuildSpecForGovardService(service, tag, repoPrefix)
	if err != nil {
		return fmt.Errorf("local build spec for %s: %w", image, err)
	}

	contextPath := filepath.Join(dockerRoot, spec.ContextRel)
	if stat, statErr := os.Stat(contextPath); statErr != nil || !stat.IsDir() {
		if statErr != nil {
			return fmt.Errorf("build context %s: %w", contextPath, statErr)
		}
		return fmt.Errorf("build context %s is not a directory", contextPath)
	}

	args := []string{"build", "-t", image}
	if spec.DockerfileRel != "" {
		dockerfilePath := filepath.Join(dockerRoot, spec.DockerfileRel)
		if _, statErr := os.Stat(dockerfilePath); statErr != nil {
			return fmt.Errorf("dockerfile %s: %w", dockerfilePath, statErr)
		}
		args = append(args, "-f", dockerfilePath)
	}
	for _, buildArg := range spec.BuildArgs {
		args = append(args, "--build-arg", buildArg.Name+"="+buildArg.Value)
	}
	args = append(args, contextPath)

	command := exec.Command("docker", args...)
	command.Stdout = normalizeWriter(out)
	command.Stderr = normalizeWriter(errOut)
	if err := command.Run(); err != nil {
		return fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func localBuildSpecForGovardService(service string, tag string, repoPrefix string) (localImageBuildSpec, error) {
	service = strings.TrimSpace(service)
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = "latest"
	}
	if strings.TrimSpace(repoPrefix) == "" {
		repoPrefix = defaultGovardImageRepository
	}

	baseTag, isDebug, xdebugVersionOverride := splitDebugTag(tag)

	switch service {
	case "apache":
		return localImageBuildSpec{
			ContextRel: "apache",
			BuildArgs: []localImageBuildArg{
				{Name: "APACHE_VERSION", Value: resolveApacheBuildVersion(tag)},
			},
		}, nil
	case "nginx":
		return localImageBuildSpec{
			ContextRel: "nginx",
			BuildArgs: []localImageBuildArg{
				{Name: "NGINX_VERSION", Value: resolveNginxBuildVersion(tag)},
			},
		}, nil
	case "php":
		if isDebug {
			return localImageBuildSpec{
				ContextRel:    "php",
				DockerfileRel: filepath.Join("php", "debug", "Dockerfile"),
				BuildArgs:     debugBuildArgs(repoPrefix+"php:"+baseTag, xdebugVersionOverride),
				Dependencies:  []string{repoPrefix + "php:" + baseTag},
			}, nil
		}
		return localImageBuildSpec{
			ContextRel: "php",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvPHPVersion, Value: tag},
			},
		}, nil
	case "mariadb":
		return localImageBuildSpec{
			ContextRel: "mariadb",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvMariaDBVersion, Value: tag},
			},
		}, nil
	case "mysql":
		return localImageBuildSpec{
			ContextRel: "mysql",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvMySQLVersion, Value: tag},
			},
		}, nil
	case "redis":
		return localImageBuildSpec{
			ContextRel: "redis",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvRedisVersion, Value: tag},
			},
		}, nil
	case "valkey":
		return localImageBuildSpec{
			ContextRel: "valkey",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvValkeyVersion, Value: tag},
			},
		}, nil
	case "rabbitmq":
		return localImageBuildSpec{
			ContextRel: "rabbitmq",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvRabbitMQVersion, Value: tag},
			},
		}, nil
	case "opensearch":
		actualVersion := resolveOpensearchBuildVersion(tag)
		return localImageBuildSpec{
			ContextRel: "opensearch",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvOpenSearchVersion, Value: actualVersion},
			},
		}, nil
	case "elasticsearch":
		elasticsearchImage := "docker.elastic.co/elasticsearch/elasticsearch"
		if tag == "2.4.6" {
			elasticsearchImage = "elasticsearch"
		}
		actualVersion := resolveElasticsearchBuildVersion(tag)
		return localImageBuildSpec{
			ContextRel: "elasticsearch",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvElasticsearchVersion, Value: actualVersion},
				{Name: "ELASTICSEARCH_IMAGE", Value: elasticsearchImage},
			},
		}, nil
	case "varnish":
		varnishVersion, varnishImageTag := resolveVarnishBuildVersions(tag)
		return localImageBuildSpec{
			ContextRel: "varnish",
			BuildArgs: []localImageBuildArg{
				{Name: conventions.EnvVarnishVersion, Value: varnishVersion},
				{Name: "VARNISH_IMAGE_TAG", Value: varnishImageTag},
			},
		}, nil
	case "dnsmasq":
		return localImageBuildSpec{
			ContextRel: "dnsmasq",
		}, nil
	default:
		if variant, ok := phpVariantFromServiceName(service); ok {
			serviceImage := "php-" + variant
			if isDebug {
				return localImageBuildSpec{
					ContextRel:    "php",
					DockerfileRel: filepath.Join("php", "debug", "Dockerfile"),
					BuildArgs:     debugBuildArgs(repoPrefix+serviceImage+":"+baseTag, xdebugVersionOverride),
					Dependencies:  []string{repoPrefix + serviceImage + ":" + baseTag},
				}, nil
			}
			return localImageBuildSpec{
				ContextRel:    "php",
				DockerfileRel: filepath.Join("php", variant, "Dockerfile"),
				BuildArgs: []localImageBuildArg{
					{Name: conventions.EnvPHPVersion, Value: tag},
					{Name: "GOVARD_IMAGE_REPOSITORY", Value: repoPrefix},
				},
				Dependencies: []string{repoPrefix + "php:" + tag},
			}, nil
		}
		return localImageBuildSpec{}, fmt.Errorf("unsupported Govard image service %q", service)
	}
}

// phpVariantFromServiceName reports whether service is a registered PHP
// image variant's service name (e.g. "php-magento1"), returning the bare
// variant ("magento1"). Lets this switch recognize any registered variant
// generically instead of one literal case per framework family.
func phpVariantFromServiceName(service string) (string, bool) {
	const prefix = "php-"
	if !strings.HasPrefix(service, prefix) {
		return "", false
	}
	variant := strings.TrimPrefix(service, prefix)
	for _, v := range registeredPHPImageVariants() {
		if v == variant {
			return variant, true
		}
	}
	return "", false
}

// splitDebugTag splits a Govard image tag into its base (non-debug) version,
// whether it targets the debug variant, and an optional Xdebug version
// override encoded as a "-xdebug-<version>" suffix (see base.yml, which
// appends this suffix when stack.xdebug_version is set).
func splitDebugTag(tag string) (baseTag string, isDebug bool, xdebugVersionOverride string) {
	const debugMarker = "-debug"
	const xdebugVersionMarker = "-xdebug-"

	idx := strings.Index(tag, debugMarker)
	if idx < 0 {
		return tag, false, ""
	}

	baseTag = tag[:idx]
	rest := tag[idx+len(debugMarker):]
	if strings.HasPrefix(rest, xdebugVersionMarker) {
		xdebugVersionOverride = strings.TrimPrefix(rest, xdebugVersionMarker)
	}
	return baseTag, true, xdebugVersionOverride
}

func debugBuildArgs(baseImage string, xdebugVersionOverride string) []localImageBuildArg {
	args := []localImageBuildArg{
		{Name: "BASE_IMAGE", Value: baseImage},
	}
	if xdebugVersionOverride != "" {
		args = append(args, localImageBuildArg{Name: "XDEBUG_VERSION_OVERRIDE", Value: xdebugVersionOverride})
	}
	return args
}

func parseGovardImageReference(image string) (repoPrefix string, service string, tag string, ok bool) {
	repository, version := splitImageRepositoryAndTag(image)
	if repository == "" {
		return "", "", "", false
	}

	markerIndex := strings.LastIndex(repository, "govard-")
	if markerIndex < 0 {
		return "", "", "", false
	}

	repoPrefix = repository[:markerIndex+len("govard-")]
	service = strings.TrimSpace(repository[markerIndex+len("govard-"):])
	if service == "" {
		return "", "", "", false
	}
	return repoPrefix, service, version, true
}

func splitImageRepositoryAndTag(image string) (repository string, tag string) {
	trimmed := strings.TrimSpace(image)
	if trimmed == "" {
		return "", ""
	}

	if atIndex := strings.Index(trimmed, "@"); atIndex >= 0 {
		trimmed = trimmed[:atIndex]
	}

	lastSlash := strings.LastIndex(trimmed, "/")
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon > lastSlash {
		return trimmed[:lastColon], strings.TrimSpace(trimmed[lastColon+1:])
	}
	return trimmed, "latest"
}

func resolveApacheBuildVersion(tag string) string {
	switch strings.TrimSpace(tag) {
	case "", "latest", "2.4":
		return "2.4.66"
	default:
		return tag
	}
}

func resolveNginxBuildVersion(tag string) string {
	tag = strings.TrimSpace(tag)
	switch tag {
	case "", "latest", "1.28":
		return "1.28.0"
	default:
		if majorMinorVersionPattern.MatchString(tag) {
			return tag + ".0"
		}
		return tag
	}
}

func resolveVarnishBuildVersions(tag string) (version string, imageTag string) {
	tag = strings.TrimSpace(tag)
	switch tag {
	case "", "latest":
		return "7.6", "7.6"
	case "6.0":
		return "6.0", "6.0"
	default:
		return tag, tag
	}
}

func resolveElasticsearchBuildVersion(tag string) string {
	tag = strings.TrimSpace(tag)
	switch tag {
	case "7.9":
		return "7.9.3"
	case "7.6":
		return "7.6.2"
	case "7.17":
		return "7.17.10"
	case "6.8":
		return "6.8.23"
	case "8.4":
		return "8.4.3"
	default:
		return tag
	}
}

func resolveOpensearchBuildVersion(tag string) string {
	tag = strings.TrimSpace(tag)
	switch tag {
	case "2.12":
		return "2.12.0"
	case "2.5":
		return "2.5.0"
	default:
		if majorMinorVersionPattern.MatchString(tag) {
			return tag + ".0"
		}
		return tag
	}
}

func inspectLocalImage(image string) (localImageInspection, error) {
	command := exec.Command("docker", "image", "inspect", image, "--format", "{{.Architecture}}")
	output, err := command.Output()
	if err != nil {
		return localImageInspection{}, err
	}
	return localImageInspection{
		Architecture: strings.TrimSpace(string(output)),
	}, nil
}

func shouldRebuildGovardImageForHost(imageArchitecture string) bool {
	return shouldRebuildGovardImageForHostWithRuntime(imageArchitecture, runtime.GOOS, runtime.GOARCH)
}

func shouldRebuildGovardImageForHostWithRuntime(imageArchitecture string, goos string, goarch string) bool {
	return strings.TrimSpace(goos) == "darwin" &&
		strings.TrimSpace(goarch) == "arm64" &&
		strings.TrimSpace(imageArchitecture) == "amd64"
}

func ensureDockerAssetsRoot(startDir string) (string, error) {
	if root, err := findDockerAssetsDir(startDir); err == nil {
		return root, nil
	}
	return ensureEmbeddedDockerAssetsDir()
}

func findDockerAssetsDir(startDir string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("GOVARD_DOCKER_DIR")); override != "" {
		if abs, err := filepath.Abs(override); err == nil && isDockerAssetsDir(abs) {
			return abs, nil
		}
	}

	absStart, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	current := absStart
	for {
		candidate := filepath.Join(current, "docker")
		if isDockerAssetsDir(candidate) {
			return candidate, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	for _, candidate := range []string{
		"/usr/local/share/govard/docker",
		"/usr/share/govard/docker",
	} {
		if isDockerAssetsDir(candidate) {
			return candidate, nil
		}
	}

	if executablePath, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executablePath)
		for _, candidate := range []string{
			filepath.Join(executableDir, "docker"),
			filepath.Join(executableDir, "..", "docker"),
		} {
			clean := filepath.Clean(candidate)
			if isDockerAssetsDir(clean) {
				return clean, nil
			}
		}
	}

	return "", fmt.Errorf("docker build assets directory not found")
}

func ensureEmbeddedDockerAssetsDir() (string, error) {
	embeddedDockerAssetsOnce.Do(func() {
		tempDir, err := os.MkdirTemp("", "govard-docker-assets-*")
		if err != nil {
			embeddedDockerAssetsErr = fmt.Errorf("create temp docker assets dir: %w", err)
			return
		}
		if err := materializeDockerAssetsFS(dockerassets.FS, tempDir); err != nil {
			embeddedDockerAssetsErr = fmt.Errorf("materialize embedded docker assets: %w", err)
			return
		}
		embeddedDockerAssetsDir = tempDir
	})
	if embeddedDockerAssetsErr != nil {
		return "", embeddedDockerAssetsErr
	}
	return embeddedDockerAssetsDir, nil
}

func materializeDockerAssetsFS(source fs.FS, destination string) error {
	return fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}

		targetPath := filepath.Join(destination, path)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, conventions.DefaultDirPerm)
		}

		content, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), conventions.DefaultDirPerm); err != nil {
			return err
		}
		return os.WriteFile(targetPath, content, conventions.DefaultFilePerm)
	})
}

func isDockerAssetsDir(root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	requiredPaths := []string{
		filepath.Join(root, "docker-bake.hcl"),
		filepath.Join(root, "php", "Dockerfile"),
		filepath.Join(root, "dnsmasq", "Dockerfile"),
	}
	for _, requiredPath := range requiredPaths {
		if _, err := os.Stat(requiredPath); err != nil {
			return false
		}
	}
	return true
}

func normalizeWriter(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

// ParseGovardImageReferenceForTest exposes Govard image parsing for tests.
func ParseGovardImageReferenceForTest(image string) (string, string, string, bool) {
	return parseGovardImageReference(image)
}

// ResolveLocalBuildSpecForTest exposes local build spec resolution for tests.
func ResolveLocalBuildSpecForTest(service string, tag string, repositoryPrefix string) (LocalBuildSpecForTest, error) {
	spec, err := localBuildSpecForGovardService(service, tag, repositoryPrefix)
	if err != nil {
		return LocalBuildSpecForTest{}, err
	}
	buildArgs := make(map[string]string, len(spec.BuildArgs))
	for _, buildArg := range spec.BuildArgs {
		buildArgs[buildArg.Name] = buildArg.Value
	}
	return LocalBuildSpecForTest{
		ContextRel:    spec.ContextRel,
		DockerfileRel: spec.DockerfileRel,
		BuildArgs:     buildArgs,
		Dependencies:  spec.Dependencies,
	}, nil
}

// ShouldRebuildGovardImageForHostForTest exposes host/image architecture policy for tests.
func ShouldRebuildGovardImageForHostForTest(imageArchitecture string, goos string, goarch string) bool {
	return shouldRebuildGovardImageForHostWithRuntime(imageArchitecture, goos, goarch)
}
