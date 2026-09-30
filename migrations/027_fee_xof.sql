-- 027: add fee_xof to billing tables (network fees passed to customer)
ALTER TABLE billing_payments ADD COLUMN IF NOT EXISTS fee_xof INT NOT NULL DEFAULT 0;
ALTER TABLE storage_addons   ADD COLUMN IF NOT EXISTS fee_xof INT NOT NULL DEFAULT 0;
