# Production image. Build: docker build --build-arg GIT_SHA=$(git rev-parse
# --short HEAD) -t teka-api apps/api
# The same image serves and migrates: default CMD is `serve`; run with
# `migrate up` as a pre-deploy step (migrations are embedded in the binary).
# Digest-pinned for reproducible builds; Dependabot (docker ecosystem) bumps it.
FROM golang:1.27-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS build

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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=build /out/api /api

EXPOSE 8080

ENTRYPOINT ["/api"]
CMD ["serve"]
