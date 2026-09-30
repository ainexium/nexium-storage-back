-- 025: multi-pays, SasPay, multidevise

-- ── 1. Extend payment_channels ───────────────────────────────────────────────
ALTER TABLE payment_channels
  ADD COLUMN IF NOT EXISTS country_code TEXT NOT NULL DEFAULT 'CI',
  ADD COLUMN IF NOT EXISTS provider     TEXT NOT NULL DEFAULT 'adullam';

-- Set existing CI channels
UPDATE payment_channels SET country_code = 'CI', provider = 'adullam'
WHERE provider = 'adullam';

-- ── 2. Countries reference table ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS billing_countries (
  code             TEXT PRIMARY KEY,          -- ISO 3166-1 alpha-2
  name             TEXT NOT NULL,
  currency_code    TEXT NOT NULL,             -- XOF, XAF, GNF, ...
  currency_symbol  TEXT NOT NULL,             -- CFA, FCFA, GNF, ...
  local_per_xof    NUMERIC(14, 4) NOT NULL DEFAULT 1, -- local = xof * rate
  flag_emoji       TEXT NOT NULL DEFAULT '',
  is_active        BOOLEAN NOT NULL DEFAULT true
);

INSERT INTO billing_countries (code, name, currency_code, currency_symbol, local_per_xof, flag_emoji, is_active) VALUES
  ('CI', 'Côte d''Ivoire', 'XOF',  'CFA',  1,      '🇨🇮', true),
  ('BJ', 'Bénin',           'XOF',  'CFA',  1,      '🇧🇯', true),
  ('SN', 'Sénégal',         'XOF',  'CFA',  1,      '🇸🇳', true),
  ('ML', 'Mali',             'XOF',  'CFA',  1,      '🇲🇱', true),
  ('BF', 'Burkina Faso',    'XOF',  'CFA',  1,      '🇧🇫', true),
  ('TG', 'Togo',             'XOF',  'CFA',  1,      '🇹🇬', true),
  ('NE', 'Niger',            'XOF',  'CFA',  1,      '🇳🇪', true),
  ('CM', 'Cameroun',         'XAF',  'FCFA', 1,      '🇨🇲', true),
  ('GN', 'Guinée',           'GNF',  'GNF',  15.00,  '🇬🇳', false),
  ('GA', 'Gabon',            'XAF',  'FCFA', 1,      '🇬🇦', false),
  ('CG', 'Congo',            'XAF',  'FCFA', 1,      '🇨🇬', false)
ON CONFLICT (code) DO NOTHING;

-- ── 3. Extend billing_payments ───────────────────────────────────────────────
ALTER TABLE billing_payments
  ADD COLUMN IF NOT EXISTS provider     TEXT NOT NULL DEFAULT 'adullam',
  ADD COLUMN IF NOT EXISTS country_code TEXT NOT NULL DEFAULT 'CI';

-- ── 4. Extend storage_addons ─────────────────────────────────────────────────
ALTER TABLE storage_addons
  ADD COLUMN IF NOT EXISTS provider     TEXT NOT NULL DEFAULT 'adullam',
  ADD COLUMN IF NOT EXISTS country_code TEXT NOT NULL DEFAULT 'CI';

-- ── 5. SasPay channels ───────────────────────────────────────────────────────
INSERT INTO payment_channels (name, slug, logo_url, is_active, display_order, country_code, provider) VALUES
  -- Bénin
  ('MTN MoMo',    'mtn_bj',    '/logos/mtnBJ.png',    true,  1, 'BJ', 'saspay'),
  ('Moov Money',  'moov_bj',   '/logos/moovBJ.png',   true,  2, 'BJ', 'saspay'),
  -- Sénégal
  ('Wave',         'wave_sn',   '/logos/waveSN.png',   true,  1, 'SN', 'saspay'),
  ('Orange Money', 'orange_sn', '/logos/orangeSN.png', true,  2, 'SN', 'saspay'),
  ('Free Money',   'free_sn',   '/logos/freeSN.png',   false, 3, 'SN', 'saspay'),
  -- Cameroun
  ('MTN MoMo',    'mtn_cm',    '/logos/mtnCM.png',    true,  1, 'CM', 'saspay'),
  ('Orange Money', 'orange_cm', '/logos/orangeCM.png', true,  2, 'CM', 'saspay'),
  -- Mali
  ('Orange Money', 'orange_ml', '/logos/orangeML.png', true,  1, 'ML', 'saspay'),
  ('Moov Money',   'moov_ml',   '/logos/moovML.png',   true,  2, 'ML', 'saspay'),
  -- Burkina Faso
  ('Orange Money', 'orange_bf', '/logos/orangeBF.png', true,  1, 'BF', 'saspay'),
  ('Moov Money',   'moov_bf',   '/logos/moovBF.png',   true,  2, 'BF', 'saspay'),
  -- Togo
  ('Moov Money',   'moov_tg',   '/logos/moovTG.png',   true,  1, 'TG', 'saspay'),
  ('T-Money',      'tmoney_tg', '/logos/tmoneyTG.png', true,  2, 'TG', 'saspay'),
  -- Niger
  ('Airtel Money', 'airtel_ne', '/logos/airtelNE.png', true,  1, 'NE', 'saspay'),
  -- Guinée
  ('Orange Money', 'orange_gn', '/logos/orangeGN.png', true,  1, 'GN', 'saspay'),
  -- Gabon
  ('Airtel Money', 'airtel_ga', '/logos/airtelGA.png', true,  1, 'GA', 'saspay'),
  -- Congo
  ('MTN MoMo',    'mtn_cg',    '/logos/mtnCG.png',    true,  1, 'CG', 'saspay'),
  ('Airtel Money', 'airtel_cg', '/logos/airtelCG.png', true,  2, 'CG', 'saspay')
ON CONFLICT (slug) DO NOTHING;
