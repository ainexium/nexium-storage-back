-- 026: channels expansion — slug normalisation, new countries, card + international

-- ── 1. Normalise old CI slugs (camelCase → snake_case) ──────────────────────
UPDATE payment_channels SET slug='mtn_ci',    updated_at=now() WHERE slug='mtnCI';
UPDATE payment_channels SET slug='orange_ci', updated_at=now() WHERE slug='orangeCI';
UPDATE payment_channels SET slug='wave_ci',   updated_at=now() WHERE slug='waveCI';
UPDATE payment_channels SET slug='moov_ci',   updated_at=now() WHERE slug='moovCI';

-- Make sure CI channels have correct country_code + provider
UPDATE payment_channels SET country_code='CI', provider='adullam', updated_at=now()
WHERE slug IN ('mtn_ci','orange_ci','wave_ci','moov_ci');

-- Propagate rename to transaction tables
UPDATE billing_payments SET channel='mtn_ci'    WHERE channel='mtnCI';
UPDATE billing_payments SET channel='orange_ci' WHERE channel='orangeCI';
UPDATE billing_payments SET channel='wave_ci'   WHERE channel='waveCI';
UPDATE billing_payments SET channel='moov_ci'   WHERE channel='moovCI';
UPDATE storage_addons   SET channel='mtn_ci'    WHERE channel='mtnCI';
UPDATE storage_addons   SET channel='orange_ci' WHERE channel='orangeCI';
UPDATE storage_addons   SET channel='wave_ci'   WHERE channel='waveCI';
UPDATE storage_addons   SET channel='moov_ci'   WHERE channel='moovCI';

-- ── 2. Fix Togo: T-Money → MTN MoMo (SasPay) ────────────────────────────────
UPDATE payment_channels SET slug='mtn_tg', name='MTN MoMo', updated_at=now() WHERE slug='tmoney_tg';
UPDATE billing_payments SET channel='mtn_tg' WHERE channel='tmoney_tg';
UPDATE storage_addons   SET channel='mtn_tg' WHERE channel='tmoney_tg';

-- ── 3. Disable Free Money SN (not in SasPay coverage) ───────────────────────
UPDATE payment_channels SET is_active=false, updated_at=now() WHERE slug='free_sn';

-- ── 4. Add missing channels for existing countries ───────────────────────────
INSERT INTO payment_channels (name, slug, is_active, display_order, country_code, provider) VALUES
  ('Celtiis Cash', 'celtiis_bj', true,  3, 'BJ', 'saspay'),
  ('YAS',          'yas_sn',     true,  4, 'SN', 'saspay'),
  ('Moov Money',   'moov_ga',    true,  2, 'GA', 'saspay')
ON CONFLICT (slug) DO NOTHING;

-- ── 5. New countries ─────────────────────────────────────────────────────────
INSERT INTO billing_countries (code, name, currency_code, currency_symbol, local_per_xof, flag_emoji, is_active) VALUES
  ('GH', 'Ghana',             'GHS', 'GH₵', 0.0230, '🇬🇭', false),
  ('KE', 'Kenya',             'KES', 'Ksh',  0.1950, '🇰🇪', false),
  ('NG', 'Nigeria',           'NGN', '₦',    2.4400, '🇳🇬', false),
  ('TZ', 'Tanzanie',          'TZS', 'TSh',  4.0000, '🇹🇿', false),
  ('UG', 'Ouganda',           'UGX', 'USh',  5.7000, '🇺🇬', false),
  ('RW', 'Rwanda',            'RWF', 'Fr',   2.1000, '🇷🇼', false),
  ('CD', 'RD Congo',          'CDF', 'FC',   4.3000, '🇨🇩', false),
  ('MZ', 'Mozambique',        'MZN', 'MT',   0.0980, '🇲🇿', false),
  ('MW', 'Malawi',            'MWK', 'MK',   2.6000, '🇲🇼', false),
  ('LS', 'Lesotho',           'LSL', 'M',    0.0280, '🇱🇸', false),
  ('SL', 'Sierra Leone',      'SLE', 'Le',   0.0340, '🇸🇱', false),
  ('ET', 'Éthiopie',          'ETB', 'Br',   0.1900, '🇪🇹', false),
  ('INT','International',     'USD', '$',    0.0015, '🌍',  false)
ON CONFLICT (code) DO NOTHING;

-- ── 6. Channels for new countries ────────────────────────────────────────────
INSERT INTO payment_channels (name, slug, is_active, display_order, country_code, provider) VALUES
  -- Ghana
  ('MTN MoMo',       'mtn_gh',        true,  1, 'GH', 'saspay'),
  ('AirtelTigo Money','airteltigo_gh', true,  2, 'GH', 'saspay'),
  ('Telecel Cash',   'telecel_gh',    true,  3, 'GH', 'saspay'),
  -- Kenya
  ('M-Pesa',         'mpesa_ke',      true,  1, 'KE', 'saspay'),
  -- Nigeria
  ('MTN MoMo',       'mtn_ng',        true,  1, 'NG', 'saspay'),
  ('Airtel Money',   'airtel_ng',     true,  2, 'NG', 'saspay'),
  -- Tanzanie
  ('Vodacom M-Pesa', 'vodacom_tz',    true,  1, 'TZ', 'saspay'),
  ('YAS',            'yas_tz',        true,  2, 'TZ', 'saspay'),
  ('Airtel Money',   'airtel_tz',     true,  3, 'TZ', 'saspay'),
  ('Halotel',        'halotel_tz',    true,  4, 'TZ', 'saspay'),
  -- Ouganda
  ('MTN MoMo',       'mtn_ug',        true,  1, 'UG', 'saspay'),
  ('Airtel Money',   'airtel_ug',     true,  2, 'UG', 'saspay'),
  -- Rwanda
  ('MTN MoMo',       'mtn_rw',        true,  1, 'RW', 'saspay'),
  ('Airtel Money',   'airtel_rw',     true,  2, 'RW', 'saspay'),
  -- RD Congo
  ('Vodacom M-Pesa', 'vodacom_cd',    true,  1, 'CD', 'saspay'),
  ('Airtel Money',   'airtel_cd',     true,  2, 'CD', 'saspay'),
  ('Orange Money',   'orange_cd',     true,  3, 'CD', 'saspay'),
  -- Mozambique
  ('M-Pesa',         'mpesa_mz',      true,  1, 'MZ', 'saspay'),
  ('Movitel',        'movitel_mz',    true,  2, 'MZ', 'saspay'),
  -- Malawi
  ('Airtel Money',   'airtel_mw',     true,  1, 'MW', 'saspay'),
  ('TNM Mpamba',     'tnm_mw',        true,  2, 'MW', 'saspay'),
  -- Lesotho
  ('M-Pesa',         'mpesa_ls',      true,  1, 'LS', 'saspay'),
  -- Sierra Leone
  ('Orange Money',   'orange_sl',     true,  1, 'SL', 'saspay'),
  -- Éthiopie
  ('M-Pesa',         'mpesa_et',      true,  1, 'ET', 'saspay'),
  ('Telebirr',       'telebirr_et',   true,  2, 'ET', 'saspay')
ON CONFLICT (slug) DO NOTHING;

-- ── 7. Carte bancaire par pays (désactivée) ───────────────────────────────────
-- Ajoutée pour chaque pays actif + les nouveaux pays ; display_order=99 pour apparaître en dernier
INSERT INTO payment_channels (name, slug, is_active, display_order, country_code, provider) VALUES
  ('Carte bancaire', 'card_ci',  false, 99, 'CI',  'saspay'),
  ('Carte bancaire', 'card_bj',  false, 99, 'BJ',  'saspay'),
  ('Carte bancaire', 'card_sn',  false, 99, 'SN',  'saspay'),
  ('Carte bancaire', 'card_ml',  false, 99, 'ML',  'saspay'),
  ('Carte bancaire', 'card_bf',  false, 99, 'BF',  'saspay'),
  ('Carte bancaire', 'card_tg',  false, 99, 'TG',  'saspay'),
  ('Carte bancaire', 'card_ne',  false, 99, 'NE',  'saspay'),
  ('Carte bancaire', 'card_cm',  false, 99, 'CM',  'saspay'),
  ('Carte bancaire', 'card_gn',  false, 99, 'GN',  'saspay'),
  ('Carte bancaire', 'card_ga',  false, 99, 'GA',  'saspay'),
  ('Carte bancaire', 'card_cg',  false, 99, 'CG',  'saspay'),
  ('Carte bancaire', 'card_gh',  false, 99, 'GH',  'saspay'),
  ('Carte bancaire', 'card_ke',  false, 99, 'KE',  'saspay'),
  ('Carte bancaire', 'card_ng',  false, 99, 'NG',  'saspay'),
  ('Carte bancaire', 'card_tz',  false, 99, 'TZ',  'saspay'),
  ('Carte bancaire', 'card_ug',  false, 99, 'UG',  'saspay'),
  ('Carte bancaire', 'card_rw',  false, 99, 'RW',  'saspay'),
  ('Carte bancaire', 'card_cd',  false, 99, 'CD',  'saspay'),
  ('Carte bancaire', 'card_mz',  false, 99, 'MZ',  'saspay'),
  ('Carte bancaire', 'card_mw',  false, 99, 'MW',  'saspay'),
  ('Carte bancaire', 'card_ls',  false, 99, 'LS',  'saspay'),
  ('Carte bancaire', 'card_sl',  false, 99, 'SL',  'saspay'),
  ('Carte bancaire', 'card_et',  false, 99, 'ET',  'saspay')
ON CONFLICT (slug) DO NOTHING;

-- ── 8. Canaux internationaux (Visa / Mastercard / SWIFT) — désactivés ────────
-- country_code='INT' ; l'admin active/désactive indépendamment de tout pays
INSERT INTO payment_channels (name, slug, is_active, display_order, country_code, provider) VALUES
  ('Visa',           'visa_int',       false, 1, 'INT', 'saspay'),
  ('Mastercard',     'mastercard_int', false, 2, 'INT', 'saspay'),
  ('Virement SWIFT', 'swift_int',      false, 3, 'INT', 'saspay')
ON CONFLICT (slug) DO NOTHING;

-- ── 9. Nettoyage des faux chemins placeholder de la migration 025 ─────────────
-- Les vrais logos uploadés commencent par 'https://' ; les '/logos/...' sont des
-- placeholders qui n'existent pas — on les passe à NULL pour que la propagation
-- ci-dessous puisse les remplir avec les vrais logos CI déjà uploadés par l'admin.
UPDATE payment_channels SET logo_url = NULL, updated_at = now()
WHERE logo_url LIKE '/logos/%';

-- ── 10. Propagation de logos existants vers les canaux sans logo ───────────────
-- Si mtn_ci a déjà un vrai logo uploadé (URL R2), il est copié vers mtn_bj,
-- mtn_gh, mtn_ng, etc. Un seul upload couvre tous les pays du même réseau.
UPDATE payment_channels dst
SET    logo_url   = src.logo_url,
       updated_at = now()
FROM   payment_channels src
WHERE  dst.logo_url IS NULL
  AND  src.logo_url IS NOT NULL
  AND  LEFT(dst.slug, LENGTH(dst.slug)-3) = LEFT(src.slug, LENGTH(src.slug)-3)
  AND  dst.id != src.id;
