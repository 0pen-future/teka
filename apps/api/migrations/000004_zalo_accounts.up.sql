-- One linked personal Zalo account per teacher. The Zalo session credentials
-- (IMEI + cookie jar) are full account-takeover material, so they exist here
-- only as a sealed blob: there is deliberately no plaintext column for them,
-- and nothing but the application's credential key can read encrypted_credentials.
--
-- teacher_id is the primary key rather than a surrogate id: one account per
-- teacher is a product constraint, and expressing it as the PK makes a second
-- row impossible instead of merely discouraged.
--
-- consent_version is NOT NULL because linking hands a third party's session to
-- this system; a linked row must always be backed by the exact consent text
-- the teacher acknowledged, with consent_at recording when.
CREATE TABLE zalo_accounts (
    teacher_id             UUID PRIMARY KEY REFERENCES teachers(id) ON DELETE CASCADE,
    encrypted_credentials  BYTEA        NOT NULL,
    zalo_uid               VARCHAR(50),
    display_name           VARCHAR(100),
    status                 VARCHAR(20)  NOT NULL DEFAULT 'linked'
                               CHECK (status IN ('linked', 'expired')),
    consent_version        VARCHAR(20)  NOT NULL,
    consent_at             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    linked_at              TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_verified_at       TIMESTAMPTZ,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at             TIMESTAMPTZ
);
