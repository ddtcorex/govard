# syntax=docker/dockerfile:1
#
# Govard-owned Magento lint runtime.
#
# Stage "phpbase" installs every PHP CLI Govard supports for Magento lint work
# (7.4 and 8.0 through 8.5) plus Composer 2. Stage "runtime" installs the seven
# committed analyzer toolchains from their locks and adds the Govard runner.
#
# The same "phpbase" stage generates the committed locks:
#   docker build --target phpbase -t govard-local/magelint-phpbase docker/audit-magento
#   docker run --rm -v "$PWD/docker/audit-magento/toolchains":/toolchains \
#     govard-local/magelint-phpbase sh -c '...composer update --no-install...'

ARG COMPOSER_IMAGE=composer:2

FROM ${COMPOSER_IMAGE} AS composer_source

FROM ubuntu:24.04 AS phpbase

ARG DEBIAN_FRONTEND=noninteractive
# The single declaration of the PHP policy in this image: exactly the
# versions Govard supports for Magento lint work. Both build stages and the
# image label derive from it, so the list cannot drift within the image.
ARG GOVARD_MAGELINT_PHP_VERSIONS="7.4,8.0,8.1,8.2,8.3,8.4,8.5"
ENV GOVARD_MAGELINT_PHP_VERSIONS="${GOVARD_MAGELINT_PHP_VERSIONS}"
ENV COMPOSER_ALLOW_SUPERUSER=1
# QEMU-emulated arm64 builds run Composer's usual parallel downloads (default
# COMPOSER_MAX_PARALLEL_HTTP=12) through a software-emulated CPU; several
# concurrent TLS handshakes then compete for the same emulated cycles and can
# all miss their connect timeout together (seen as simultaneous "SSL
# connection timeout" failures across unrelated packages). Capping this at 1
# keeps each handshake within its timeout regardless of host emulation speed.
ENV COMPOSER_MAX_PARALLEL_HTTP=1

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        gnupg \
        openssh-client \
        rsync \
        software-properties-common \
        unzip; \
    add-apt-repository -y ppa:ondrej/php; \
    apt-get update; \
    for version in $(printf '%s' "${GOVARD_MAGELINT_PHP_VERSIONS}" | tr ',' ' '); do \
        packages="php${version}-cli php${version}-mbstring php${version}-xml php${version}-curl php${version}-zip php${version}-intl php${version}-bcmath php${version}-gd"; \
        case "${version}" in \
            7.*) packages="${packages} php${version}-json" ;; \
        esac; \
        apt-get install -y --no-install-recommends ${packages}; \
    done; \
    apt-get purge -y --auto-remove software-properties-common gnupg; \
    rm -rf /var/lib/apt/lists/*; \
    for version in $(printf '%s' "${GOVARD_MAGELINT_PHP_VERSIONS}" | tr ',' ' '); do \
        "php${version}" --version >/dev/null; \
    done; \
    ! command -v php7.3

COPY --from=composer_source /usr/bin/composer /usr/local/bin/composer

RUN set -eux; \
    chmod 0755 /usr/local/bin/composer; \
    php8.3 /usr/local/bin/composer --version

FROM phpbase AS runtime

ARG GOVARD_VERSION="dev"
ARG GOVARD_CONTEXT_DIGEST="unknown"
ARG GOVARD_SOURCE_REVISION="unknown"

LABEL org.opencontainers.image.title="magelint" \
      org.opencontainers.image.description="Govard-owned Magento static-analysis runtime for PHP 7.4 and 8.0-8.5" \
      org.opencontainers.image.source="https://github.com/ddtcorex/govard" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.revision="${GOVARD_SOURCE_REVISION}" \
      org.opencontainers.image.version="${GOVARD_VERSION}" \
      io.govard.version="${GOVARD_VERSION}" \
      io.govard.audit.context-digest="${GOVARD_CONTEXT_DIGEST}" \
      io.govard.audit.report-schema="2" \
      io.govard.audit.php-versions="${GOVARD_MAGELINT_PHP_VERSIONS}"

COPY toolchains /opt/govard/toolchains

RUN set -eux; \
    for version in $(printf '%s' "${GOVARD_MAGELINT_PHP_VERSIONS}" | tr ',' ' '); do \
        cd "/opt/govard/toolchains/php-${version}"; \
        attempt=1; \
        until "php${version}" /usr/local/bin/composer install \
            --no-interaction --no-progress --no-dev --optimize-autoloader; do \
            if [ "${attempt}" -ge 3 ]; then \
                echo "composer install for php${version} failed after ${attempt} attempts" >&2; \
                exit 1; \
            fi; \
            sleep "$(( attempt * 10 ))"; \
            attempt=$(( attempt + 1 )); \
        done; \
        "php${version}" "/opt/govard/toolchains/php-${version}/vendor/bin/phpcs" -i | grep -q Magento2; \
        "php${version}" "/opt/govard/toolchains/php-${version}/vendor/bin/phpstan" --version >/dev/null; \
    done; \
    rm -rf /root/.cache/composer /root/.composer

RUN composer global config --no-plugins allow-plugins.dealerdirect/phpcodesniffer-composer-installer true && composer global require wp-coding-standards/wpcs:^3.1 dealerdirect/phpcodesniffer-composer-installer:^1.0 --no-interaction \
 && phpcs --config-set installed_paths $(composer global config home)/vendor/wp-coding-standards/wpcs,$(composer global config home)/vendor/squizlabs/php_codesniffer/src/Standards || $(composer global config home)/vendor/bin/phpcs --config-set installed_paths $(composer global config home)/vendor/wp-coding-standards/wpcs,$(composer global config home)/vendor/squizlabs/php_codesniffer/src/Standards
RUN composer global require szepeviktor/phpstan-wordpress phpstan/phpstan-symfony:^1.3 --no-interaction

COPY bin/report.php /usr/local/lib/govard/report.php
COPY bin/magelint /usr/local/bin/magelint

RUN set -eux; \
    chmod 0755 /usr/local/bin/magelint; \
    chmod 0644 /usr/local/lib/govard/report.php; \
    mkdir -p /source /cache /output; \
    chmod 0777 /cache /output

ENV GOVARD_LINT_TOOLCHAIN_DIR=/opt/govard/toolchains \
    GOVARD_LINT_REPORT_SCRIPT=/usr/local/lib/govard/report.php \
    GOVARD_LINT_COMPOSER=/usr/local/bin/composer

WORKDIR /source

ENTRYPOINT ["/usr/local/bin/magelint"]
