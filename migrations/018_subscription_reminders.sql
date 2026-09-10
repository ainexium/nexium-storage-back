-- 018: colonnes de suivi des rappels d'expiration d'abonnement
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS reminded_10d_at TIMESTAMPTZ;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS reminded_5d_at  TIMESTAMPTZ;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS reminded_2d_at  TIMESTAMPTZ;
