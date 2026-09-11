package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"

	"gopkg.in/yaml.v3"
)

// The sandbox remote is written to `.govard.local.yml`, which is local-only and
// gitignored: a sandbox is a fact about one developer's machine and must never
// be committed into a project's shared configuration.
//
// The write goes through a YAML node tree rather than through a decoded map. A
// project may keep its own local remotes — and its own comments — in that file,
// and re-marshalling a map would reformat all of it and drop every comment.

// WriteSandboxRemote adds (or replaces) one remote in the project's local
// configuration layer.
func WriteSandboxRemote(projectRoot, name string, remote engine.RemoteConfig) error {
	path := filepath.Join(projectRoot, conventions.LocalConfigFile)
	document, err := loadLocalConfigNode(path)
	if err != nil {
		return err
	}
	root := documentMapping(document)
	remotes := childMapping(root, "remotes", true)

	var encoded yaml.Node
	if err := encoded.Encode(remote); err != nil {
		return fmt.Errorf("encode the sandbox remote: %w", err)
	}
	setChild(remotes, name, &encoded)

	return writeLocalConfigNode(path, document)
}

// RemoveSandboxRemote deletes one remote from the local configuration layer.
// Removing a remote that is not there is not an error: `sandbox down` has to be
// repeatable.
func RemoveSandboxRemote(projectRoot, name string) error {
	path := filepath.Join(projectRoot, conventions.LocalConfigFile)
	document, err := loadLocalConfigNode(path)
	if err != nil {
		return err
	}
	root := documentMapping(document)
	remotes := childMapping(root, "remotes", false)
	if remotes == nil {
		return nil
	}
	if !removeChild(remotes, name) {
		return nil
	}
	// An empty `remotes:` key is noise in a file a human reads.
	if len(remotes.Content) == 0 {
		removeChild(root, "remotes")
	}
	if len(root.Content) == 0 {
		return os.Remove(path)
	}
	return writeLocalConfigNode(path, document)
}

// loadLocalConfigNode reads the file into a node tree, or returns an empty
// document when it does not exist yet.
func loadLocalConfigNode(path string) (*yaml.Node, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyDocument(), nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if document.Kind == 0 {
		return emptyDocument(), nil
	}
	return &document, nil
}

func emptyDocument() *yaml.Node {
	return &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
	}
}

// documentMapping returns the document's root mapping, refusing a file whose
// shape govard cannot safely edit.
func documentMapping(document *yaml.Node) *yaml.Node {
	if len(document.Content) == 0 {
		document.Kind = yaml.DocumentNode
		document.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := document.Content[0]
	if root.Kind == 0 {
		root.Kind = yaml.MappingNode
		root.Tag = "!!map"
	}
	return root
}

// childMapping finds a mapping-valued key, creating it when create is true.
func childMapping(parent *yaml.Node, key string, create bool) *yaml.Node {
	for idx := 0; idx+1 < len(parent.Content); idx += 2 {
		if parent.Content[idx].Value != key {
			continue
		}
		value := parent.Content[idx+1]
		if value.Kind == 0 {
			value.Kind = yaml.MappingNode
			value.Tag = "!!map"
		}
		return value
	}
	if !create {
		return nil
	}
	value := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	return value
}

// setChild inserts or replaces one key of a mapping, keeping its position.
func setChild(mapping *yaml.Node, key string, value *yaml.Node) {
	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		if mapping.Content[idx].Value == key {
			mapping.Content[idx+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

// removeChild deletes one key and reports whether it was there.
func removeChild(mapping *yaml.Node, key string) bool {
	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		if mapping.Content[idx].Value != key {
			continue
		}
		mapping.Content = append(mapping.Content[:idx], mapping.Content[idx+2:]...)
		return true
	}
	return false
}

// writeLocalConfigNode writes the tree back, atomically. The file is
// configuration a human may be editing by hand, so a half-written file would be
// worse than a failed command.
func writeLocalConfigNode(path string, document *yaml.Node) error {
	var buffer strings.Builder
	encoder := yaml.NewEncoder(&buffer)
	// Two spaces matches the rest of govard's configuration files: a sandbox
	// write must not make the project's own file look like a different file.
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	encoded := buffer.String()
	temporary := path + ".govard-tmp"
	if err := os.WriteFile(temporary, []byte(encoded), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", temporary, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
