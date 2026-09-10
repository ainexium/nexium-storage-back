-- 004: per-user storage quota (NULL = use platform default from STORAGE_QUOTA_GB)
ALTER TABLE users ADD COLUMN IF NOT EXISTS storage_quota_bytes BIGINT;
