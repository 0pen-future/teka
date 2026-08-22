# Homelab Traefik deployment

Status: completed · Branch: current

## Contract

- **Outcome:** Teka API is reachable at `https://teka-api.cauchuyenlaptrinh.com` and web at `https://teka-web.cauchuyenlaptrinh.com` through the existing Traefik and Cloudflare Tunnel path.
- **Constraints:** API and web publish no host ports; Traefik discovers only labeled containers on external `homelab`; both images listen internally on 8080; routers use entrypoint `web`; API retains a private network for database access; migrate stays private-only and gates API; API readiness is `/readyz`; web is built with `VITE_API_URL=https://teka-api.cauchuyenlaptrinh.com/api/v1`; CORS and statement links use `https://teka-web.cauchuyenlaptrinh.com`; credentials remain untracked.
- **Non-goals:** Provision DNS, Cloudflare Tunnel, Traefik, or PostgreSQL; execute the deployment; change application behavior; expose database/application ports; add bundled PostgreSQL.
- **Acceptance:**
  1. Merged Compose configuration has no published ports for API/web and resolves their internal port as 8080.
  2. API and web routers use the exact requested hosts, entrypoint `web`, external network `homelab`, and explicit target port 8080.
  3. API joins the private default network and `homelab`; migrate joins only the private default network; web joins only `homelab`.
  4. Traefik checks API readiness at `/readyz`; successful migration completion still gates API startup.
  5. Public configuration uses the accepted URLs; the production env example contains placeholders only for secrets, image references, and the database DSN.
  6. Deployment documentation covers build, validation, startup, verification, logs, updates, and rollback without claiming to provision external infrastructure.
  7. Compose validation, lint, relevant tests, and production image builds pass; development Compose behavior is unchanged.

## Phases

- [x] [Phase 1: Compose topology and safe environment contract](./phase-01-compose-topology.md)
- [x] [Phase 2: Operator documentation and deployment validation](./phase-02-operator-docs-and-validation.md)

## Touchpoints

- `docker-compose.prod.yml` — remove API/web host bindings and document internal ports with `expose`.
- `docker-compose.homelab.yml` — add only homelab environment overrides, Traefik labels, service networks, and the external network declaration.
- `.env.production.example` — provide a credential-free operator template.
- `.gitignore` — narrowly allow the production example while keeping real production env files ignored.
- `docs/deployment.md` — add the concrete homelab workflow while retaining platform-neutral guidance.

## Verification

- Render the merged Compose configuration using dummy, non-secret values from a temporary env file.
- Assert API/web have no published ports and migrate has no Traefik exposure.
- Run repository lint and relevant API/web tests.
- Build both production images, with the accepted API URL baked into the web image.
