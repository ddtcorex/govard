FROM debian:13-slim
LABEL dev.govard.image.name="sshd"

RUN apt-get update \
    && apt-get install -y --no-install-recommends openssh-server \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd

COPY sshd-gateway.conf /etc/ssh/sshd_config.d/gateway.conf
COPY entrypoint.sh authorized-keys-command.sh gw-router.sh /usr/local/bin/
RUN chmod 0755 /usr/local/bin/entrypoint.sh /usr/local/bin/authorized-keys-command.sh /usr/local/bin/gw-router.sh

EXPOSE 22
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/sbin/sshd", "-D", "-e"]
