-- Auth infrastructure, deliberately outside the baseline schema: refresh
-- tokens are an implementation detail of the chosen auth mechanism, not
-- product domain data. A future switch to opaque sessions or OTP-only login
-- deletes this table without touching the domain schema.
CREATE TABLE refresh_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES user_accounts (id) ON DELETE CASCADE,
    -- sha256 hex of the opaque token; the plaintext is never stored.
    token_hash text NOT NULL,
    -- Tokens issued through rotation share a family; reuse of a rotated
    -- token revokes the whole family.
    family_id  uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX refresh_tokens_token_hash_unique ON refresh_tokens (token_hash);
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);
CREATE INDEX refresh_tokens_family_id_idx ON refresh_tokens (family_id);
