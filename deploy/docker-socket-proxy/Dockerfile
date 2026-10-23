# Preserve tecnativa's documented read-only Docker API filter behavior.
# The upstream v0.5.0 image has fixable HIGH package advisories.
# Upgrade Alpine packages at build time; audit the resulting image, not the upstream tag.
FROM tecnativa/docker-socket-proxy:v0.5.0
USER root
RUN apk upgrade --no-cache
