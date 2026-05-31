# Stage 1 — build the React SPA.
# Cached separately from the Go build so a frontend-only change does not
# invalidate the Go layer and vice-versa.
FROM node:24-alpine AS frontend
WORKDIR /app
RUN corepack enable
COPY frontend/pnpm-workspace.yaml frontend/package.json frontend/pnpm-lock.yaml ./
RUN corepack prepare pnpm@11.5.0 --activate && pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

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
