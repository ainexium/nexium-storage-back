-- 017: payment_channels table + addons_enabled per plan

ALTER TABLE plans ADD COLUMN IF NOT EXISTS addons_enabled BOOLEAN NOT NULL DEFAULT false;
UPDATE plans SET addons_enabled = true WHERE slug IN ('pro', 'business');

CREATE TABLE IF NOT EXISTS payment_channels (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT        NOT NULL,
    slug             TEXT        NOT NULL UNIQUE,
    logo_url         TEXT,
    is_active        BOOLEAN     NOT NULL DEFAULT true,
    maintenance_note TEXT,
    display_order    INTEGER     NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO payment_channels (name, slug, logo_url, is_active, display_order) VALUES
    ('MTN MoMo',     'mtnCI',    '/logos/mtnCI.png',    true, 1),
    ('Orange Money', 'orangeCI', '/logos/orangeCI.png', true, 2),
    ('Wave',         'waveCI',   '/logos/waveCI.png',   true, 3),
    ('Moov Money',   'moovCI',   '/logos/moovCI.png',   true, 4)
ON CONFLICT (slug) DO NOTHING;
