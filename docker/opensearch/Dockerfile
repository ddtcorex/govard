ARG OPENSEARCH_VERSION=2.11.0
FROM opensearchproject/opensearch:${OPENSEARCH_VERSION}

LABEL dev.govard.image.name="opensearch"
LABEL dev.govard.opensearch.version="${OPENSEARCH_VERSION}"

RUN bin/opensearch-plugin install --batch analysis-icu || true
RUN bin/opensearch-plugin install --batch analysis-phonetic || true
