package audit

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// magentoModuleDIAnalyzer checks Magento module registration, dependency
// injection wiring, and that the route definitions a module ships are
// well formed. It parses XML and PHP registration files directly, so it runs
// without PHP, Composer, or a container.
type magentoModuleDIAnalyzer struct{}

func init() { RegisterIntegrityAnalyzer(magentoModuleDIAnalyzer{}) }

func (magentoModuleDIAnalyzer) ID() string { return "magento-module-di" }

// Directories that never carry first-party module sources.
var magentoScanSkipDirs = map[string]bool{
	"vendor": true, "generated": true, "var": true, "node_modules": true,
	".git": true, "pub": true, "setup": true,
}

var magentoRegistrationComponent = regexp.MustCompile(`(?i)ComponentRegistrar::MODULE\s*,\s*['"]([A-Za-z0-9_]+)['"]`)

// magentoModuleXML mirrors etc/module.xml: the document root is <config> and the
// module identity lives on the <module> child.
type magentoModuleXML struct {
	XMLName xml.Name `xml:"config"`
	Module  struct {
		Name     string `xml:"name,attr"`
		Sequence []struct {
			Name string `xml:"name,attr"`
		} `xml:"sequence>module"`
	} `xml:"module"`
}

type magentoConfigXML struct {
	XMLName     xml.Name `xml:"config"`
	Preferences []struct {
		For string `xml:"for,attr"`
	} `xml:"preference"`
	Types []struct {
		Name string `xml:"name,attr"`
	} `xml:"type"`
	VirtualTypes []struct {
		Name string `xml:"name,attr"`
	} `xml:"virtualType"`
}

type magentoModuleFile struct {
	moduleXMLPath string
	registration  string
	name          string
	sequence      []string
}

func (magentoModuleDIAnalyzer) Analyze(request IntegrityRequest) ([]LintFinding, error) {
	root := strings.TrimSpace(request.TargetPath)
	if root == "" {
		root = request.ProjectRoot
	}
	if root == "" {
		return nil, nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	findings := []LintFinding{}
	modules := []magentoModuleFile{}
	diFiles := []string{}
	routeFiles := []string{}

	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtrees are reported by the walk root, not here
		}
		if entry.IsDir() {
			if path != root && magentoScanSkipDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		base := strings.ToLower(entry.Name())
		if base != "module.xml" && base != "di.xml" && base != "webapi.xml" && base != "routes.xml" {
			return nil
		}
		if strings.ToLower(filepath.Base(filepath.Dir(path))) != "etc" {
			return nil
		}
		switch base {
		case "module.xml":
			modules = append(modules, magentoModuleFile{moduleXMLPath: path})
		case "di.xml":
			diFiles = append(diFiles, path)
		case "webapi.xml", "routes.xml":
			// Both are route definitions: a broken one silently disables an
			// endpoint or a frontName, and neither carries identity or wiring
			// this analyzer could reason about beyond well-formedness.
			routeFiles = append(routeFiles, path)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan Magento modules: %w", walkErr)
	}
	if len(modules) == 0 && len(diFiles) == 0 && len(routeFiles) == 0 {
		return nil, nil // not a Magento module tree
	}

	sort.Slice(modules, func(i, j int) bool { return modules[i].moduleXMLPath < modules[j].moduleXMLPath })
	sort.Strings(diFiles)
	sort.Strings(routeFiles)

	for index := range modules {
		module := &modules[index]
		raw, readErr := os.ReadFile(module.moduleXMLPath)
		relative := relativeTo(root, module.moduleXMLPath)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", relative, readErr)
		}
		var parsed magentoModuleXML
		if err := xml.Unmarshal(raw, &parsed); err != nil {
			findings = append(findings, integrityFinding("MAGENTO_XML_INVALID", relative,
				fmt.Sprintf("%s is not valid XML: %v", filepath.Base(module.moduleXMLPath), err)))
			continue
		}
		module.name = strings.TrimSpace(parsed.Module.Name)
		for _, entry := range parsed.Module.Sequence {
			if name := strings.TrimSpace(entry.Name); name != "" {
				module.sequence = append(module.sequence, name)
			}
		}
		module.registration = readRegistrationComponent(filepath.Join(filepath.Dir(filepath.Dir(module.moduleXMLPath)), "registration.php"))
	}

	for _, module := range modules {
		relative := relativeTo(root, module.moduleXMLPath)
		if module.registration != "" && module.name != "" && module.registration != module.name {
			findings = append(findings, integrityFinding("MAGENTO_MODULE_NAME_MISMATCH", relative,
				fmt.Sprintf("registration.php registers %q but module.xml declares %q", module.registration, module.name)))
		}
	}

	// The known-module set spans first-party modules and installed Composer
	// modules: a <sequence> entry routinely names a Magento core module that
	// lives under vendor/. Vendor code is read for names only — it is never the
	// subject of the DI or identity checks below.
	known := map[string]bool{}
	for _, module := range modules {
		if module.name != "" {
			known[module.name] = true
		}
	}
	for _, name := range vendorModuleNames(root) {
		known[name] = true
	}
	for _, module := range modules {
		for _, dependency := range module.sequence {
			if known[dependency] {
				continue
			}
			findings = append(findings, integrityFinding("MAGENTO_SEQUENCE_UNKNOWN_MODULE", relativeTo(root, module.moduleXMLPath),
				fmt.Sprintf("%s depends on module %q, which is not present in this tree", module.name, dependency)))
		}
	}

	for _, path := range diFiles {
		relative := relativeTo(root, path)
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", relative, readErr)
		}
		var parsed magentoConfigXML
		if err := xml.Unmarshal(raw, &parsed); err != nil {
			findings = append(findings, integrityFinding("MAGENTO_XML_INVALID", relative,
				fmt.Sprintf("%s is not valid XML: %v", filepath.Base(path), err)))
			continue
		}
		findings = append(findings, duplicateDITargets(relative, parsed)...)
	}

	for _, path := range routeFiles {
		relative := relativeTo(root, path)
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", relative, readErr)
		}
		if err := wellFormedXML(raw); err != nil {
			findings = append(findings, integrityFinding("MAGENTO_XML_INVALID", relative,
				fmt.Sprintf("%s is not valid XML: %v", filepath.Base(path), err)))
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Rule != findings[j].Rule {
			return findings[i].Rule < findings[j].Rule
		}
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Message < findings[j].Message
	})
	return findings, nil
}

// wellFormedXML parses one XML document and discards it.
//
// Well-formedness is the whole check for a route definition: its schema says
// nothing about module identity or DI wiring, and a file that cannot be parsed
// at all is a defect regardless of what it was meant to declare.
func wellFormedXML(raw []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		if _, err := decoder.Token(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func duplicateDITargets(relative string, config magentoConfigXML) []LintFinding {
	findings := []LintFinding{}
	preferences := map[string]int{}
	for _, preference := range config.Preferences {
		preferences[strings.TrimSpace(preference.For)]++
	}
	types := map[string]int{}
	for _, declared := range config.Types {
		types[strings.TrimSpace(declared.Name)]++
	}
	virtualTypes := map[string]int{}
	for _, declared := range config.VirtualTypes {
		virtualTypes[strings.TrimSpace(declared.Name)]++
	}
	duplicated := make([]string, 0, len(preferences))
	for target, count := range preferences {
		if target != "" && count > 1 {
			duplicated = append(duplicated, fmt.Sprintf("preference for %q is declared %d times", target, count))
		}
	}
	for target, count := range types {
		if target != "" && count > 1 {
			duplicated = append(duplicated, fmt.Sprintf("type %q is declared %d times", target, count))
		}
	}
	for target, count := range virtualTypes {
		if target != "" && count > 1 {
			duplicated = append(duplicated, fmt.Sprintf("virtualType %q is declared %d times", target, count))
		}
	}
	sort.Strings(duplicated)
	for _, message := range duplicated {
		findings = append(findings, integrityFinding("MAGENTO_DI_DUPLICATE_TARGET", relative, message))
	}
	return findings
}

func readRegistrationComponent(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	match := magentoRegistrationComponent.FindSubmatch(raw)
	if match == nil {
		return ""
	}
	return string(match[1])
}

func relativeTo(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

// vendorModuleNames reads module names declared by installed Composer packages.
// Both package layouts are covered explicitly — `etc/module.xml` at the package
// root and the `src/etc/module.xml` variant — instead of walking vendor/ deeply,
// which keeps the scan bounded and fast on a real Magento install. Vendor code
// is read for names only; it is never the subject of the checks themselves.
func vendorModuleNames(root string) []string {
	vendorRoot := filepath.Join(root, "vendor")
	if info, err := os.Stat(vendorRoot); err != nil || !info.IsDir() {
		return nil
	}
	names := []string{}
	for _, pattern := range []string{
		filepath.Join(vendorRoot, "*", "*", "etc", "module.xml"),
		filepath.Join(vendorRoot, "*", "*", "src", "etc", "module.xml"),
	} {
		matches, globErr := filepath.Glob(pattern)
		if globErr != nil {
			continue
		}
		for _, path := range matches {
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				continue
			}
			var parsed magentoModuleXML
			if xmlErr := xml.Unmarshal(raw, &parsed); xmlErr != nil {
				continue
			}
			if name := strings.TrimSpace(parsed.Module.Name); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}
