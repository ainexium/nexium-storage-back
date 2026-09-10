-- 011: taille de fichier maximale par plan

ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_file_bytes BIGINT NOT NULL DEFAULT 104857600; -- 100 MB par défaut

UPDATE plans SET max_file_bytes = 104857600   WHERE slug = 'free';      -- 100 MB
UPDATE plans SET max_file_bytes = 1073741824  WHERE slug = 'starter';   -- 1 GB
UPDATE plans SET max_file_bytes = 5368709120  WHERE slug = 'pro';       -- 5 GB
UPDATE plans SET max_file_bytes = 21474836480 WHERE slug = 'business';  -- 20 GB
