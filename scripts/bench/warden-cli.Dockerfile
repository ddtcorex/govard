# Warden CLI in a container: its installer writes to /etc/ssh, chowns a tunnel key
# and trusts a CA, all of which need root. Inside this image we are root, and the
# only host surface is the Docker socket, so the stack it starts is still real.
FROM docker:27-cli

RUN apk add --no-cache bash git openssh-client curl jq python3 docker-cli-compose openssl gettext

WORKDIR /opt
RUN git clone --depth 1 https://github.com/wardenenv/warden.git /opt/warden

RUN ln -s /opt/warden/bin/warden /usr/local/bin/warden
# Inside the container we are already root, so the install's privileged steps are
# plain commands. A shim keeps Warden's script unmodified instead of patching it.
RUN printf '#!/bin/sh\nexec "$@"\n' > /usr/local/bin/sudo && chmod +x /usr/local/bin/sudo
ENTRYPOINT ["/bin/bash", "-lc"]
