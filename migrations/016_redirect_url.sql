-- 016: add redirect_url to billing_payments and storage_addons (Wave payments)
ALTER TABLE billing_payments ADD COLUMN IF NOT EXISTS redirect_url TEXT;
ALTER TABLE storage_addons   ADD COLUMN IF NOT EXISTS redirect_url TEXT;
