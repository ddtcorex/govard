package gateway

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
)

// The gateway's second hop authenticates to every target with one keypair,
// generated in Go rather than by shelling out to ssh-keygen: gateway
// administration (allow-key/revoke-key/status) is capability "none" (see
// internal/cmd/gateway.go), so it must work on a host with no ssh-keygen
// binary at all.
//
// This is a deliberate, self-contained copy of the encoder in
// internal/deploy/sandbox_key.go, not an import of it: internal/deploy
// already imports internal/gateway (to register a sandbox as a gateway
// target on `sandbox up`, Task 5), so the reverse import would cycle. The
// format is a short, fully specified container with no decisions left in
// it, so duplicating it once here is cheaper than restructuring the
// sandbox package's import boundary for it.
const (
	secondHopKeyName = "id_ed25519"
	secondHopKeyType = "ssh-ed25519"
	secondHopKeyNote = "govard-gateway"
	opensshKeyBegin  = "-----BEGIN OPENSSH PRIVATE KEY-----"
	opensshKeyEnd    = "-----END OPENSSH PRIVATE KEY-----"
	opensshKeyMagic  = "openssh-key-v1\x00"
)

// EnsureSecondHopKey returns the gateway's second-hop identity, generating it
// on first use. An existing pair is reused unchanged: every provisioned
// target already trusts its public half, and regenerating it would silently
// lock the gateway out of every target that is not re-provisioned in the
// same moment.
func EnsureSecondHopKey() (keyPath string, publicLine string, err error) {
	dir := GatewayDir()
	privatePath := filepath.Join(dir, secondHopKeyName)
	publicPath := filepath.Join(dir, secondHopKeyName+".pub")

	if existing, readErr := readPublicKeyLine(publicPath); readErr == nil {
		return privatePath, existing, nil
	} else if !os.IsNotExist(readErr) {
		return "", "", readErr
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("create %s: %w", dir, err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate the gateway second-hop key: %w", err)
	}

	publicLine = publicKeyLine(public)
	privateFile, err := encodeOpenSSHPrivateKey(private, secondHopKeyNote)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(privatePath, privateFile, 0o600); err != nil {
		return "", "", fmt.Errorf("write the gateway second-hop private key: %w", err)
	}
	// WriteFile only applies the mode at creation (masked by umask), so
	// enforce it explicitly: a pre-existing file or restrictive umask must
	// not leave the private key readable by others.
	if err := os.Chmod(privatePath, 0o600); err != nil {
		return "", "", fmt.Errorf("chmod the gateway second-hop private key: %w", err)
	}
	if err := os.WriteFile(publicPath, []byte(publicLine+"\n"), 0o644); err != nil {
		return "", "", fmt.Errorf("write the gateway second-hop public key: %w", err)
	}
	if err := os.Chmod(publicPath, 0o644); err != nil {
		return "", "", fmt.Errorf("chmod the gateway second-hop public key: %w", err)
	}
	return privatePath, publicLine, nil
}

func readPublicKeyLine(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(content))
	if line == "" {
		return "", fmt.Errorf("the gateway public key %s is empty", path)
	}
	return line, nil
}

// publicKeyLine renders the authorized_keys line for an ed25519 key.
func publicKeyLine(public ed25519.PublicKey) string {
	blob := sshString(secondHopKeyType)
	blob = append(blob, sshString(public)...)
	return secondHopKeyType + " " + base64.StdEncoding.EncodeToString(blob) + " " + secondHopKeyNote
}

// encodeOpenSSHPrivateKey writes an unencrypted ed25519 key in the
// openssh-key-v1 container format that OpenSSH 6.5+ reads.
func encodeOpenSSHPrivateKey(private ed25519.PrivateKey, comment string) ([]byte, error) {
	if len(private) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("unexpected ed25519 private key length %d", len(private))
	}
	public := private.Public().(ed25519.PublicKey)

	publicBlob := append(sshString(secondHopKeyType), sshString(public)...)

	check := make([]byte, 4)
	if _, err := rand.Read(check); err != nil {
		return nil, fmt.Errorf("seed the gateway key check value: %w", err)
	}

	privateSection := append([]byte{}, check...)
	privateSection = append(privateSection, check...)
	privateSection = append(privateSection, sshString(secondHopKeyType)...)
	privateSection = append(privateSection, sshString(public)...)
	privateSection = append(privateSection, sshString(private)...)
	privateSection = append(privateSection, sshString(comment)...)
	for pad := byte(1); len(privateSection)%8 != 0; pad++ {
		privateSection = append(privateSection, pad)
	}

	var body []byte
	body = append(body, []byte(opensshKeyMagic)...)
	body = append(body, sshString("none")...)
	body = append(body, sshString("none")...)
	body = append(body, sshString("")...)
	body = append(body, beUint32(1)...)
	body = append(body, sshString(publicBlob)...)
	body = append(body, sshString(privateSection)...)

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
		return nil, fmt.Errorf("the generated gateway key is not valid PEM")
	}
	return []byte(builder.String()), nil
}

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
