-- 007: index improvements

-- verification_codes: composite index for GetLatestCode (user_id + purpose + filters)
DROP INDEX IF EXISTS idx_verification_codes_user_id;
CREATE INDEX IF NOT EXISTS idx_verification_codes_lookup
    ON verification_codes(user_id, purpose)
    WHERE used_at IS NULL;

-- files: composite index for ListByBucket ordered by created_at DESC
DROP INDEX IF EXISTS idx_files_bucket_id;
CREATE INDEX IF NOT EXISTS idx_files_bucket_created
    ON files(bucket_id, created_at DESC);

-- projects: enforce slug uniqueness per user
ALTER TABLE projects
    ADD CONSTRAINT uq_projects_user_slug UNIQUE (user_id, slug);

-- remove redundant indexes that duplicate UNIQUE constraints
DROP INDEX IF EXISTS idx_refresh_tokens_hash;
DROP INDEX IF EXISTS idx_api_keys_hash;
