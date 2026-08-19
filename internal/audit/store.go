package audit

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	manifestFilename = "manifest.json"
	resultFilename   = "audit-result.json"
)

var validID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Store struct {
	root        string
	now         func() time.Time
	randomBytes func([]byte) error
}

type StoreOption func(*Store)

func WithClock(now func() time.Time) StoreOption {
	return func(store *Store) {
		store.now = now
	}
}

func WithRandomBytes(randomBytes func([]byte) error) StoreOption {
	return func(store *Store) {
		store.randomBytes = randomBytes
	}
}

func NewStore(root string, options ...StoreOption) *Store {
	store := &Store{
		root: root,
		now:  time.Now,
		randomBytes: func(buffer []byte) error {
			_, err := io.ReadFull(rand.Reader, buffer)
			return err
		},
	}
	for _, option := range options {
		option(store)
	}
	return store
}

func DefaultStoreRoot(govardHome string) string {
	return filepath.Join(govardHome, "audit")
}

// ProjectID hashes the canonical project root and repository identity. Callers
// must resolve the root with filepath.EvalSymlinks and filepath.Abs first.
func ProjectID(canonicalRoot, repositoryIdentity string) string {
	sum := sha256.Sum256([]byte(canonicalRoot + "\n" + repositoryIdentity))
	return "project-" + hex.EncodeToString(sum[:])[:16]
}

func (s *Store) CreateSession(manifest SessionManifest) (SessionManifest, error) {
	if err := validateSchemaVersion(manifest.SchemaVersion); err != nil {
		return SessionManifest{}, err
	}
	if err := validateID("project", manifest.ProjectID); err != nil {
		return SessionManifest{}, err
	}
	if manifest.SessionID == "" {
		id, err := s.newSessionID()
		if err != nil {
			return SessionManifest{}, fmt.Errorf("generate session ID: %w", err)
		}
		manifest.SessionID = id
	} else if err := validateID("session", manifest.SessionID); err != nil {
		return SessionManifest{}, err
	}
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = s.now().UTC()
	} else {
		manifest.CreatedAt = manifest.CreatedAt.UTC()
	}
	if manifest.Runs == nil {
		manifest.Runs = []RunReference{}
	}

	sessionPath, err := s.sessionPath(manifest.ProjectID, manifest.SessionID)
	if err != nil {
		return SessionManifest{}, err
	}
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o700); err != nil {
		return SessionManifest{}, fmt.Errorf("create audit sessions directory: %w", err)
	}
	if err := os.Mkdir(sessionPath, 0o700); err != nil {
		return SessionManifest{}, fmt.Errorf("create audit session %q: %w", manifest.SessionID, err)
	}
	if err := writeJSONAtomic(filepath.Join(sessionPath, manifestFilename), manifest); err != nil {
		return SessionManifest{}, fmt.Errorf("write session manifest: %w", err)
	}
	return manifest, nil
}

func (s *Store) ReadSession(projectID, sessionID string) (SessionManifest, error) {
	sessionPath, err := s.sessionPath(projectID, sessionID)
	if err != nil {
		return SessionManifest{}, err
	}
	var manifest SessionManifest
	if err := readJSON(filepath.Join(sessionPath, manifestFilename), &manifest); err != nil {
		return SessionManifest{}, fmt.Errorf("read session manifest %q: %w", sessionID, err)
	}
	if manifest.Runs == nil {
		manifest.Runs = []RunReference{}
	}
	return manifest, nil
}

func (s *Store) CreateRun(projectID, sessionID string) (RunReference, error) {
	manifest, err := s.ReadSession(projectID, sessionID)
	if err != nil {
		return RunReference{}, err
	}
	run := RunReference{
		RunID:     fmt.Sprintf("run-%04d", len(manifest.Runs)+1),
		CreatedAt: s.now().UTC(),
	}
	if err := validateID("run", run.RunID); err != nil {
		return RunReference{}, err
	}
	sessionPath, err := s.sessionPath(projectID, sessionID)
	if err != nil {
		return RunReference{}, err
	}
	runsPath := filepath.Join(sessionPath, "runs")
	if err := os.MkdirAll(runsPath, 0o700); err != nil {
		return RunReference{}, fmt.Errorf("create audit runs directory: %w", err)
	}
	if err := os.Mkdir(filepath.Join(runsPath, run.RunID), 0o700); err != nil {
		return RunReference{}, fmt.Errorf("create audit run %q: %w", run.RunID, err)
	}
	manifest.Runs = append(manifest.Runs, run)
	if err := writeJSONAtomic(filepath.Join(sessionPath, manifestFilename), manifest); err != nil {
		return RunReference{}, fmt.Errorf("write session manifest: %w", err)
	}
	return run, nil
}

func (s *Store) WriteResult(result RunResult) error {
	if err := validateSchemaVersion(result.SchemaVersion); err != nil {
		return err
	}
	resultPath, err := s.resultPath(result.ProjectID, result.SessionID, result.RunID)
	if err != nil {
		return err
	}
	if result.Jobs == nil {
		result.Jobs = []JobResult{}
	}
	if result.Artifacts == nil {
		result.Artifacts = []Artifact{}
	}
	if result.Errors == nil {
		result.Errors = []AuditError{}
	}
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o700); err != nil {
		return fmt.Errorf("create audit result directory: %w", err)
	}
	if err := writeJSONAtomic(resultPath, result); err != nil {
		return fmt.Errorf("write audit result: %w", err)
	}
	return nil
}

func (s *Store) ReadResult(projectID, sessionID, runID string) (RunResult, error) {
	resultPath, err := s.resultPath(projectID, sessionID, runID)
	if err != nil {
		return RunResult{}, err
	}
	var result RunResult
	if err := readJSON(resultPath, &result); err != nil {
		return RunResult{}, fmt.Errorf("read audit result %q: %w", runID, err)
	}
	if result.Jobs == nil {
		result.Jobs = []JobResult{}
	}
	if result.Artifacts == nil {
		result.Artifacts = []Artifact{}
	}
	if result.Errors == nil {
		result.Errors = []AuditError{}
	}
	return result, nil
}

func (s *Store) SessionPath(projectID, sessionID string) string {
	path, err := s.sessionPath(projectID, sessionID)
	if err != nil {
		return ""
	}
	return path
}

func (s *Store) CleanupOlderThan(projectID string, cutoff time.Time) ([]string, error) {
	if err := validateID("project", projectID); err != nil {
		return nil, err
	}
	sessionsPath := filepath.Join(s.root, projectID, "sessions")
	entries, err := os.ReadDir(sessionsPath)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read audit sessions directory: %w", err)
	}

	removed := make([]string, 0)
	for _, entry := range entries {
		if err := validateID("session", entry.Name()); err != nil {
			continue
		}
		sessionPath := filepath.Join(sessionsPath, entry.Name())
		info, err := os.Lstat(sessionPath)
		if err != nil {
			return nil, fmt.Errorf("stat audit session %q: %w", entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		manifestPath := filepath.Join(sessionPath, manifestFilename)
		manifestInfo, err := os.Lstat(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat session manifest %q: %w", entry.Name(), err)
		}
		if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
			continue
		}
		var manifest SessionManifest
		if err := readJSON(manifestPath, &manifest); err != nil {
			return nil, fmt.Errorf("read session manifest %q: %w", entry.Name(), err)
		}
		if !manifest.CreatedAt.Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(sessionPath); err != nil {
			return nil, fmt.Errorf("remove audit session %q: %w", entry.Name(), err)
		}
		removed = append(removed, entry.Name())
	}
	return removed, nil
}

func (s *Store) newSessionID() (string, error) {
	buffer := make([]byte, 4)
	if err := s.randomBytes(buffer); err != nil {
		return "", err
	}
	return s.now().UTC().Format("20060102T150405Z-") + hex.EncodeToString(buffer), nil
}

func (s *Store) sessionPath(projectID, sessionID string) (string, error) {
	if err := validateID("project", projectID); err != nil {
		return "", err
	}
	if err := validateID("session", sessionID); err != nil {
		return "", err
	}
	return filepath.Join(s.root, projectID, "sessions", sessionID), nil
}

func (s *Store) resultPath(projectID, sessionID, runID string) (string, error) {
	sessionPath, err := s.sessionPath(projectID, sessionID)
	if err != nil {
		return "", err
	}
	if err := validateID("run", runID); err != nil {
		return "", err
	}
	return filepath.Join(sessionPath, "runs", runID, resultFilename), nil
}

func validateID(kind, value string) error {
	if value == "" || value == "." || value == ".." || !validID.MatchString(value) {
		return fmt.Errorf("invalid audit %s ID %q", kind, value)
	}
	return nil
}

func validateSchemaVersion(version int) error {
	if version != SchemaVersion {
		return fmt.Errorf("unsupported audit schema version %d", version)
	}
	return nil
}

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func writeJSONAtomic(path string, value any) (err error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err = temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err = temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
