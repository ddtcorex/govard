ARG ELASTICSEARCH_VERSION=7.17.10
ARG ELASTICSEARCH_IMAGE=docker.elastic.co/elasticsearch/elasticsearch
FROM ${ELASTICSEARCH_IMAGE}:${ELASTICSEARCH_VERSION}
ARG ELASTICSEARCH_VERSION

LABEL dev.govard.image.name="elasticsearch"
LABEL dev.govard.elasticsearch.version="${ELASTICSEARCH_VERSION}"

RUN bin/elasticsearch-plugin install --batch analysis-icu || true
RUN bin/elasticsearch-plugin install --batch analysis-phonetic || true
