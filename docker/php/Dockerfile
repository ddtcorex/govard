ARG PHP_VERSION=8.4
ARG PHP_MEMORY_LIMIT=4G
ARG PHP_MAX_EXECUTION_TIME=3600

FROM php:${PHP_VERSION}-fpm-alpine
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
    ICU_DATA_FULL=""; \
    if [ "$PHP_VERSION_INT" -ge 81 ]; then ICU_DATA_FULL="icu-data-full"; fi; \
    ONIGURUMA=""; \
    if [ "$PHP_VERSION_INT" -ge 74 ]; then ONIGURUMA="oniguruma-dev"; fi; \
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
    openssl-dev \
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
    sudo \
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
    docker-php-ext-install -j$(nproc) \
    bcmath \
    bz2 \
    calendar \
    exif \
    gd \
    gettext \
    gmp \
    intl \
    mysqli \
    opcache \
    pdo_mysql \
    soap \
    sockets \
    sodium \
    xsl \
    zip \
    pcntl \
    ftp

# Install PECL extensions
RUN set -eux; \
    PHP_VERSION_INT=$(echo $PHP_VERSION | cut -d. -f1,2 | tr -d .); \
    REDIS_VERSION="redis"; \
    if [ "$PHP_VERSION_INT" -lt 72 ]; then REDIS_VERSION="redis-4.3.0"; fi; \
    pecl install $REDIS_VERSION imagick amqp \
    && docker-php-ext-enable redis imagick amqp

# Install Composer
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer

# Install Mailpit sendmail as a replacement for mhsendmail
RUN curl -sL https://raw.githubusercontent.com/axllent/mailpit/develop/install.sh | sh

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
