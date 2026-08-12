ARG PHP_VERSION=8.4
ARG PHP_BASE_TAG=${PHP_VERSION}-fpm-alpine
ARG PHP_MEMORY_LIMIT=4G
ARG PHP_MAX_EXECUTION_TIME=3600

# PHP_BASE_TAG lets a consumer of this Dockerfile override the upstream base tag
# (e.g. to pin a specific Alpine release) without duplicating this file. See the
# "php-magento2-base" target in docker-bake.hcl, which pins Alpine 3.19 to work
# around a Magento-2-specific libxml2 issue.
FROM php:${PHP_BASE_TAG}
ARG PHP_VERSION
ARG PHP_MEMORY_LIMIT
ARG PHP_MAX_EXECUTION_TIME

LABEL dev.govard.image.name="php"
LABEL dev.govard.php.version="${PHP_VERSION}"

# Set environment variables
ENV COMPOSER_ALLOW_SUPERUSER=1
ENV PHP_MEMORY_LIMIT=${PHP_MEMORY_LIMIT}
ENV PHP_MAX_EXECUTION_TIME=${PHP_MAX_EXECUTION_TIME}

# Install system dependencies
RUN set -eux; \
    PHP_VERSION_INT=$(echo $PHP_VERSION | cut -d. -f1,2 | tr -d .); \
    apk update; \
    ICU_DATA_FULL=$(apk search -q icu-data-full); \
    ONIGURUMA=""; \
    if [ "$PHP_VERSION_INT" -ge 74 ]; then ONIGURUMA="oniguruma-dev"; fi; \
    SSL_LIB="openssl-dev"; \
    if [ "$PHP_VERSION_INT" -lt 72 ]; then SSL_LIB="libressl-dev"; fi; \
    CRON_PKG="cronie"; \
    if [ "$PHP_VERSION_INT" -lt 73 ]; then CRON_PKG="dcron"; fi; \
    apk add --no-cache \
    freetype-dev \
    libjpeg-turbo-dev \
    libpng-dev \
    libzip-dev \
    libxml2-dev \
    icu-dev \
    $ICU_DATA_FULL \
    libxslt-dev \
    libgcrypt-dev \
    $ONIGURUMA \
    gmp-dev \
    bzip2-dev \
    gettext-dev \
    $SSL_LIB \
    libsodium-dev \
    rabbitmq-c-dev \
    imagemagick-dev \
    libtool \
    make \
    gcc \
    g++ \
    autoconf \
    git \
    unzip \
    bash \
    ca-certificates \
    sudo \
    $CRON_PKG \
    linux-headers \
    patch \
    openssh-client \
    shadow \
    pv \
    socat \
    bind-tools \
    mariadb-client \
    postgresql-client \
    jq

# Install PHP extensions
RUN set -eux; \
    PHP_VERSION_INT=$(echo $PHP_VERSION | cut -d. -f1,2 | tr -d .); \
    if [ "$PHP_VERSION_INT" -ge 74 ]; then \
    docker-php-ext-configure gd --with-freetype --with-jpeg; \
    else \
    docker-php-ext-configure gd --with-freetype-dir=/usr/include/ --with-jpeg-dir=/usr/include/; \
    fi; \
    if [ "$PHP_VERSION_INT" -ge 72 ]; then \
    libsodium_opt="sodium"; \
    else \
    libsodium_opt=""; \
    fi; \
    WANTED_EXTS="bcmath bz2 calendar exif gd gettext gmp intl mysqli opcache pdo_mysql soap sockets xsl zip pcntl ftp $libsodium_opt"; \
    MISSING_EXTS=""; \
    for ext in $WANTED_EXTS; do \
    if ! php -m | grep -qiE "^($ext|Zend $ext)$"; then \
    MISSING_EXTS="$MISSING_EXTS $ext"; \
    fi; \
    done; \
    if [ -n "$MISSING_EXTS" ]; then \
    docker-php-ext-install -j$(nproc) $MISSING_EXTS; \
    fi

# Install PECL extensions
RUN set -eux; \
    PHP_VERSION_INT=$(echo $PHP_VERSION | cut -d. -f1,2 | tr -d .); \
    REDIS_VERSION="redis"; \
    IMAGICK_VERSION="imagick"; \
    AMQP_VERSION="amqp"; \
    if [ "$PHP_VERSION_INT" -lt 74 ]; then \
    REDIS_VERSION="redis-5.3.7"; \
    AMQP_VERSION="amqp-1.11.0"; \
    fi; \
    if [ "$PHP_VERSION_INT" -lt 72 ]; then \
    REDIS_VERSION="redis-4.3.0"; \
    fi; \
    if [ "$PHP_VERSION_INT" -lt 73 ]; then \
    IMAGICK_VERSION="imagick-3.4.4"; \
    fi; \
    if [ "$PHP_VERSION_INT" -lt 70 ]; then \
    REDIS_VERSION="redis-2.2.8"; \
    IMAGICK_VERSION="imagick-3.4.4"; \
    AMQP_VERSION="amqp-1.9.4"; \
    fi; \
    pecl install $REDIS_VERSION $IMAGICK_VERSION $AMQP_VERSION; \
    if [ "$PHP_VERSION_INT" -ge 70 ] && [ "$PHP_VERSION_INT" -lt 72 ]; then \
    pecl install libsodium; \
    docker-php-ext-enable sodium; \
    fi; \
    docker-php-ext-enable redis imagick amqp

# Install Composer
COPY --from=composer:1 /usr/bin/composer /usr/local/bin/composer1
COPY --from=composer:2 /usr/bin/composer /usr/local/bin/composer2
COPY --from=composer:2.2 /usr/bin/composer /usr/local/bin/composer2lts

RUN set -eux; \
    PHP_VERSION_INT=$(echo $PHP_VERSION | cut -d. -f1,2 | tr -d .); \
    if [ "$PHP_VERSION_INT" -lt 73 ]; then \
        ln -sf /usr/local/bin/composer2lts /usr/local/bin/composer; \
    else \
        ln -sf /usr/local/bin/composer2 /usr/local/bin/composer; \
    fi

# Install Mailpit sendmail as a replacement for mhsendmail
ARG TARGETARCH
RUN set -eux; \
    case "${TARGETARCH}" in \
        amd64) ARCH='amd64' ;; \
        arm64) ARCH='arm64' ;; \
        *) ARCH='amd64' ;; \
    esac; \
    curl -sL https://github.com/axllent/mailpit/releases/download/v1.23.1/mailpit-linux-${ARCH}.tar.gz | tar zxf - -C /usr/local/bin mailpit

# Copy custom PHP configuration
COPY etc/php.ini /usr/local/etc/php/conf.d/govard.ini
COPY etc/entrypoint.sh /usr/local/bin/entrypoint.sh

# Configure PHP-FPM for Docker
RUN set -eux; \
    cd /usr/local/etc/php-fpm.d; \
    { \
    echo '[global]'; \
    echo 'error_log = /proc/self/fd/2'; \
    echo; \
    echo '[www]'; \
    echo 'access.log = /proc/self/fd/2'; \
    echo 'clear_env = no'; \
    echo 'catch_workers_output = yes'; \
    echo 'decorate_workers_output = no'; \
    } | tee docker-tuned.conf

# Setup working directory
WORKDIR /var/www/html

# Create a non-root user that matches host typical UID/GID
RUN if getent group www-data >/dev/null 2>&1; then groupmod -g 1000 www-data; else addgroup -g 1000 -S www-data; fi \
    && if id -u www-data >/dev/null 2>&1; then usermod -u 1000 -g 1000 www-data; else adduser -u 1000 -S -G www-data www-data; fi \
    && echo "www-data ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers \
    && chmod +x /usr/local/bin/entrypoint.sh

USER www-data

ENTRYPOINT ["entrypoint.sh"]
CMD ["php-fpm"]
