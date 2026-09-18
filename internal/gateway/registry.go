package gateway

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/engine"
)

// reservedUsers can never name a gateway target: the gateway's username
// namespace is shared with the container's own accounts, and a project could
// otherwise be named the same as one of them.
var reservedUsers = map[string]bool{
	"root":   true,
	"admin":  true,
	"govard": true,
	"sshd":   true,
	"nobody": true,
}

// allowedKeyTypes is the key-type allowlist AllowKey enforces: a typo'd
// type would store fine and fail later at sshd, so reject it at allow time
// with an error that names the type.
var allowedKeyTypes = map[string]bool{
	"ssh-rsa":                            true,
	"ssh-dss":                            true,
	"ecdsa-sha2-nistp256":                true,
	"ecdsa-sha2-nistp384":                true,
	"ecdsa-sha2-nistp521":                true,
	"ssh-ed25519":                        true,
	"sk-ssh-ed25519@openssh.com":         true,
	"sk-ecdsa-sha2-nistp256@openssh.com": true,
}

// Target is one routed username: which container the second hop lands on,
// and which user it logs into there.
type Target struct {
	Container  string `json:"container"`
	TargetUser string `json:"target_user"`
	Project    string `json:"project"`
}

// AllowedKey is one allowlisted client public key. KeyType and KeyData hold
// the material verbatim (as it appears in an authorized_keys line) because
// the sshd image's AuthorizedKeysCommand must re-emit the exact key line for
// every request; the fingerprint alone (what a human reads) cannot be
// reversed back into it.
type AllowedKey struct {
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"key_type"`
	KeyData     string `json:"key_data"`
	Comment     string `json:"comment"`
	Added       string `json:"added"`
}

// Registry is the gateway's whole state: who it routes to, and whose keys it
// accepts. It is loaded, mutated in memory, and written back with Save --
// there is no partial/incremental writer.
type Registry struct {
	Targets   map[string]Target `json:"targets"`
	Allowlist []AllowedKey      `json:"allowlist"`
}

// GatewayDir is where the registry and its rendered flat files live. It is
// 0755, not 0700: it is bind-mounted read-only into the sshd container, whose
// AuthorizedKeysCommandUser is an unprivileged account that must be able to
// traverse into it (see RenderFlatFiles for the files' own permissions).
func GatewayDir() string {
	return filepath.Join(engine.GovardHomeDir(), "gateway")
}

func registryPath() string {
	return filepath.Join(GatewayDir(), "registry.json")
}

// Load reads the registry, returning an empty one if it has never been
// written -- there is nothing to allow or route on a fresh machine.
func Load() (Registry, error) {
	data, err := os.ReadFile(registryPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Targets: map[string]Target{}}, nil
		}
		return Registry{}, fmt.Errorf("read %s: %w", registryPath(), err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return Registry{}, fmt.Errorf("parse %s: %w", registryPath(), err)
	}
	if reg.Targets == nil {
		reg.Targets = map[string]Target{}
	}
	return reg, nil
}

// Save persists the registry (host-only, 0600 -- it is never mounted into the
// container) and re-renders the flat files the sshd image's shell scripts
// read (see RenderFlatFiles). It is the only writer, so every mutation lands
// in both places or neither.
func (r *Registry) Save() error {
	if r.Targets == nil {
		r.Targets = map[string]Target{}
	}
	if err := os.MkdirAll(GatewayDir(), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", GatewayDir(), err)
	}
	// MkdirAll only applies the mode at creation and masks it with umask,
	// so enforce the intended mode on every save: the gateway container's
	// unprivileged AuthorizedKeysCommandUser must traverse this dir.
	if err := os.Chmod(GatewayDir(), 0o755); err != nil {
		return fmt.Errorf("chmod %s: %w", GatewayDir(), err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(GatewayDir(), "registry.json", data, 0o600); err != nil {
		return err
	}
	return r.RenderFlatFiles()
}

// writeFileAtomic writes data to dir/name via a temp file in the same
// directory plus rename, so a crash mid-write never leaves a truncated file
// where the bastion's shell scripts read. The mode is enforced with Chmod
// after the rename: the temp file's creation mode must not leak through,
// and a pre-existing file's stale mode must not survive the replace.
func writeFileAtomic(dir, name string, data []byte, mode os.FileMode) error {
	path := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup; a successful rename removes tmpName anyway.
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

// RenderFlatFiles writes the two files the gateway image's shell scripts
// parse with `grep -F`/`awk` (no JSON parser in that stock-openssh image):
//
//   - targets: "<user> <container> <target-user>", one per line, read by
//     gw-router.sh to resolve where a session goes.
//   - keys: "<fingerprint> <keytype> <keydata> <comment>", one per line,
//     read by authorized-keys-command.sh to emit real authorized_keys
//     lines. It doubles as the human-readable allowlist audit trail: every
//     field ssh-keygen -lf would show is present, plus the material sshd
//     itself needs.
//
// Both files are 0644: they hold only public routing metadata and public key
// material, and the container's AuthorizedKeysCommandUser is not root.
func (r *Registry) RenderFlatFiles() error {
	// Direct callers must not fail on a missing dir: every other path goes
	// through Save, but RenderFlatFiles is exported, so ensure it here.
	if err := os.MkdirAll(GatewayDir(), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", GatewayDir(), err)
	}
	if err := os.Chmod(GatewayDir(), 0o755); err != nil {
		return fmt.Errorf("chmod %s: %w", GatewayDir(), err)
	}
	users := make([]string, 0, len(r.Targets))
	for user := range r.Targets {
		users = append(users, user)
	}
	sort.Strings(users)

	var targets strings.Builder
	for _, user := range users {
		t := r.Targets[user]
		fmt.Fprintf(&targets, "%s %s %s\n", user, t.Container, t.TargetUser)
	}
	if err := writeFileAtomic(GatewayDir(), "targets", []byte(targets.String()), 0o644); err != nil {
		return fmt.Errorf("render targets: %w", err)
	}

	// Sort the allowlist by fingerprint: insertion order is an accident of
	// when keys were allowed, and the rendered file should be deterministic.
	ordered := make([]AllowedKey, len(r.Allowlist))
	copy(ordered, r.Allowlist)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Fingerprint < ordered[j].Fingerprint })
	var keys strings.Builder
	for _, key := range ordered {
		fmt.Fprintf(&keys, "%s %s %s %s\n", key.Fingerprint, key.KeyType, key.KeyData, key.Comment)
	}
	if err := writeFileAtomic(GatewayDir(), "keys", []byte(keys.String()), 0o644); err != nil {
		return fmt.Errorf("render keys: %w", err)
	}
	return nil
}

// AddTarget registers (or re-registers) one username's route. Re-adding the
// same user for the same container is the idempotent case a repeated
// `sandbox up` takes; a different container under the same username is a
// naming collision the operator must resolve, not something to silently
// overwrite (the previous project would go dark with no diagnostic).
func (r *Registry) AddTarget(user string, t Target) error {
	if reservedUsers[strings.ToLower(user)] {
		return fmt.Errorf("%q is a reserved gateway username and cannot be a target", user)
	}
	if user == "" {
		return fmt.Errorf("a gateway target needs a non-empty username")
	}
	if existing, ok := r.Targets[user]; ok && existing.Container != t.Container {
		return fmt.Errorf("gateway user %q is already routed to container %q (wanted %q)", user, existing.Container, t.Container)
	}
	if r.Targets == nil {
		r.Targets = map[string]Target{}
	}
	r.Targets[user] = t
	return nil
}

// RemoveTarget prunes a username's route. Removing one that is not present
// is a no-op: `sandbox down` on a target the gateway never learned about
// (an old sandbox, predating this feature) has nothing to prune. The
// dynamically-created system account for that username (see
// docker/sshd/entrypoint.sh) is left in place; it is inert once its route is
// gone, because gw-router.sh refuses any username absent from `targets`.
func (r *Registry) RemoveTarget(user string) {
	delete(r.Targets, user)
}

// AllowKey adds one client public key to the allowlist and returns its
// fingerprint. Re-allowing an already-present key (matched by fingerprint)
// updates its comment and timestamp rather than duplicating the entry.
func (r *Registry) AllowKey(pubLine string) (string, error) {
	// The keys flat file is line-oriented (one key per line for the sshd
	// scripts to parse), so a comment carrying a newline would corrupt it.
	// Check the raw line: field-splitting below would normalize an embedded
	// newline away before the joined comment is ever inspected.
	if strings.ContainsAny(pubLine, "\n\r") {
		return "", fmt.Errorf("key comment must not contain a newline")
	}
	fields := strings.Fields(strings.TrimSpace(pubLine))
	if len(fields) < 2 {
		return "", fmt.Errorf("not a public key line: %q", pubLine)
	}
	keyType, keyData := fields[0], fields[1]
	comment := strings.Join(fields[2:], " ")

	if !allowedKeyTypes[keyType] {
		return "", fmt.Errorf("unsupported key type %q (want one of ssh-rsa, ssh-dss, ecdsa-sha2-nistp256, ecdsa-sha2-nistp384, ecdsa-sha2-nistp521, ssh-ed25519, sk-ssh-ed25519@openssh.com, sk-ecdsa-sha2-nistp256@openssh.com)", keyType)
	}

	fingerprint, err := Fingerprint(keyData)
	if err != nil {
		return "", err
	}

	for i, existing := range r.Allowlist {
		if existing.Fingerprint == fingerprint {
			r.Allowlist[i].Comment = comment
			r.Allowlist[i].Added = time.Now().UTC().Format(time.RFC3339)
			return fingerprint, nil
		}
	}
	r.Allowlist = append(r.Allowlist, AllowedKey{
		Fingerprint: fingerprint,
		KeyType:     keyType,
		KeyData:     keyData,
		Comment:     comment,
		Added:       time.Now().UTC().Format(time.RFC3339),
	})
	return fingerprint, nil
}

// RevokeKey removes every allowlisted key matched by exact fingerprint or
// exact comment. Matching nothing is an error: a revoke is a deliberate
// access cut, and a silent no-op would look like it worked.
func (r *Registry) RevokeKey(fingerprintOrComment string) error {
	kept := make([]AllowedKey, 0, len(r.Allowlist))
	removed := 0
	for _, existing := range r.Allowlist {
		if existing.Fingerprint == fingerprintOrComment || existing.Comment == fingerprintOrComment {
			removed++
			continue
		}
		kept = append(kept, existing)
	}
	if removed == 0 {
		return fmt.Errorf("no allowlisted key matches %q", fingerprintOrComment)
	}
	r.Allowlist = kept
	return nil
}

// Fingerprint renders the SHA256:<base64> form ssh-keygen -lf prints for a
// key's base64 field: unpadded standard base64 of the SHA-256 digest of the
// decoded key blob (RFC 4648 raw encoding -- StdEncoding's trailing "=" would
// never match real ssh-keygen output).
func Fingerprint(keyDataBase64 string) (string, error) {
	blob, err := base64.StdEncoding.DecodeString(keyDataBase64)
	if err != nil {
		return "", fmt.Errorf("decode key material: %w", err)
	}
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]), nil
}
