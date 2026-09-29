# Production image: static Vite build served by unprivileged nginx (runs as
# uid 101, listens on 8080 — non-root images cannot bind 80).
# VITE_API_URL is baked into the bundle at build time; the build fails loudly
# if it is missing or malformed (src/lib/config/env.ts validates at boot).
# VITE_PUBLIC_ORIGIN (the web app's own origin) makes the og:image link-preview
# URL in index.html absolute; it is required too.
#   docker build --build-arg VITE_API_URL=https://api.example.com/api/v1 \
#     --build-arg VITE_PUBLIC_ORIGIN=https://app.example.com -t teka-web apps/web
# Digest-pinned for reproducible builds; Dependabot (docker ecosystem) bumps it.
FROM node:26-alpine@sha256:ef24c5053d50fdc3e4e56eb4e7ddb7861874ab0fdc797046ba897581deb8e868 AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

ARG VITE_API_URL
# env.ts only validates in the browser at boot; without this guard a missing
# build-arg produces a green build that white-screens at runtime.
RUN test -n "${VITE_API_URL}" || { echo "VITE_API_URL build-arg is required" >&2; exit 1; }
ENV VITE_API_URL=${VITE_API_URL}
ARG VITE_PUBLIC_ORIGIN
# Vite leaves an undefined %VITE_PUBLIC_ORIGIN% verbatim in index.html.
RUN test -n "${VITE_PUBLIC_ORIGIN}" || { echo "VITE_PUBLIC_ORIGIN build-arg is required" >&2; exit 1; }
ENV VITE_PUBLIC_ORIGIN=${VITE_PUBLIC_ORIGIN}
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.31-alpine@sha256:2ddec616f1cb58bcac057aa388f28cb81e35137641ef4226d321714499329bd1

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY nginx-security-headers.conf /etc/nginx/snippets/teka-security-headers.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 8080
