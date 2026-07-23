package engine

import (
	"compress/gzip"
	"fmt"
	"govard/internal/conventions"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

type SnapshotMetadata struct {
	Name         string    `yaml:"name"`
	CreatedAt    time.Time `yaml:"created_at"`
	Framework    string    `yaml:"framework"`
	Domain       string    `yaml:"domain"`
	ExtraDomains []string  `yaml:"extra_domains,omitempty"`
	DB           bool      `yaml:"db"`
	Media        bool      `yaml:"media"`
	SizeBytes    int64     `yaml:"size_bytes"`
}

type snapshotDBCredentials struct {
	Username string
	Password string
	Database string
}

func SnapshotRoot(projectRoot string) string {
	return filepath.Join(projectRoot, ".govard", "snapshots")
}

func CreateSnapshot(projectRoot string, config Config, name string) (string, error) {
	if name == "" {
		name = time.Now().Format("20060102-150405")
	}

	root := SnapshotRoot(projectRoot)
	snapshotDir := filepath.Join(root, name)
	if _, err := os.Stat(snapshotDir); err == nil {
		return "", fmt.Errorf("snapshot %s already exists", name)
	}

	if err := os.MkdirAll(snapshotDir, conventions.DefaultDirPerm); err != nil {
		return "", fmt.Errorf("create snapshot directory: %w", err)
	}

	meta := SnapshotMetadata{
		Name:         name,
		CreatedAt:    time.Now(),
		Framework:    config.Framework,
		Domain:       config.Domain,
		ExtraDomains: config.ExtraDomains,
		DB:           false,
		Media:        false,
	}

	containerName := fmt.Sprintf("%s-db-1", config.ProjectName)
	credentials := resolveSnapshotDBCredentials(containerName)
	dbPath := filepath.Join(snapshotDir, "db.sql.gz")
	dbFile, err := os.Create(dbPath)
	if err == nil {
		gzipWriter := gzip.NewWriter(dbFile)
		pterm.Info.Printf("Creating DB snapshot for %s [User: %s, DB: %s]\n", containerName, credentials.Username, credentials.Database)
		dumpCmd := buildSnapshotDumpCommand(containerName, credentials)
		dumpCmd.Stdout = gzipWriter
		dumpCmd.Stderr = os.Stderr
		if err := dumpCmd.Run(); err == nil {
			_ = gzipWriter.Close()
			meta.DB = true
		} else {
			_ = gzipWriter.Close()
		}
		_ = dbFile.Close()
	}

	mediaSource := ResolveLocalMediaPath(config, projectRoot)
	mediaDest := filepath.Join(snapshotDir, "media")
	if info, err := os.Stat(mediaSource); err == nil && info.IsDir() {
		if err := copyDir(mediaSource, mediaDest); err == nil {
			meta.Media = true
		}
	}

	payload, err := yaml.Marshal(meta)
	if err == nil {
		_ = os.WriteFile(filepath.Join(snapshotDir, "metadata.yml"), payload, conventions.DefaultFilePerm)
	}

	return snapshotDir, nil
}

func ListSnapshots(projectRoot string) ([]SnapshotMetadata, error) {
	root := SnapshotRoot(projectRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []SnapshotMetadata{}, nil
		}
		return nil, fmt.Errorf("read snapshots directory %s: %w", root, err)
	}

	snapshots := make([]SnapshotMetadata, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		snapshotDir := filepath.Join(root, name)
		metaPath := filepath.Join(snapshotDir, "metadata.yml")
		payload, err := os.ReadFile(metaPath)

		var meta SnapshotMetadata
		if err != nil {
			meta = SnapshotMetadata{Name: name}
		} else if err := yaml.Unmarshal(payload, &meta); err != nil {
			meta = SnapshotMetadata{Name: name}
		}

		if meta.Name == "" {
			meta.Name = name
		}

		// Calculate size
		meta.SizeBytes = calculateDirSize(snapshotDir)

		snapshots = append(snapshots, meta)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	return snapshots, nil
}

func calculateDirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func RestoreSnapshot(projectRoot string, config Config, name string, dbOnly bool, mediaOnly bool) error {
	snapshotDir := filepath.Join(SnapshotRoot(projectRoot), name)
	if _, err := os.Stat(snapshotDir); err != nil {
		return fmt.Errorf("snapshot %s not found", name)
	}

	if !mediaOnly {
		dbPathGzip := filepath.Join(snapshotDir, "db.sql.gz")
		dbPathRaw := filepath.Join(snapshotDir, "db.sql")

		var dbReader io.ReadCloser
		var err error

		if _, err = os.Stat(dbPathGzip); err == nil {
			file, err := os.Open(dbPathGzip)
			if err != nil {
				return fmt.Errorf("open gzipped database snapshot %s: %w", name, err)
			}
			gzReader, err := gzip.NewReader(file)
			if err != nil {
				_ = file.Close()
				return fmt.Errorf("create gzip reader for %s: %w", name, err)
			}
			dbReader = struct {
				io.Reader
				io.Closer
			}{gzReader, file}
		} else if _, err = os.Stat(dbPathRaw); err == nil {
			dbReader, err = os.Open(dbPathRaw)
			if err != nil {
				return fmt.Errorf("open raw database snapshot %s: %w", name, err)
			}
		}

		if dbReader != nil {
			defer dbReader.Close()
			containerName := fmt.Sprintf("%s-db-1", config.ProjectName)
			credentials := resolveSnapshotDBCredentials(containerName)
			importCmd := buildSnapshotImportCommand(containerName, credentials)
			importCmd.Stdin = dbReader
			importCmd.Stdout = os.Stdout
			importCmd.Stderr = os.Stderr
			if err := importCmd.Run(); err != nil {
				return fmt.Errorf("restore database from snapshot %s: %w", name, err)
			}
		}
	}

	if !dbOnly {
		mediaSnapshot := filepath.Join(snapshotDir, "media")
		if info, err := os.Stat(mediaSnapshot); err == nil && info.IsDir() {
			targetMedia := ResolveLocalMediaPath(config, projectRoot)
			if err := os.RemoveAll(targetMedia); err != nil {
				return fmt.Errorf("remove existing media directory %s: %w", targetMedia, err)
			}
			if err := copyDir(mediaSnapshot, targetMedia); err != nil {
				return fmt.Errorf("restore media from snapshot %s: %w", name, err)
			}
		}
	}

	return nil
}

func copyDir(src string, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read source directory %s: %w", src, err)
	}
	if err := os.MkdirAll(dst, conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create snapshot restore dir: %w", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("read entry info for %s: %w", srcPath, err)
		}

		if info.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := copyFileWithMode(srcPath, dstPath, info.Mode()); err != nil {
			return err
		}
	}

	return nil
}

func copyFileWithMode(src string, dst string, mode os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create snapshot parent: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open destination file %s: %w", dst, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return nil
}

func resolveSnapshotDBCredentials(containerName string) snapshotDBCredentials {
	credentials := snapshotDBCredentials{
		Username: "magento",
		Password: "magento",
		Database: "magento",
	}

	inspectCommand := exec.Command("docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", containerName)
	output, err := inspectCommand.Output()
	if err != nil {
		return credentials
	}

	envMap := parseSnapshotEnvMap(string(output))
	if user := strings.TrimSpace(envMap["MYSQL_USER"]); user != "" {
		credentials.Username = user
	}
	if password := envMap["MYSQL_PASSWORD"]; password != "" {
		credentials.Password = password
	}
	if database := strings.TrimSpace(envMap["MYSQL_DATABASE"]); database != "" {
		credentials.Database = database
	}
	return credentials
}

func buildSnapshotDumpCommand(containerName string, credentials snapshotDBCredentials) *exec.Cmd {
	credentials = normalizeSnapshotDBCredentials(credentials)
	args := []string{"exec", "-i"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args, containerName, "mysqldump", "-u", credentials.Username, credentials.Database)
	return exec.Command("docker", args...)
}

func buildSnapshotImportCommand(containerName string, credentials snapshotDBCredentials) *exec.Cmd {
	credentials = normalizeSnapshotDBCredentials(credentials)
	args := []string{"exec", "-i"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args, containerName, "mysql", "-u", credentials.Username, credentials.Database)
	return exec.Command("docker", args...)
}

func normalizeSnapshotDBCredentials(credentials snapshotDBCredentials) snapshotDBCredentials {
	result := credentials
	if strings.TrimSpace(result.Username) == "" {
		result.Username = "magento"
	}
	if strings.TrimSpace(result.Database) == "" {
		result.Database = "magento"
	}
	return result
}

func parseSnapshotEnvMap(raw string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result[strings.TrimSpace(parts[0])] = parts[1]
	}
	return result
}

func BuildSnapshotDumpCommandForTest(containerName string, username string, password string, database string) []string {
	command := buildSnapshotDumpCommand(containerName, snapshotDBCredentials{
		Username: username,
		Password: password,
		Database: database,
	})
	return command.Args
}

func BuildSnapshotImportCommandForTest(containerName string, username string, password string, database string) []string {
	command := buildSnapshotImportCommand(containerName, snapshotDBCredentials{
		Username: username,
		Password: password,
		Database: database,
	})
	return command.Args
}
func DeleteSnapshot(projectRoot string, name string) error {
	snapshotDir := filepath.Join(SnapshotRoot(projectRoot), name)
	if _, err := os.Stat(snapshotDir); err != nil {
		return fmt.Errorf("snapshot %s not found", name)
	}
	return os.RemoveAll(snapshotDir)
}

func ExportSnapshot(projectRoot string, name string, targetPath string) error {
	snapshotDir := filepath.Join(SnapshotRoot(projectRoot), name)
	if _, err := os.Stat(snapshotDir); err != nil {
		return fmt.Errorf("snapshot %s not found", name)
	}

	if targetPath == "" {
		targetPath = fmt.Sprintf("%s.tar.gz", name)
	}

	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}

	pterm.Info.Printf("Exporting snapshot %s to %s...\n", name, absTargetPath)

	// Create a tar.gz of the snapshot directory
	cmd := exec.Command("tar", "-czf", absTargetPath, "-C", filepath.Dir(snapshotDir), name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar failed: %w\n%s", err, string(output))
	}

	return nil
}
