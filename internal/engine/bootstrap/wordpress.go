package bootstrap

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"govard/internal/conventions"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

const (
	wordpressCoreArchiveURL = "https://wordpress.org/latest.tar.gz"
	wordpressCorePrefix     = "wordpress/"
)

var wordpressCoreDownloader = downloadAndExtractWordPressCore

type WordPressBootstrap struct {
	Options Options
}

func NewWordPressBootstrap(opts Options) *WordPressBootstrap {
	return &WordPressBootstrap{Options: opts}
}

func (w *WordPressBootstrap) Name() string {
	return "wordpress"
}

func (w *WordPressBootstrap) SupportsFreshInstall() bool {
	return true
}

func (w *WordPressBootstrap) SupportsClone() bool {
	return true
}

func (w *WordPressBootstrap) FreshCommands() []string {
	return []string{
		"download WordPress core archive",
	}
}

func (w *WordPressBootstrap) CreateProject(projectDir string) error {
	pterm.Info.Println("Creating fresh WordPress project...")

	if err := removeProjectContents(projectDir); err != nil {
		return err
	}
	if err := wordpressCoreDownloader(projectDir); err != nil {
		return fmt.Errorf("failed to download WordPress core: %w", err)
	}

	pterm.Success.Println("WordPress downloaded successfully")
	return nil
}

func (w *WordPressBootstrap) Install(projectDir string) error {
	pterm.Info.Println("Running WordPress installation steps...")

	configPath := filepath.Join(wordpressAppDir(projectDir), "wp-config.php")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := w.createWordPressConfig(projectDir); err != nil {
			return err
		}
		pterm.Success.Println("Created wp-config.php")
	}

	if err := w.waitForWordPressDatabase(projectDir); err != nil {
		return err
	}

	siteURL := "http://localhost"
	if strings.TrimSpace(w.Options.Domain) != "" {
		siteURL = "https://" + strings.TrimSpace(w.Options.Domain)
	}

	if err := w.installWordPressSite(projectDir, siteURL); err != nil {
		return err
	}
	if err := w.updateWordPressSiteURL(projectDir, siteURL); err != nil {
		return err
	}

	pterm.Success.Println("WordPress installation completed")
	return nil
}

func (w *WordPressBootstrap) Configure(projectDir string) error {
	pterm.Info.Println("Configuring WordPress environment...")
	pterm.Success.Println("WordPress configured successfully")
	return nil
}

func (w *WordPressBootstrap) PostClone(projectDir string) error {
	pterm.Info.Println("Setting up cloned WordPress project...")

	configPath := filepath.Join(wordpressAppDir(projectDir), "wp-config.php")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := w.createWordPressConfig(projectDir); err != nil {
			return err
		}
	}

	siteURL := fmt.Sprintf("https://%s", w.Options.Domain)
	if strings.TrimSpace(w.Options.Domain) == "" {
		siteURL = "http://localhost"
	}
	if err := w.updateWordPressSiteURL(projectDir, siteURL); err != nil {
		pterm.Warning.Printf("Note: Could not automatically update site URLs: %v\n", err)
	} else {
		pterm.Success.Printf("WordPress site URLs updated to %s\n", siteURL)
	}

	pterm.Success.Println("Post-clone setup completed")
	return nil
}

func (w *WordPressBootstrap) createWordPressConfig(projectDir string) error {
	appDir := wordpressAppDir(projectDir)
	samplePath := filepath.Join(appDir, "wp-config-sample.php")
	data, err := os.ReadFile(samplePath)
	if err != nil {
		return fmt.Errorf("read wp-config-sample.php: %w", err)
	}

	dbHost, dbUser, dbPass, dbName := w.resolveDBConfig()
	content := string(data)
	content = strings.ReplaceAll(content, "database_name_here", dbName)
	content = strings.ReplaceAll(content, "username_here", dbUser)
	content = strings.ReplaceAll(content, "password_here", dbPass)
	content = strings.ReplaceAll(content, "localhost", dbHost)
	for strings.Contains(content, "put your unique phrase here") {
		secret, err := generateWordPressSecret()
		if err != nil {
			return fmt.Errorf("generate WordPress secret: %w", err)
		}
		content = strings.Replace(content, "put your unique phrase here", secret, 1)
	}

	content = injectWordPressProxySupport(content)
	if err := os.WriteFile(filepath.Join(appDir, "wp-config.php"), []byte(content), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write wp-config.php: %w", err)
	}

	return nil
}

func injectWordPressProxySupport(content string) string {
	if strings.Contains(content, "HTTP_X_FORWARDED_PROTO") {
		return content
	}

	snippet := "\n// Govard: trust HTTPS proxy headers.\nif (isset($_SERVER['HTTP_X_FORWARDED_PROTO']) && $_SERVER['HTTP_X_FORWARDED_PROTO'] === 'https') {\n    $_SERVER['HTTPS'] = 'on';\n    $_SERVER['SERVER_PORT'] = 443;\n}\n"
	if strings.HasPrefix(content, "<?php") {
		return strings.Replace(content, "<?php", "<?php"+snippet, 1)
	}
	return snippet + content
}

func (w *WordPressBootstrap) installWordPressSite(projectDir, siteURL string) error {
	appDir := wordpressAppDir(projectDir)
	loadPath := filepath.Join(appDir, "wp-load.php")
	upgradePath := filepath.Join(appDir, "wp-admin", "includes", "upgrade.php")
	if w.Options.Runner != nil {
		runnerAppDir := wordpressRunnerAppDir(projectDir)
		loadPath = path.Join(runnerAppDir, "wp-load.php")
		upgradePath = path.Join(runnerAppDir, "wp-admin", "includes", "upgrade.php")
	}

	host := strings.TrimPrefix(siteURL, "https://")
	host = strings.TrimPrefix(host, "http://")
	httpsValue := ""
	if strings.HasPrefix(siteURL, "https://") {
		httpsValue = "on"
	}

	code := strings.Join([]string{
		"$_SERVER['HTTP_HOST'] = " + strconv.Quote(host) + ";",
		"$_SERVER['SERVER_NAME'] = " + strconv.Quote(host) + ";",
		"$_SERVER['REQUEST_URI'] = '/';",
		"$_SERVER['HTTPS'] = " + strconv.Quote(httpsValue) + ";",
		"require " + strconv.Quote(loadPath) + ";",
		"require " + strconv.Quote(upgradePath) + ";",
		"if (!is_blog_installed()) {",
		"    wp_install(" + strconv.Quote("WordPress Site") + ", " + strconv.Quote(conventions.DefaultAdminUser) + ", " + strconv.Quote(conventions.DefaultAdminEmail) + ", true, '', " + strconv.Quote(conventions.DefaultAdminPassword) + ");",
		"}",
	}, "\n")

	if err := runPHPOneLiner(projectDir, w.Options.Runner, code); err != nil {
		return fmt.Errorf("install WordPress site: %w", err)
	}

	return nil
}

func (w *WordPressBootstrap) updateWordPressSiteURL(projectDir, siteURL string) error {
	loadPath := filepath.Join(wordpressAppDir(projectDir), "wp-load.php")
	if w.Options.Runner != nil {
		loadPath = path.Join(wordpressRunnerAppDir(projectDir), "wp-load.php")
	}

	host := strings.TrimPrefix(siteURL, "https://")
	host = strings.TrimPrefix(host, "http://")
	httpsValue := ""
	if strings.HasPrefix(siteURL, "https://") {
		httpsValue = "on"
	}

	code := strings.Join([]string{
		"$_SERVER['HTTP_HOST'] = " + strconv.Quote(host) + ";",
		"$_SERVER['SERVER_NAME'] = " + strconv.Quote(host) + ";",
		"$_SERVER['REQUEST_URI'] = '/';",
		"$_SERVER['HTTPS'] = " + strconv.Quote(httpsValue) + ";",
		"require " + strconv.Quote(loadPath) + ";",
		"update_option('siteurl', " + strconv.Quote(siteURL) + ");",
		"update_option('home', " + strconv.Quote(siteURL) + ");",
	}, "\n")

	if err := runPHPOneLiner(projectDir, w.Options.Runner, code); err != nil {
		return fmt.Errorf("update WordPress site URL: %w", err)
	}

	return nil
}

func (w *WordPressBootstrap) resolveDBConfig() (host, user, pass, name string) {
	host = strings.TrimSpace(w.Options.DBHost)
	if host == "" {
		host = conventions.DefaultDBHost
	}
	user = strings.TrimSpace(w.Options.DBUser)
	if user == "" {
		user = "wordpress"
	}
	pass = w.Options.DBPass
	if pass == "" {
		pass = "wordpress"
	}
	name = strings.TrimSpace(w.Options.DBName)
	if name == "" {
		name = "wordpress"
	}
	return host, user, pass, name
}

func (w *WordPressBootstrap) waitForWordPressDatabase(projectDir string) error {
	dbHost, dbUser, dbPass, dbName := w.resolveDBConfig()
	return waitForMySQLDatabase(projectDir, w.Options.Runner, dbHost, dbUser, dbPass, dbName)
}

func downloadAndExtractWordPressCore(projectDir string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(wordpressCoreArchiveURL)
	if err != nil {
		return fmt.Errorf("download WordPress core: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download WordPress core: unexpected status %s", resp.Status)
	}

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("read WordPress archive: %w", err)
	}
	defer func() {
		if closeErr := gzipReader.Close(); closeErr != nil {
			pterm.Warning.Printf("Could not close WordPress archive reader: %v\n", closeErr)
		}
	}()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("extract WordPress archive: %w", err)
		}

		if !strings.HasPrefix(header.Name, wordpressCorePrefix) {
			continue
		}

		relativePath := strings.TrimPrefix(header.Name, wordpressCorePrefix)
		relativePath = strings.TrimPrefix(relativePath, "/")
		if relativePath == "" {
			continue
		}

		targetPath := filepath.Join(projectDir, filepath.FromSlash(relativePath))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, conventions.DefaultDirPerm); err != nil {
				return fmt.Errorf("create WordPress directory %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), conventions.DefaultDirPerm); err != nil {
				return fmt.Errorf("create WordPress parent directory %s: %w", filepath.Dir(targetPath), err)
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create WordPress file %s: %w", targetPath, err)
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				closeErr := file.Close()
				return fmt.Errorf("write WordPress file %s: %w", targetPath, errors.Join(err, closeErr))
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("close WordPress file %s: %w", targetPath, err)
			}
		}
	}

	return nil
}

func generateWordPressSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func wordpressAppDir(projectDir string) string {
	subdir := filepath.Join(projectDir, "wordpress")
	for _, filename := range []string{"wp-load.php", "wp-config-sample.php", "wp-config.php"} {
		if _, err := os.Stat(filepath.Join(projectDir, filename)); err == nil {
			return projectDir
		}
		if _, err := os.Stat(filepath.Join(subdir, filename)); err == nil {
			return subdir
		}
	}

	return projectDir
}

func wordpressRunnerAppDir(projectDir string) string {
	appDir := wordpressAppDir(projectDir)
	relativeDir, err := filepath.Rel(projectDir, appDir)
	if err != nil || relativeDir == "." {
		return conventions.DefaultWorkDir
	}

	return path.Join(conventions.DefaultWorkDir, filepath.ToSlash(relativeDir))
}

func SetWordPressCoreDownloaderForTest(fn func(projectDir string) error) func() {
	previous := wordpressCoreDownloader
	if fn != nil {
		wordpressCoreDownloader = fn
	}
	return func() {
		wordpressCoreDownloader = previous
	}
}
