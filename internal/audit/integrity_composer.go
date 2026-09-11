package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// composerIntegrityAnalyzer checks the project manifest against its lock file and
// the configured stack runtime. It needs neither PHP nor a container.
type composerIntegrityAnalyzer struct{}

func init() { RegisterIntegrityAnalyzer(composerIntegrityAnalyzer{}) }

func (composerIntegrityAnalyzer) ID() string { return "composer" }

const integrityToolName = "govard-integrity"

type composerManifest struct {
	Name       string            `json:"name"`
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

type composerLockedPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type composerLockFile struct {
	ContentHash string                  `json:"content-hash"`
	Packages    []composerLockedPackage `json:"packages"`
	PackagesDev []composerLockedPackage `json:"packages-dev"`
}

// platformRequirements are satisfied by the runtime itself, never by a locked
// package, so they are excluded from the lock consistency check.
var composerPlatformRequirement = regexp.MustCompile(`^(php(-64bit)?|hhvm|ext-[a-z0-9_-]+|lib-[a-z0-9_-]+|composer-(plugin|runtime)-api)$`)

var composerPHPVersionToken = regexp.MustCompile(`(\d+)\.\d+`)

func (composerIntegrityAnalyzer) Analyze(request IntegrityRequest) ([]LintFinding, error) {
	root := strings.TrimSpace(request.TargetPath)
	if root == "" {
		root = request.ProjectRoot
	}
	if root == "" {
		return nil, nil
	}

	manifestPath := filepath.Join(root, "composer.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // not a Composer project: nothing to check
		}
		return nil, fmt.Errorf("read composer.json: %w", err)
	}

	var manifest composerManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return []LintFinding{integrityFinding("COMPOSER_JSON_INVALID", "composer.json",
			fmt.Sprintf("composer.json is not valid JSON: %v", err))}, nil
	}

	findings := []LintFinding{}

	lockPath := filepath.Join(root, "composer.lock")
	lockRaw, lockErr := os.ReadFile(lockPath)
	if lockErr != nil {
		if os.IsNotExist(lockErr) {
			findings = append(findings, integrityFinding("COMPOSER_LOCK_MISSING", "composer.lock",
				"composer.json is present but composer.lock is missing; dependencies are not pinned"))
			return findings, nil
		}
		return nil, fmt.Errorf("read composer.lock: %w", lockErr)
	}

	var lock composerLockFile
	if err := json.Unmarshal(lockRaw, &lock); err != nil {
		findings = append(findings, integrityFinding("COMPOSER_LOCK_INVALID", "composer.lock",
			fmt.Sprintf("composer.lock is not valid JSON: %v", err)))
		return findings, nil
	}

	locked := map[string]string{}
	duplicates := map[string]bool{}
	for _, pkg := range append(append([]composerLockedPackage{}, lock.Packages...), lock.PackagesDev...) {
		name := strings.TrimSpace(strings.ToLower(pkg.Name))
		if name == "" {
			continue
		}
		if _, seen := locked[name]; seen {
			duplicates[name] = true
			continue
		}
		locked[name] = pkg.Version
	}
	for _, name := range sortedKeys(duplicates) {
		findings = append(findings, integrityFinding("COMPOSER_DUPLICATE_PACKAGE", "composer.lock",
			fmt.Sprintf("package %q is locked more than once", name)))
	}

	for _, requirement := range append(requiredNames(manifest.Require), requiredNames(manifest.RequireDev)...) {
		if composerPlatformRequirement.MatchString(requirement) {
			continue
		}
		if _, ok := locked[strings.ToLower(requirement)]; !ok {
			findings = append(findings, integrityFinding("COMPOSER_REQUIREMENT_NOT_LOCKED", "composer.lock",
				fmt.Sprintf("%q is required in composer.json but absent from composer.lock", requirement)))
		}
	}

	if finding, ok := composerPHPConstraintFinding(manifest.Require["php"], request.StackPHPVersion); ok {
		findings = append(findings, finding)
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Rule != findings[j].Rule {
			return findings[i].Rule < findings[j].Rule
		}
		return findings[i].Message < findings[j].Message
	})
	return findings, nil
}

// composerPHPConstraintFinding compares the manifest's PHP constraint against the
// configured stack runtime at major-version granularity. That level is
// deliberately conservative: composer constraint semantics (^, ~, ranges) cannot
// be reproduced exactly without the PHP implementation, and a false positive
// here would be worse than a missed warning.
func composerPHPConstraintFinding(constraint string, stackPHP string) (LintFinding, bool) {
	constraint = strings.TrimSpace(constraint)
	stackPHP = strings.TrimSpace(stackPHP)
	if constraint == "" || stackPHP == "" {
		return LintFinding{}, false
	}
	stackMajor := majorVersion(stackPHP)
	if stackMajor < 0 {
		return LintFinding{}, false
	}
	majors := map[int]bool{}
	for _, match := range composerPHPVersionToken.FindAllStringSubmatch(constraint, -1) {
		if major, err := strconv.Atoi(match[1]); err == nil {
			majors[major] = true
		}
	}
	if len(majors) == 0 || majors[stackMajor] {
		return LintFinding{}, false
	}
	allowed := make([]string, 0, len(majors))
	for major := range majors {
		allowed = append(allowed, strconv.Itoa(major))
	}
	sort.Strings(allowed)
	return integrityFinding("COMPOSER_PHP_CONSTRAINT_MISMATCH", "composer.json",
		fmt.Sprintf("composer.json requires PHP %s but the project stack runs PHP %s",
			strings.Join(allowed, "/"), stackPHP)), true
}

func majorVersion(version string) int {
	match := composerPHPVersionToken.FindStringSubmatch(version)
	if match == nil {
		return -1
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return major
}

func requiredNames(requirements map[string]string) []string {
	names := make([]string, 0, len(requirements))
	for name := range requirements {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func integrityFinding(rule string, path string, message string) LintFinding {
	return LintFinding{Tool: integrityToolName, Rule: rule, Path: path, Message: message}
}
