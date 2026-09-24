-- 022: grace period after subscription expiry
-- data_grace_end  = 30 days after expiry; files deleted when it passes
-- grace_reminded_*_at = dedup columns for grace-period warning emails
ALTER TABLE subscriptions
  ADD COLUMN IF NOT EXISTS data_grace_end         TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS grace_reminded_15d_at  TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS grace_reminded_7d_at   TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS grace_reminded_1d_at   TIMESTAMPTZ;
