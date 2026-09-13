package deploy

import (
	"fmt"
	"strings"
)

// A sandbox that cannot serve the release cannot rehearse the last step of the
// deploy. `deploy:verify` proves the live target answers over HTTP when the
// remote sets `deploy.verify.url`, and without a web server in the container that
// check has only ever been exercised against a stub — the one part of the
// pipeline a rehearsal could not reach. This file renders the web tier: nginx
// serving the served path plus the project's web root (`/pub` for a storefront that
// serves from a subdirectory), and
// PHP-FPM running as the deployer so the application can write the directories
// `deploy:writable` handed to that user.
//
// The tiers are baked into the image rather than configured per run, for the same
// reason the PHP series is: the image tag is the hash of the rendered definition,
// so two sandboxes that serve differently are two images rather than one image
// behaving differently on the day.

// sandboxWebService is the single service name the entrypoint starts for the web
// tier. One name keeps the service list independent of the PHP series — the init
// script behind it finds whichever `php<series>-fpm` the distribution installed.
const sandboxWebService = "govard-sandbox-web"

// sandboxFPMServicePath is that init script.
const sandboxFPMServicePath = "/etc/init.d/" + sandboxWebService

// sandboxFPMSocket is the socket nginx talks to. It is named rather than derived
// from the PHP version so the nginx server block and the FPM pool agree without
// either of them knowing which series was installed.
const sandboxFPMSocket = "/run/php/govard-sandbox-fpm.sock"

// sandboxWebPort is the port the web tier listens on inside the container. The
// host side is chosen by Docker, exactly as the SSH port is.
const sandboxWebPort = 80

// The two files the web tier is built from. They are separate build-context files
// rather than text spliced into the Dockerfile: a multi-line file inside a RUN
// has to end every line with a backslash, and getting that wrong produces a
// Dockerfile Docker refuses — which is exactly what the first attempt did.
const (
	sandboxNginxConfFile = "govard-sandbox-nginx.conf"
	sandboxWebInitFile   = "govard-sandbox-web"
)

// SandboxWebBinding is how the web port is published — loopback, ephemeral host
// port, so a sandbox never collides with a development server on the same machine.
const SandboxWebBinding = "127.0.0.1::80"

// SandboxServesWeb reports whether a profile ships the web tier.
//
// Only the profiles that carry PHP: nginx without an interpreter would serve
// the application's `index.php` as a download, which is worse than no web server,
// because a verify check would then fail for a reason that looks like a defect.
func SandboxServesWeb(profile string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case SandboxProfilePHP, SandboxProfileFull:
		return true
	default:
		return false
	}
}

// SandboxWebPackages are the packages the web tier needs, given the PHP series
// the image provides. An empty series means the distribution's own PHP, whose FPM
// package is unversioned.
func SandboxWebPackages(series string) []string {
	if strings.TrimSpace(series) == "" {
		return []string{"nginx", "php-fpm"}
	}
	return []string{"nginx", "php" + series + "-fpm"}
}

// NormalizeSandboxWebRoot turns a project's `stack.web_root` into the path suffix
// the web server serves from: `/pub` for a subdirectory-served project, `/` when
// nothing is configured.
func NormalizeSandboxWebRoot(webRoot string) string {
	trimmed := strings.TrimSpace(webRoot)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return strings.TrimRight(trimmed, "/")
}

// SandboxWebDocumentRoot is the directory nginx serves: the path the target
// serves from, plus the project's web root inside it. `current` is a symlink to
// whatever release is live, and nginx resolves it per request, so a swap moves
// the served application with it.
func SandboxWebDocumentRoot(webRoot string) string {
	normalized := NormalizeSandboxWebRoot(webRoot)
	if normalized == "/" {
		return SandboxDefaultPaths().Current
	}
	return SandboxDefaultPaths().Current + normalized
}

// sandboxNginxConfig renders the server block.
//
// `try_files ... /index.php` and the PHP location are the two lines that make it
// a front controller rather than a file server: without them every extensionless
// request 404s and every .php file is offered as a download.
func sandboxNginxConfig(webRoot string) string {
	return fmt.Sprintf(`server {
    listen %d default_server;
    listen [::]:%d default_server;
    server_name _;

    root %s;
    index index.php index.html;

    client_max_body_size 64m;

    location / {
        try_files $uri $uri/ /index.php$is_args$args;
    }

    location ~ \.php$ {
        include fastcgi_params;
        fastcgi_pass unix:%s;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_read_timeout 120s;
    }

    location ~* \.(js|css|png|jpg|jpeg|gif|svg|ico|woff|woff2|ttf|eot)$ {
        expires 1h;
        access_log off;
    }
}
`, sandboxWebPort, sandboxWebPort, SandboxWebDocumentRoot(webRoot), sandboxFPMSocket)
}

// SandboxBuildFiles are the extra files the image build copies in, keyed by the
// name they are written under in the build context. Empty for a profile with no
// web tier.
//
// They are part of the image definition, not an implementation detail: the tag is
// the hash of the Dockerfile *and* of these, so a change to the served root or to
// the pool cannot silently reuse an image built with the old one.
func SandboxBuildFiles(spec SandboxSpec) map[string]string {
	resolved, err := ValidateSandboxProfile(spec.Profile)
	if err != nil || !SandboxServesWeb(resolved) {
		return nil
	}
	return map[string]string{
		sandboxNginxConfFile: sandboxNginxConfig(spec.WebRoot),
		sandboxWebInitFile:   sandboxFPMInitScript(),
	}
}

// sandboxFPMInitScript starts whichever FPM the distribution installed and then
// nginx. It exists so the entrypoint's service list stays one name for every PHP
// series: the pool file is versioned, the service is not.
func sandboxFPMInitScript() string {
	return `#!/bin/sh
# Starts the sandbox web tier: the distribution's PHP-FPM, then nginx.
case "$1" in
  start)
    for unit in /etc/init.d/php*-fpm; do
      [ -x "$unit" ] || continue
      "$unit" start
    done
    /usr/sbin/nginx
    ;;
  stop)
    /usr/sbin/nginx -s stop 2>/dev/null || true
    for unit in /etc/init.d/php*-fpm; do
      [ -x "$unit" ] || continue
      "$unit" stop
    done
    ;;
esac
exit 0
`
}

// sandboxFPMPoolPatch rewrites the parts of the distribution's pool that a
// sandbox has to change, on every pool file it finds.
//
// The pool runs as the deployer: `deploy:writable` chowns `var`, `pub/static`,
// `generated` and `app/etc` to that user, and an interpreter running as
// `www-data` could not write them — a page render would fail with a permission
// error that no real target would produce. The socket keeps a `www-data` group so
// nginx, which does run as `www-data`, can connect to it.
func sandboxFPMPoolPatch() string {
	return fmt.Sprintf(`set -eux; \
    rm -f /etc/nginx/sites-enabled/default; \
    for pool in /etc/php/*/fpm/pool.d/*.conf; do \
      sed -i \
        -e 's|^user = .*|user = %s|' \
        -e 's|^group = .*|group = %s|' \
        -e 's|^listen = .*|listen = %s|' \
        -e 's|^;\?listen.owner = .*|listen.owner = %s|' \
        -e 's|^;\?listen.group = .*|listen.group = www-data|' \
        -e 's|^;\?listen.mode = .*|listen.mode = 0660|' \
        -e 's|^;\?clear_env = .*|clear_env = no|' \
        "$pool"; \
    done`,
		SandboxUser, SandboxUser, sandboxFPMSocket, SandboxUser)
}
