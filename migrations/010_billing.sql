-- 010: billing — plans, subscriptions, billing_payments

CREATE TABLE IF NOT EXISTS plans (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT        NOT NULL UNIQUE,
    slug          TEXT        NOT NULL UNIQUE,
    storage_bytes BIGINT      NOT NULL,
    price_xof     INTEGER     NOT NULL DEFAULT 0,
    max_projects  INTEGER     NOT NULL DEFAULT -1,  -- -1 = unlimited
    is_active     BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id              UUID        NOT NULL REFERENCES plans(id),
    status               TEXT        NOT NULL DEFAULT 'active', -- active, expired, cancelled
    current_period_start TIMESTAMPTZ NOT NULL DEFAULT now(),
    current_period_end   TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id)
);

CREATE TABLE IF NOT EXISTS billing_payments (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id     UUID        NOT NULL REFERENCES plans(id),
    adullam_id  TEXT,
    amount_xof  INTEGER     NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed, expired
    channel     TEXT        NOT NULL,
    phone       TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id     ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_billing_payments_user_id  ON billing_payments(user_id);
CREATE INDEX IF NOT EXISTS idx_billing_payments_adullam  ON billing_payments(adullam_id);

-- Plans par défaut (compétitifs marché Afrique de l'Ouest)
INSERT INTO plans (name, slug, storage_bytes, price_xof, max_projects) VALUES
    ('Free',     'free',     10737418240,    0,     3),
    ('Starter',  'starter',  53687091200,    2500,  10),
    ('Pro',      'pro',      268435456000,   6000,  -1),
    ('Business', 'business', 1099511627776,  15000, -1)
ON CONFLICT (slug) DO NOTHING;
