# syntax=docker/dockerfile:1

# ---------- build stage ----------
# Builder >= the go.mod version (1.22). Alpine keeps it small.
FROM golang:1.23-alpine AS build
WORKDIR /src

# Download deps first so this layer caches unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Build a fully static binary (CGO off) so it runs in a minimal scratch image.
# -trimpath + -ldflags strip paths and debug info for a smaller, cleaner binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- run stage ----------
# distroless/static = no shell, no package manager, ~2MB base. Runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server

# Informational; the server actually binds to $PORT (Render sets it).
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
