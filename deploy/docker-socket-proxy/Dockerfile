# Preserve tecnativa's documented read-only Docker API filter behavior.
# The upstream v0.5.0 image has fixable HIGH package advisories.
# Upgrade Alpine packages at build time; audit the resulting image, not the upstream tag.
FROM tecnativa/docker-socket-proxy:v0.5.0@sha256:1f5038b54f06c3e18422902cf00ba21803d1c97805aae032e5e6673d532d3459
USER root
RUN apk upgrade --no-cache
