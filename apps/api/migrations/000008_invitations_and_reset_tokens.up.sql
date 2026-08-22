-- =============================================================
-- 000008 — Invite-only onboarding: teacher accounts come to exist only via
-- owner-created invitations; teachers recover passwords through single-use
-- reset links delivered over the owner's linked Zalo.
--
-- Both tables follow the refresh_tokens (000002) template: an opaque token is
-- minted once, only its sha256-hex hash is stored, and the plaintext lives in
-- the returned link alone. Neither table touches centers/teachers columns —
-- centers.owner_id stays NOT NULL (is_owner is resolved as (c.owner_id = t.id)
-- in raw SQL; a nullable owner would force a NULL->bool scan rewrite).
-- =============================================================

-- Owner-created invitation to onboard a teacher into an existing center. The
-- token is single-use (status flips to 'accepted'); 'revoked' is the owner's
-- manual cancel. The 'expired' state is DERIVED (expires_at < now() while
-- still 'pending') — no cron, no status writer.
CREATE TABLE invitations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id   UUID NOT NULL REFERENCES centers (id),
    phone       VARCHAR(20) NOT NULL,            -- E.164, normalized
    token_hash  VARCHAR(64) NOT NULL UNIQUE,     -- sha256 hex
    status      VARCHAR(16) NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'accepted', 'revoked')),
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- At most one live (pending) invitation per (center, phone): a re-invite to
-- the same phone supersedes rather than piling up. A second pending invite for
-- a *different* center is allowed.
CREATE UNIQUE INDEX uq_invitations_pending_phone
    ON invitations (center_id, phone) WHERE status = 'pending';

-- Single-use password reset token for a teacher, delivered over the center
-- owner's Zalo. used_at = consumed; superseded_at = replaced by a newer
-- request (cooldown/supersede).
CREATE TABLE password_reset_tokens (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES user_accounts (id),
    token_hash    VARCHAR(64) NOT NULL UNIQUE,
    expires_at    TIMESTAMPTZ NOT NULL,
    used_at       TIMESTAMPTZ,
    superseded_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_password_reset_tokens_user ON password_reset_tokens (user_id);

-- At most one live token per account: enforces cooldown/supersede as a DB
-- invariant (not just service logic) and closes the concurrent-create race.
-- Phase 4's service must set used_at/superseded_at in the same tx it inserts a
-- replacement, or this index will reject the legitimate re-request.
CREATE UNIQUE INDEX uq_password_reset_active ON password_reset_tokens (user_id)
    WHERE used_at IS NULL AND superseded_at IS NULL;
