# Stage 1 — build the React SPA.
# Cached separately from the Go build so a frontend-only change does not
# invalidate the Go layer and vice-versa.
FROM node:24-alpine AS frontend
WORKDIR /app
# vp is the toolchain (it carries vitest, oxlint, oxfmt and the bundler), so the
# image installs it directly rather than a package manager whose only job is to
# fetch it.
#
# The version is read out of package.json rather than pinned here. The installer
# defaults to latest, which would drift on every rebuild, and a second literal
# pin in this file would drift the other way: dependabot bumps the dependency and
# would leave the Dockerfile behind.
COPY frontend/pnpm-workspace.yaml frontend/package.json frontend/pnpm-lock.yaml ./
RUN apk add --no-cache curl bash \
    && VP_VERSION="$(node -p "require('./package.json').devDependencies['vite-plus'].replace(/^[^0-9]*/, '')")" \
       curl -fsSL https://vite.plus | bash
ENV PATH="/root/.vite-plus/bin:${PATH}"
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
