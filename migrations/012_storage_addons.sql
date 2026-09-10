-- 012: mise à jour plan Pro (300 GB) + add-ons de stockage

UPDATE plans SET storage_bytes = 322122547200 WHERE slug = 'pro'; -- 300 GB

CREATE TABLE IF NOT EXISTS storage_addons (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    package_id  TEXT        NOT NULL,  -- "addon-50gb", "addon-100gb", etc.
    bytes       BIGINT      NOT NULL,
    price_xof   INTEGER     NOT NULL,
    adullam_id  TEXT,
    status      TEXT        NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed, expired
    channel     TEXT        NOT NULL,
    phone       TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_storage_addons_user_id ON storage_addons(user_id);
CREATE INDEX IF NOT EXISTS idx_storage_addons_adullam ON storage_addons(adullam_id);
