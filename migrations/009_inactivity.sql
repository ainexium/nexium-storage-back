-- 009: per-user activity tracking + platform lock

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS last_active_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS inactivity_warned_at  TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_users_last_active_at ON users(last_active_at);

-- Single-row settings table for super-admin toggles
CREATE TABLE IF NOT EXISTS platform_settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

INSERT INTO platform_settings (key, value)
  VALUES ('storage_locked', 'false')
  ON CONFLICT (key) DO NOTHING;
