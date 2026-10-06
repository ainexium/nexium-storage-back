-- Monthly API request quota per plan (NULL = unlimited)
ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_monthly_api_requests BIGINT;

UPDATE plans SET max_monthly_api_requests = 10000    WHERE slug = 'free';
UPDATE plans SET max_monthly_api_requests = 500000   WHERE slug = 'starter';
UPDATE plans SET max_monthly_api_requests = 5000000  WHERE slug = 'pro';
-- business: NULL = unlimited

-- Counter table: one row per (user, month)
CREATE TABLE IF NOT EXISTS api_request_counts (
    user_id UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    month   CHAR(7) NOT NULL,  -- 'YYYY-MM'
    count   BIGINT  NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, month)
);
