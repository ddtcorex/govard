ARG PHP_VERSION=8.4

FROM govard/php:${PHP_VERSION}
ARG PHP_VERSION

USER root

RUN apk add --no-cache nodejs npm curl openssh-client

RUN npm install -g grunt-cli gulp yarn

RUN set -eux \
    && PHP_VERSION=$(php -v | head -n1 | cut -d' ' -f2 | awk -F '.' '{print $1$2}') \
    && if (( ${PHP_VERSION} < 80 )); \
        then MAGERUN_PHAR_URL=https://files.magerun.net/n98-magerun2-7.5.0.phar; \
        else MAGERUN_PHAR_URL=https://files.magerun.net/n98-magerun2.phar; \
    fi \
    && mkdir -p /usr/local/bin \
    && curl -s ${MAGERUN_PHAR_URL} > /usr/local/bin/n98-magerun \
    && chmod +x /usr/local/bin/n98-magerun \
    && ln -s /usr/local/bin/n98-magerun /usr/local/bin/mr

USER www-data
