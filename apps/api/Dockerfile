# Production image. Build: docker build --build-arg GIT_SHA=$(git rev-parse
# --short HEAD) -t teka-api apps/api
# The same image serves and migrates: default CMD is `serve`; run with
# `migrate up` as a pre-deploy step (migrations are embedded in the binary).
# Digest-pinned for reproducible builds; Dependabot (docker ecosystem) bumps it.
FROM golang:1.25-alpine@sha256:56961d79ea8129efddcc0b8643fd8a5416b4e6228cfd477e3fd61deb2672c587 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# docs/ is excluded by .dockerignore, so the swagger package is always
# regenerated from the annotations in this source tree — never stale.
RUN go tool swag init -g cmd/api/main.go -o docs --parseInternal

ARG GIT_SHA=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${GIT_SHA}" \
    -o /out/api ./cmd/api

# Static binary, no shell, non-root user baked into the base image.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:1b7b9f0f0e0a1d2155f531db587cc48ec26aaf97ab64364225f5bf18a054e66a

COPY --from=build /out/api /api

EXPOSE 8080

ENTRYPOINT ["/api"]
CMD ["serve"]
