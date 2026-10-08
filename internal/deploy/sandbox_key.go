package deploy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/gitguard"
)

// The sandbox authenticates with a dedicated key pair under `.govard/sandbox/`.
// That directory keeps itself out of version control: EnsureSandboxStateDir
// writes a `.gitignore` of `*` into it, because nothing in the project's own
// ignore rules covers it and a `git add -A` would otherwise commit the private
// key. The pair is generated in Go rather than by shelling out to
// `ssh-keygen`: `govard sandbox` declares `docker`, and ssh-keygen is not
// part of that contract, so depending on it would make the command fail on a
// host that meets its own stated requirement.
const (
	SandboxKeyName  = "id_ed25519"
	sandboxKeyType  = "ssh-ed25519"
	sandboxKeyNote  = "govard-sandbox"
	opensshKeyBegin = "-----BEGIN OPENSSH PRIVATE KEY-----"
	opensshKeyEnd   = "-----END OPENSSH PRIVATE KEY-----"
	opensshKeyMagic = "openssh-key-v1\x00"
)

// SandboxKeyPair is the generated identity.
type SandboxKeyPair struct {
	PrivatePath string
	PublicPath  string
	// PublicKey is the whole `ssh-ed25519 <base64> <comment>` line, which is
	// exactly what goes into authorized_keys.
	PublicKey string
}

// SandboxStateDir is where a project's sandbox keeps its key, its mirror and the
// rendered Dockerfile.
func SandboxStateDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".govard", "sandbox")
}

// SandboxMirrorPath is the bare mirror the container bind-mounts.
func SandboxMirrorPath(projectRoot string) string {
	return filepath.Join(SandboxStateDir(projectRoot), "repo.git")
}

// SandboxDockerfilePath is where the rendered Dockerfile is kept: the image tag
// is its hash, and a developer debugging a failed build needs to read it.
func SandboxDockerfilePath(projectRoot string) string {
	return filepath.Join(SandboxStateDir(projectRoot), "Dockerfile")
}

// EnsureSandboxStateDir creates the sandbox state directory (0700) and the
// self-ignoring `.gitignore` inside it (see gitguard.EnsureDir). The project is
// untrusted input: a state directory that is a symlink is refused.
func EnsureSandboxStateDir(dir string) error {
	if err := gitguard.EnsureDir(dir, 0o700); err != nil {
		return fmt.Errorf("sandbox state directory: %w", err)
	}
	return nil
}

// EnsureSandboxKey returns the project's key pair, generating it on first use.
//
// An existing pair is reused unchanged: the public key is installed in the
// container, and regenerating one would silently break every SSH connection to a
// sandbox that is already running.
func EnsureSandboxKey(dir string) (SandboxKeyPair, error) {
	pair := SandboxKeyPair{
		PrivatePath: filepath.Join(dir, SandboxKeyName),
		PublicPath:  filepath.Join(dir, SandboxKeyName+".pub"),
	}

	if err := EnsureSandboxStateDir(dir); err != nil {
		return SandboxKeyPair{}, err
	}

	if existing, err := readSandboxPublicKey(pair.PublicPath); err == nil {
		pair.PublicKey = existing
		// A private key that was loosened by hand or by a copy is tightened
		// again: it is the one file here that must never be group-readable.
		// Lstat, not Stat: chmod follows symlinks, and a link planted at the
		// key path would change the mode of whatever it points at.
		if info, statErr := os.Lstat(pair.PrivatePath); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return SandboxKeyPair{}, fmt.Errorf("refusing to use %s: the sandbox private key is a symlink", pair.PrivatePath)
			}
			if info.Mode().Perm() != 0o600 {
				_ = os.Chmod(pair.PrivatePath, 0o600)
			}
		}
		return pair, nil
	} else if !os.IsNotExist(err) {
		return SandboxKeyPair{}, err
	}

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return SandboxKeyPair{}, fmt.Errorf("generate the sandbox key: %w", err)
	}

	pair.PublicKey = sandboxPublicKeyLine(public)
	privateFile, err := encodeOpenSSHPrivateKey(private, sandboxKeyNote)
	if err != nil {
		return SandboxKeyPair{}, err
	}
	if err := createNewFile(pair.PrivatePath, privateFile, 0o600); err != nil {
		return SandboxKeyPair{}, fmt.Errorf("write the sandbox private key: %w", err)
	}
	if err := createNewFile(pair.PublicPath, []byte(pair.PublicKey+"\n"), 0o644); err != nil {
		return SandboxKeyPair{}, fmt.Errorf("write the sandbox public key: %w", err)
	}
	return pair, nil
}

// createNewFile writes a file that must not exist yet. O_EXCL makes the open
// fail on any existing entry, including a symlink (dangling or not), so nothing
// is ever written through a link.
func createNewFile(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readSandboxPublicKey(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(content))
	if line == "" {
		return "", fmt.Errorf("the sandbox public key %s is empty", path)
	}
	return line, nil
}

// sandboxPublicKeyLine renders the authorized_keys line for an ed25519 key.
func sandboxPublicKeyLine(public ed25519.PublicKey) string {
	blob := sshString(sandboxKeyType)
	blob = append(blob, sshString(public)...)
	return sandboxKeyType + " " + base64.StdEncoding.EncodeToString(blob) + " " + sandboxKeyNote
}

// encodeOpenSSHPrivateKey writes an unencrypted ed25519 key in the
// `openssh-key-v1` container format that OpenSSH 6.5+ reads.
//
// The format is written out rather than pulled in with a dependency: it is a
// short, fully specified container, and this is the only key govard ever needs
// to produce.
func encodeOpenSSHPrivateKey(private ed25519.PrivateKey, comment string) ([]byte, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("unexpected ed25519 private key length %d", len(private))
	}
	public := private.Public().(ed25519.PublicKey)

	publicBlob := append(sshString(sandboxKeyType), sshString(public)...)

	check := make([]byte, 4)
	if _, err := rand.Read(check); err != nil {
		return nil, fmt.Errorf("seed the sandbox key check value: %w", err)
	}

	// The private section repeats the check value twice, so a wrong passphrase
	// (there is none here) is detectable.
	privateSection := append([]byte{}, check...)
	privateSection = append(privateSection, check...)
	privateSection = append(privateSection, sshString(sandboxKeyType)...)
	privateSection = append(privateSection, sshString(public)...)
	privateSection = append(privateSection, sshString(private)...)
	privateSection = append(privateSection, sshString(comment)...)
	// Padding to the cipher block size, 8 bytes for the "none" cipher.
	for pad := byte(1); len(privateSection)%8 != 0; pad++ {
		privateSection = append(privateSection, pad)
	}

	var body []byte
	body = append(body, []byte(opensshKeyMagic)...)
	body = append(body, sshString("none")...)         // ciphername
	body = append(body, sshString("none")...)         // kdfname
	body = append(body, sshString("")...)             // kdfoptions
	body = append(body, beUint32(1)...)               // number of keys
	body = append(body, sshString(publicBlob)...)     // public key
	body = append(body, sshString(privateSection)...) // private keys, "encrypted"

	encoded := base64.StdEncoding.EncodeToString(body)
	var builder strings.Builder
	builder.WriteString(opensshKeyBegin)
	builder.WriteString("\n")
	for len(encoded) > 64 {
		builder.WriteString(encoded[:64])
		builder.WriteString("\n")
		encoded = encoded[64:]
	}
	builder.WriteString(encoded)
	builder.WriteString("\n")
	builder.WriteString(opensshKeyEnd)
	builder.WriteString("\n")

	if block, _ := pem.Decode([]byte(builder.String())); block == nil {
		return nil, fmt.Errorf("the generated private key is not valid PEM")
	}
	return []byte(builder.String()), nil
}

// sshString is the length-prefixed byte string the SSH wire format uses.
func sshString(value any) []byte {
	var raw []byte
	switch typed := value.(type) {
	case string:
		raw = []byte(typed)
	case []byte:
		raw = typed
	case ed25519.PublicKey:
		raw = []byte(typed)
	case ed25519.PrivateKey:
		raw = []byte(typed)
	default:
		raw = nil
	}
	return append(beUint32(uint32(len(raw))), raw...)
}

func beUint32(value uint32) []byte {
	buffer := make([]byte, 4)
	binary.BigEndian.PutUint32(buffer, value)
	return buffer
}
