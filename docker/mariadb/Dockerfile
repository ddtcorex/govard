ARG MARIADB_VERSION=11.4
FROM mariadb:${MARIADB_VERSION}

LABEL dev.govard.image.name="mariadb"
LABEL dev.govard.mariadb.version="${MARIADB_VERSION}"

SHELL [ "/usr/bin/env", "bash", "-c" ]

RUN <<EOF
    if [[ ${MARIADB_VERSION} =~ ^1:11 ]]; then
        apt-get update
        apt-get install -y mariadb-server-compat mariadb-client-compat
        rm -rf /var/lib/apt/lists/*
    fi
EOF
