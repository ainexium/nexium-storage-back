-- 008: webhooks

CREATE TABLE IF NOT EXISTS webhooks (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    url        TEXT        NOT NULL,
    secret     TEXT        NOT NULL,
    events     TEXT[]      NOT NULL DEFAULT '{}',
    is_active  BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_webhooks_project_id ON webhooks(project_id);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id  UUID        NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event       TEXT        NOT NULL,
    payload     JSONB       NOT NULL,
    status_code INT,
    success     BOOLEAN     NOT NULL DEFAULT false,
    attempts    INT         NOT NULL DEFAULT 0,
    error       TEXT,
    delivered_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_created_at ON webhook_deliveries(created_at DESC);
