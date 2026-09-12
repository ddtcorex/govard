package remote

import (
	"os"
	"path/filepath"
)

// multiplexPersist is how long a shared connection stays open after its last
// command. A deploy runs its steps seconds apart, so the window has to outlive
// the gaps inside one run and no more.
const multiplexPersist = "60s"

// sshControlDir is the directory holding one shared connection per target. It is
// govard's own, below the same home as the rest of its state, and the sockets
// never leave the machine running the deploy.
func sshControlDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".govard", "ssh")
}

// multiplexArgs returns the ssh options that reuse one connection for every
// command against a target.
//
// A deploy runs one command per step, and each of them used to pay a TCP
// handshake, a key exchange and an authentication — around twenty for a Magento
// pipeline, half of them inside the maintenance window. `ControlMaster` makes the
// first connection the only one. `%C` is OpenSSH's own hash of the local host,
// remote host, port and user, which keeps one socket per target and the path far
// below the ~104-byte limit a Unix socket allows.
//
// The options are omitted when the directory cannot be created: connection reuse
// is a performance property, and a host whose home is read-only must still deploy.
func multiplexArgs() []string {
	directory := sshControlDir()
	if directory == "" {
		return nil
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil
	}
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + filepath.Join(directory, "%C"),
		"-o", "ControlPersist=" + multiplexPersist,
	}
}

// MultiplexArgsForTest exposes the connection-sharing options to the tests/
// package.
func MultiplexArgsForTest() []string {
	return multiplexArgs()
}
