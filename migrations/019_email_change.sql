-- 019: pending email for verified email-change flow
ALTER TABLE users ADD COLUMN IF NOT EXISTS pending_email TEXT;
