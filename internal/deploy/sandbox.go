package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The sandbox is a container that plays the remote: govard creates it, points a
// generated `sandbox` remote at it, and then deploys to it over real SSH and
// real rsync. It is the deploy feature's regression suite, and it has no
// test-only branch in the pipeline.
//
// This file owns the environment the container provides. What *application* the
// container has to support is the framework recipe's business (Recipe.Sandbox);
// how large the environment is, is the profile.
const (
	SandboxProfileBasic = "basic"
	SandboxProfilePHP   = "php"
	SandboxProfileFull  = "full"

	// DefaultSandboxProfile is the profile a bare `sandbox up` builds. It is
	// the smallest one that can run a dependency install, because a sandbox
	// that cannot build is a sandbox that cannot reproduce a failed deploy.
	DefaultSandboxProfile = SandboxProfilePHP
)

// SandboxBaseImage is the pinned base image. The digest is deliberate: a
// sandbox that silently changes under a rebuild is not a regression suite.
// Bumping it is a one-line change with a visible diff.
const SandboxBaseImage = "debian@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171"

// sandboxDebianCodename is the release the pinned digest above is. It is named
// because a second packaging repository (sury, for a PHP series Debian does not
// carry) has to be pointed at a codename, and that pointer must move with the
// digest: bumping one without the other installs nothing and fails the build
// with an apt error rather than silently shipping the wrong PHP.
const sandboxDebianCodename = "bookworm"

// sandboxSuryKeyring and sandboxSurySource are where a requested PHP series
// comes from when Debian's own is not the one asked for.
const (
	sandboxSuryKeyring = "/usr/share/keyrings/sury-php.gpg"
	sandboxSurySource  = "/etc/apt/sources.list.d/sury-php.list"
)

// The package versions the image pins. They are bootstrapped from a Debian
// point release, which rotates; the Dockerfile therefore installs the pinned
// version when the archive still has it and falls back to the current one,
// recording whichever it got. A hard pin would brick the sandbox the week the
// archive rotates, for a reason unrelated to govard.
const (
	sandboxOpenSSHVersion = "1:9.2p1-2+deb12u10"
	sandboxRsyncVersion   = "3.2.7-1+deb12u6"
	sandboxGitVersion     = "1:2.39.5-0+deb12u3"
)

// SandboxUserUID and SandboxUserGID are the fixed identity the container's
// `deployer` user has. A fixed uid is what lets settings.owner in the sandbox
// remote name a real identity and exercise the ownership path instead of
// bypassing it.
const (
	SandboxUser        = "deployer"
	SandboxUserUID     = 1000
	SandboxUserGID     = 1000
	SandboxHome        = "/home/deployer"
	SandboxSSHPort     = 22
	SandboxRepoPath    = "/srv/repo.git"
	SandboxPackagesTxt = "/etc/govard-sandbox-packages"
)

// SandboxRequirements is what one framework recipe asks the sandbox to provide
// beyond the profile. The core never interprets these values: it renders them
// into the image.
type SandboxRequirements struct {
	// Packages are extra apt packages (libraries a PHP extension builds against).
	Packages []string
	// Extensions are PHP extensions, installed as the `php-<name>` package.
	Extensions []string
	// Services are init services started inside the container before sshd
	// (a database a migration step needs, a cache a cache-flush needs).
	Services []string
}

// SandboxSpec identifies one sandbox image.
type SandboxSpec struct {
	Project string
	Profile string
	// PHP is the PHP series the image provides, for example "8.4". Empty means the
	// base image's own version (Debian's), which is what a project that does not ask
	// for one gets.
	PHP string
	// WebRoot is where inside the served path the web server serves from, from the
	// project's `stack.web_root` (`/pub` for a storefront served from a subdirectory). It is part of the image
	// because the tag is the hash of the rendered definition.
	WebRoot      string
	Requirements SandboxRequirements
}

// ValidateSandboxPHP normalizes a requested PHP series and refuses anything that is
// not `major.minor`. The value is rendered into the image definition, so it is
// checked rather than trusted: `8.4; rm -rf /` is a string, not a version.
func ValidateSandboxPHP(php string) (string, error) {
	trimmed := strings.TrimSpace(php)
	if trimmed == "" {
		return "", nil
	}
	if !sandboxPHPVersion.MatchString(trimmed) {
		return "", fmt.Errorf("unsupported sandbox php version %q; use a series such as 8.3 or 8.4", php)
	}
	return trimmed, nil
}

// sandboxPHPVersion is `major.minor`, which is what every PHP packaging convention
// (Debian's `php<series>-*` and sury's repository) keys on.
var sandboxPHPVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// ValidateSandboxProfile normalizes a profile name and refuses an unknown one.
// An empty name is the default profile, because "unspecified" is a legitimate
// answer and "minimal" is not.
func ValidateSandboxProfile(profile string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(profile))
	switch normalized {
	case "":
		return DefaultSandboxProfile, nil
	case SandboxProfileBasic, SandboxProfilePHP, SandboxProfileFull:
		return normalized, nil
	default:
		return "", fmt.Errorf("unknown sandbox profile %q; use %s, %s or %s",
			profile, SandboxProfileBasic, SandboxProfilePHP, SandboxProfileFull)
	}
}

// SandboxDockerfile renders the image definition for a profile plus a recipe's
// requirements.
//
// The render is deterministic and order-independent: the image tag is a hash of
// this text, so two recipes that list the same requirements in a different order
// must produce the same image rather than two.
func SandboxDockerfile(spec SandboxSpec) (string, error) {
	resolved, err := ValidateSandboxProfile(spec.Profile)
	if err != nil {
		return "", err
	}
	series, err := ValidateSandboxPHP(spec.PHP)
	if err != nil {
		return "", err
	}
	servesWeb := SandboxServesWeb(resolved)
	exposeWeb := ""
	if servesWeb {
		exposeWeb = fmt.Sprintf(" %d", sandboxWebPort)
	}
	requirements := spec.Requirements

	packages := []string{"openssh-server", "rsync", "git", "ca-certificates", "procps"}
	switch resolved {
	case SandboxProfilePHP:
		packages = append(packages, "nodejs", "npm", "unzip")
	case SandboxProfileFull:
		packages = append(packages, "nodejs", "npm", "unzip", "mariadb-server", "redis-server")
	}
	if servesWeb {
		packages = append(packages, SandboxWebPackages(series)...)
	}
	packages = append(packages, sortedSandboxWords(requirements.Packages)...)

	extensions := sortedSandboxWords(requirements.Extensions)
	extensionPackages := make([]string, 0, len(extensions))
	// `php<series>-*` is the packaging convention of both Debian and the sury
	// repository, so the same extension list works either way; without a series the
	// base image's own `php-*` names are used.
	phpPrefix := "php-"
	if series != "" {
		phpPrefix = "php" + series + "-"
	}
	if resolved != SandboxProfileBasic {
		packages = append(packages, phpPrefix+"cli")
		// Composer comes from Debian only where Debian's PHP does: its package
		// depends on php-cli, which would install the base image's series next to the
		// requested one. The phar is version-independent and it is what the
		// distribution package wraps anyway.
		if series == "" {
			packages = append(packages, "composer")
		}
	}
	for _, extension := range extensions {
		extensionPackages = append(extensionPackages, phpPrefix+extension)
	}

	services := sortedSandboxWords(requirements.Services)
	if servesWeb {
		services = sortedSandboxWords(append(services, sandboxWebService))
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "# Generated by `govard deploy sandbox`. Do not edit: the image tag is\n")
	fmt.Fprintf(&builder, "# the hash of this file, and --recreate rebuilds from it.\n")
	fmt.Fprintf(&builder, "FROM %s\n\n", SandboxBaseImage)
	fmt.Fprintf(&builder, "ARG OPENSSH_VERSION=%s\n", sandboxOpenSSHVersion)
	fmt.Fprintf(&builder, "ARG RSYNC_VERSION=%s\n", sandboxRsyncVersion)
	fmt.Fprintf(&builder, "ARG GIT_VERSION=%s\n\n", sandboxGitVersion)

	fmt.Fprintf(&builder, "ENV DEBIAN_FRONTEND=noninteractive GOVARD_SANDBOX_SERVICES=%q\n\n", strings.Join(services, " "))

	// A PHP series other than the base image's comes from sury, the repository every
	// Debian host that needs a version other than the distribution's uses. This is
	// the difference between "rehearse the deploy" and "rehearse it on the PHP your
	// target actually runs": a project whose lock requires 8.3+ cannot install on
	// bookworm's 8.2 at all.
	if series != "" {
		fmt.Fprintf(&builder, `RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends curl gnupg ca-certificates; \
    curl -fsSL https://packages.sury.org/php/apt.gpg -o %s; \
    echo "deb [signed-by=%s] https://packages.sury.org/php/ %s main" > %s; \
    apt-get update

`, sandboxSuryKeyring, sandboxSuryKeyring, sandboxDebianCodename, sandboxSurySource)
	}

	// The pinned versions are tried first and the current ones are the fallback,
	// so a rotated Debian point release degrades the sandbox instead of
	// breaking it. Whichever was installed is recorded in the image.
	fmt.Fprintf(&builder, `RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates; \
    if ! apt-get install -y --no-install-recommends \
        "openssh-server=${OPENSSH_VERSION}" "rsync=${RSYNC_VERSION}" "git=${GIT_VERSION}"; then \
      echo "govard: pinned package versions are gone from the archive; installing current" >&2; \
      apt-get install -y --no-install-recommends openssh-server rsync git; \
    fi; \
    apt-get install -y --no-install-recommends %s; \
    dpkg-query -W -f='${Package}=${Version}\n' openssh-server rsync git > %s; \
    rm -rf /var/lib/apt/lists/*
`, strings.Join(quoteWords(append(packages, extensionPackages...)), " "), SandboxPackagesTxt)

	// Composer is not a distribution package here (see above), and `php` has to mean
	// the requested series: every recipe command, the sandbox remote's `php_bin` and
	// the stub fixtures all call the plain binary.
	if series != "" && resolved != SandboxProfileBasic {
		fmt.Fprintf(&builder, `
RUN set -eux; \
    update-alternatives --install /usr/bin/php php /usr/bin/php%s 100; \
    curl -fsSL https://getcomposer.org/download/latest-stable/composer.phar -o /usr/local/bin/composer; \
    chmod 0755 /usr/local/bin/composer; \
    php -r 'exit(strpos(PHP_VERSION, "%s") === 0 ? 0 : 1);'; \
    php -v; \
    composer --version
`, series, series)
	}

	// The mirror is bind-mounted from the host, which is a different user than
	// the container's deployer whenever the two uids differ — a CI runner is
	// 1001, the image's deployer is 1000. Git refuses to read a repository
	// owned by someone else ("detected dubious ownership"), so the mirror is
	// declared safe for every user in the container. The path is the mount, not
	// `*`: the only repository the container reads from outside its own home is
	// exactly this one.
	fmt.Fprintf(&builder, "\nRUN git config --system --add safe.directory %s\n", SandboxRepoPath)

	// The deployer identity is fixed at build time, so a sandbox release has an
	// owner a settings.owner can name.
	fmt.Fprintf(&builder, "\nRUN set -eux; \\\n")
	fmt.Fprintf(&builder, "    groupadd -g %d %s; \\\n", SandboxUserGID, SandboxUser)
	// A password field of `!` makes sshd refuse the account outright — "account
	// is locked" — and public-key authentication never gets a chance. `*` is
	// the conventional "no password can match"; PasswordAuthentication is off,
	// so nothing can use it anyway.
	fmt.Fprintf(&builder, "    useradd -m -d %s -u %d -g %d -s /bin/bash -p '*' %s; \\\n", SandboxHome, SandboxUserUID, SandboxUserGID, SandboxUser)
	fmt.Fprintf(&builder, "    install -d -m 0700 -o %s -g %s %s/.ssh; \\\n", SandboxUser, SandboxUser, SandboxHome)
	fmt.Fprintf(&builder, "    passwd -l root\n")

	if servesWeb {
		fmt.Fprintf(&builder, "\nRUN %s\n", sandboxFPMPoolPatch())
		// The web tier cannot serve a directory the deployer will replace, so the
		// served path is created here and handed over, exactly as the pipeline's
		// own release step expects to find it.
		fmt.Fprintf(&builder, "\nRUN set -eux; \\\n")
		fmt.Fprintf(&builder, "    mkdir -p /run/php /var/log/nginx /var/lib/nginx; \\\n")
		fmt.Fprintf(&builder, "    install -d -o %s -g %s %s\n", SandboxUser, SandboxUser, SandboxDefaultPaths().Current)
		// The config files travel in the build context: a multi-line file spliced
		// into a RUN has to escape every newline, and the first version of this
		// render got that wrong — Docker rejected the file outright.
		fmt.Fprintf(&builder, "\nCOPY %s /etc/nginx/conf.d/govard-sandbox.conf\n", sandboxNginxConfFile)
		fmt.Fprintf(&builder, "COPY %s %s\n", sandboxWebInitFile, sandboxFPMServicePath)
		fmt.Fprintf(&builder, "RUN chmod 0755 %s\n", sandboxFPMServicePath)
	}

	fmt.Fprintf(&builder, `
RUN set -eux; \
    mkdir -p /run/sshd; \
    printf '%%s\n' \
      'PasswordAuthentication no' \
      'PermitRootLogin no' \
      'PubkeyAuthentication yes' \
      'AuthorizedKeysFile .ssh/authorized_keys' \
      'UsePAM no' \
      'PrintMotd no' \
      > /etc/ssh/sshd_config.d/govard-sandbox.conf

RUN printf '%%s\n' \
      '#!/bin/sh' \
      'set -e' \
      'for service in $GOVARD_SANDBOX_SERVICES; do' \
      '  if [ -x "/etc/init.d/$service" ]; then /etc/init.d/$service start || true; fi' \
      'done' \
      'exec "$@"' \
      > /usr/local/bin/govard-sandbox-entrypoint; \
    chmod 0755 /usr/local/bin/govard-sandbox-entrypoint

EXPOSE %d%s
ENTRYPOINT ["/usr/local/bin/govard-sandbox-entrypoint"]
CMD ["/usr/sbin/sshd", "-D", "-e"]
`, SandboxSSHPort, exposeWeb)

	return builder.String(), nil
}

// SandboxDockerfileForTest exposes SandboxDockerfile to the tests/ package.
func SandboxDockerfileForTest(spec SandboxSpec) (string, error) {
	return SandboxDockerfile(spec)
}

// SandboxImageTag is the local image name for one spec. The hash of the
// rendered Dockerfile is part of the tag, which is what makes "rebuild only when
// the definition changes" a property of the name rather than of a cache check.
func SandboxImageTag(spec SandboxSpec) (string, error) {
	dockerfile, err := SandboxDockerfile(spec)
	if err != nil {
		return "", err
	}
	resolved, err := ValidateSandboxProfile(spec.Profile)
	if err != nil {
		return "", err
	}
	// The copied files are part of the definition: hashing the Dockerfile alone
	// would reuse an image whose nginx serves a different root.
	definition := dockerfile
	for _, name := range sortedSandboxWords(mapKeys(SandboxBuildFiles(spec))) {
		definition += "\x00" + name + "\x00" + SandboxBuildFiles(spec)[name]
	}
	digest := sha256.Sum256([]byte(definition))
	return fmt.Sprintf("govard-deploy-sandbox:%s-%s-%s",
		sandboxSlug(spec.Project), resolved, hex.EncodeToString(digest[:])[:12]), nil
}

// SandboxImageTagForTest exposes SandboxImageTag to the tests/ package.
func SandboxImageTagForTest(spec SandboxSpec) (string, error) { return SandboxImageTag(spec) }

// SandboxContainerName is the container one project's sandbox runs in.
//
// The name carries a hash of the project *path* as well as its name. Two
// checkouts of the same project would otherwise share one container, and the
// second one's `up` would reuse a container whose mirror is bind-mounted from the
// first checkout's state directory — which surfaces much later as "revision is
// not present in the deploy mirror", long after the cause.
func SandboxContainerName(project, projectRoot string) string {
	name := "govard-" + sandboxSlug(project) + "-deploy-sandbox"
	if root, err := filepath.Abs(filepath.Clean(projectRoot)); err == nil && root != "" {
		sum := sha256.Sum256([]byte(root))
		name += "-" + hex.EncodeToString(sum[:])[:8]
	}
	return name
}

// sandboxSlug reduces a project name to the character set Docker accepts in a
// repository name. A project name comes from `.govard.yml` untouched, so it can
// hold anything a human typed.
func sandboxSlug(project string) string {
	trimmed := strings.ToLower(strings.TrimSpace(project))
	var builder strings.Builder
	previousDash := false
	for _, r := range trimmed {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			previousDash = false
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
			previousDash = false
		default:
			if !previousDash {
				builder.WriteRune('-')
				previousDash = true
			}
		}
	}
	slug := strings.Trim(builder.String(), "-._")
	if slug == "" {
		return "project"
	}
	return slug
}

// mapKeys lists a file map's keys, for a deterministic render order.
func mapKeys(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	return names
}

// quoteWords single-quotes every word for the shell line inside the Dockerfile.
func quoteWords(words []string) []string {
	quoted := make([]string, 0, len(words))
	for _, word := range words {
		quoted = append(quoted, "'"+word+"'")
	}
	return quoted
}

// sortedSandboxWords trims, drops empties and sorts, so the rendered Dockerfile
// does not depend on the order a recipe happened to list its requirements in.
func sortedSandboxWords(words []string) []string {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(words))
	for _, word := range words {
		trimmed := strings.TrimSpace(word)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		cleaned = append(cleaned, trimmed)
	}
	sort.Strings(cleaned)
	return cleaned
}
