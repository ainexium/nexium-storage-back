-- 024: scope API keys to specific buckets (NULL = all buckets in project)
ALTER TABLE api_keys
  ADD COLUMN IF NOT EXISTS allowed_bucket_ids text[] DEFAULT NULL;
