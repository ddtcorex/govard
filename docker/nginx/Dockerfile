ARG NGINX_VERSION=1.28.0
FROM nginx:${NGINX_VERSION}-alpine

LABEL dev.govard.image.name="nginx"
LABEL dev.govard.nginx.version="${NGINX_VERSION}"

# Copy custom nginx configuration
COPY etc/nginx.conf /etc/nginx/nginx.conf
COPY etc/conf.d/default.conf /etc/nginx/conf.d/default.conf
COPY etc/fastcgi_params /etc/nginx/fastcgi_params
COPY etc/templates /etc/nginx/templates
COPY etc/entrypoint.sh /usr/local/bin/entrypoint.sh

RUN apk add --no-cache shadow gettext \
    && chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /var/www/html
EXPOSE 80

ENTRYPOINT ["entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
