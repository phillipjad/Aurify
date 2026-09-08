# Stage 1 — build the React SPA.
# Cached separately from the Go build so a frontend-only change does not
# invalidate the Go layer and vice-versa.
FROM node:24-alpine AS frontend
WORKDIR /app
# vp is the toolchain (it carries vitest, oxlint, oxfmt and the bundler), so the
# image installs it directly rather than a package manager whose only job is to
# fetch it.
#
# The version is read out of the pnpm catalog rather than pinned here, so a
# toolchain bump moves the image with it. package.json only says `catalog:`;
# pnpm-workspace.yaml holds the number. Empty is a hard failure, because the
# fallback would be installing whatever is latest, which is the drift this is
# here to prevent.
#
# npm rather than the installer at https://vite.plus, because npm is already in
# this base image and puts vp on the default PATH. The installer resolves its own
# install directory; when that directory moved, the PATH set here pointed at
# nothing and the image stopped building on a commit that had not changed.
COPY frontend/pnpm-workspace.yaml frontend/package.json frontend/pnpm-lock.yaml ./
RUN VP_VERSION="$(sed -n 's/^[[:space:]]*vite-plus:[[:space:]]*//p' pnpm-workspace.yaml | head -1)"; \
    if [ -z "$VP_VERSION" ]; then echo "no vite-plus version in the pnpm-workspace.yaml catalog" >&2; exit 1; fi; \
    npm install --global --no-fund --no-audit "vite-plus@$VP_VERSION"
# pnpm-workspace.yaml declares a patched dependency, so install reads the patch
# file itself and fails with ENOENT without it. It travels with the manifests
# rather than with the sources on the line below, because it is an input to this
# install step (see docs/adr/0021-patched-sonner-unmount-delay.md).
COPY frontend/patches ./patches/
RUN vp install
COPY frontend/ ./
RUN vp build

# Stage 2 — compile the Go binary with the frontend embedded.
FROM golang:1.26-alpine AS backend
ARG VERSION=dev
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Overwrite the .gitkeep placeholder with the real compiled assets so that
# //go:embed all:ui picks them up during the Go build.
COPY --from=frontend /app/dist/ ./internal/transport/http/ui/
RUN CGO_ENABLED=0 go build \
    -ldflags="-X main.version=${VERSION}" \
    -o /aurify \
    ./cmd/api

# Stage 3 — minimal runtime image.
FROM gcr.io/distroless/static-debian12
COPY --from=backend /aurify /aurify
EXPOSE 8080
ENTRYPOINT ["/aurify"]
