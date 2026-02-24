ARG ELASTICSEARCH_VERSION=7.17.10
FROM docker.elastic.co/elasticsearch/elasticsearch:${ELASTICSEARCH_VERSION}

LABEL dev.govard.image.name="elasticsearch"
LABEL dev.govard.elasticsearch.version="${ELASTICSEARCH_VERSION}"

RUN bin/elasticsearch-plugin install --batch analysis-icu || true
RUN bin/elasticsearch-plugin install --batch analysis-phonetic || true
