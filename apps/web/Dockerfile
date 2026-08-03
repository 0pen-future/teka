# Production image: static Vite build served by unprivileged nginx (runs as
# uid 101, listens on 8080 — non-root images cannot bind 80).
# VITE_API_URL is baked into the bundle at build time; the build fails loudly
# if it is missing or malformed (src/lib/config/env.ts validates at boot).
#   docker build --build-arg VITE_API_URL=https://api.example.com/api/v1 -t teka-web apps/web
# Digest-pinned for reproducible builds; Dependabot (docker ecosystem) bumps it.
FROM node:22-alpine@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32 AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

ARG VITE_API_URL
# env.ts only validates in the browser at boot; without this guard a missing
# build-arg produces a green build that white-screens at runtime.
RUN test -n "${VITE_API_URL}" || { echo "VITE_API_URL build-arg is required" >&2; exit 1; }
ENV VITE_API_URL=${VITE_API_URL}
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.29-alpine@sha256:0c79d56aee561a1d81c63f00eee5fb5fe29279560cdc55e91425133104c7fbe6

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 8080
