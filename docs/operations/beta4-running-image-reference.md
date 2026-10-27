# Diagnosing DockLantern's own image reference on a test VPS

DockLantern exposes **three different pieces of information**, which are not interchangeable:

1. **Running app build:** Version embedded by the Dockerfile's `VERSION` build argument. See sidebar `Running app:`, startup logs, or `docklantern-inventory -version`. A locally built binary may intentionally say `dev`.
2. **Docker-observed container image:** The image reference captured from `GET /containers/{id}/json`, `Config.Image`, when Docker created the running container. A `docker pull` followed by `docker restart` does **not** recreate that container or change this reference.
3. **Registry watch image:** The reference in DockLantern's persisted monitoring policy. A legacy manually managed beta.2 watch can deliberately remain different from the image used by the current Docker container.

The beta.4 Services/Service Details pages now label the second and third separately; the sidebar labels the running app build. The latest Docker-observed value is derived from the most recent successful inventory scan, and its UTC observation time appears on Service Details. **A successful pull is not evidence the container runs the pulled image.**

## Check the test VPS safely

From the *actual DockLantern deployment working directory*, using all Compose overlays that normally start it:

```sh
CID=$(docker compose -f compose.yaml -f compose.discovery.yaml ps -q docklantern)
test -n "$CID" || { echo "DockLantern service container not found"; exit 1; }

# Actual Docker container creation-time reference and immutable image ID:
docker inspect "$CID" --format 'Created with: {{.Config.Image}}
Image ID: {{.Image}}
Created at: {{.Created}}'

# Actual release/build version inside the running binary:
docker exec "$CID" /usr/local/bin/docklantern-inventory -version

# References in the current effective Compose config:
docker compose -f compose.yaml -f compose.discovery.yaml config --images
```

If your local deployment uses another overlay (for example, `compose.projects.yaml`), include it in **both** Compose commands above. Avoid posting `docker compose config` with unredacted environment variables or secrets.

**Interpretation:**
- Build says beta.4 but `Config.Image` says beta.2: investigate how the image was built, retagged or recreated; Docker's reference may still be beta.2 even with newer binary contents.
- Build and `Config.Image` both say beta.2: the running container is not the intended beta.4 build.
- Docker inspect reports beta.4, but DockLantern's Service Details shows beta.2 with an old observation time: discovery may be disabled or failing; inspect `docker_discovery` worker status and the private GET-only proxy.
- Docker-observed image is beta.4 but registry watch image says beta.2: a legacy monitoring policy may remain pinned to beta.2; update that policy separately, without treating it as deployment authorization.

Do not infer the running image from an image tag floating in a registry or from a dashboard status alone. For immutable confirmation, compare `docker inspect .Image` with the ID of the intended locally pulled image. Recreating the **DockLantern service only** after an approved configuration change may be necessary; avoid restarting or redeploying other monitored services.

The checked-in `docklantern-deploy/compose.yaml` may lag the operator's test VPS overrides, so verify the local effective Compose config before altering anything.
