ARG APACHE_VERSION=2.4.66
FROM httpd:${APACHE_VERSION}-alpine
ARG APACHE_VERSION

LABEL dev.govard.image.name="apache"
LABEL dev.govard.apache.version="${APACHE_VERSION}"

RUN apk add --no-cache bash gettext shadow

COPY etc/httpd.conf /usr/local/apache2/conf/httpd.conf
COPY etc/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /var/www/html
EXPOSE 80

ENTRYPOINT ["entrypoint.sh"]
CMD ["httpd-foreground"]
