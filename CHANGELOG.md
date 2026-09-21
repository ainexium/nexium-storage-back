# Changelog — NEXIUM Storage API

All notable changes to the backend are documented here.
Format: `[version] — date — description`

---

## [0.6.1] — 2026-09 — Pool de connexions DB

### Changed
- `pkg/database/postgres.go` — pool de connexions explicitement limité à 50 (`MaxConns = 50`). Auparavant pgxpool utilisait son défaut (~16), ce qui causait une file d'attente dès ~20 requêtes simultanées.
- `.env.example` — ajout de l'URL de connexion Supabase pooler (port 6543, `?pgbouncer=true`) en commentaire pour la production.
- `README.md` — documentation de la `DATABASE_URL` locale vs production (Supabase pooler).

---

## [0.6.0] — 2026-09 — Emails transactionnels & rappels d'expiration

### Added
- Email de confirmation de paiement avec reçu PDF en pièce jointe (pure Go, sans dépendance externe)
- Rappels d'expiration d'abonnement par email à J-10, J-5 et J-2 avant la fin de période
- Colonnes `reminded_10d_at`, `reminded_5d_at`, `reminded_2d_at` sur `subscriptions` (migration 018)
- `SendWithAttachment` sur le mailer Mailjet (support pièces jointes base64)
- `GetUserInfoFn` injecté dans `billing.Service` pour récupérer nom/email au moment de l'envoi

### Changed
- `billing.NewService` accepte deux nouveaux paramètres : `GetUserInfoFn` et `*mailer.Mailer`
- `validChannel` (liste hardcodée) remplacé par `IsActiveChannel` — requête dynamique sur `payment_channels.is_active`

### Fixed
- 6 mismatches de colonnes dans `billing/store.go` (scan errors silencieux sur `addons_enabled`) corrigés

---

## [0.5.0] — 2026-09 — Gestion des canaux de paiement

### Added
- Table `payment_channels` avec slug, logo_url, is_active, maintenance_note, display_order (migration 017)
- Colonne `addons_enabled` sur `plans` — add-ons réservés aux plans Pro et Business
- Canaux initiaux seedés : MTN MoMo, Orange Money, Wave, Moov Money (Côte d'Ivoire)
- Routes admin pour créer, modifier et désactiver les canaux de paiement
- Upload de logo opérateur vers R2 depuis l'interface admin
- Endpoint public `GET /api/v1/billing/channels` (affiché avant login)

---

## [0.4.0] — 2026-08 — Add-ons de stockage

### Added
- Table `storage_addons` — achat de stockage supplémentaire à la carte (migration 012)
- Packages disponibles : 50 GB, 100 GB, 200 GB, 500 GB
- Limite de taille de fichier par plan (`max_file_bytes`) — migration 011
- Add-ons cumulables : quota = plan de base + somme des add-ons complétés
- Réactivation automatique des add-ons au renouvellement d'abonnement
- `redirect_url` sur `billing_payments` pour les canaux à redirection (Wave) — migration 016

### Changed
- Plan Pro : 300 GB (au lieu de 250 GB) — migration 012
- Prix Starter ajustés successivement via migrations 013, 014, 015

---

## [0.3.0] — 2026-08 — Système de facturation

### Added
- Tables `plans`, `subscriptions`, `billing_payments` (migration 010)
- Plans : Free (10 GB / 0 FCFA), Starter (50 GB / 2 500 FCFA), Pro (300 GB / 6 000 FCFA), Business (1 TB / 15 000 FCFA)
- Intégration Adullam (Mobile Money) : MTN MoMo, Orange Money, Wave, Moov
- Checkout initié via API — push USSD ou redirection Wave selon le canal
- Webhook Adullam (HMAC SHA-256) pour confirmation de paiement
- Polling fallback `GET /api/v1/billing/payments/:id` si webhook non reçu
- Activation automatique de l'abonnement à la confirmation du paiement
- Job quotidien d'expiration des abonnements (`expiry.Job`) — remet le quota au niveau Free
- Expiration en temps réel dans `GetSubscription` si `current_period_end` dépassé
- Quota de stockage par utilisateur (`storage_quota_bytes` sur `users`) — migration 004
- `platform_settings` pour verrouiller le stockage plateforme (`storage_locked`)

---

## [0.2.0] — 2026-08 — Webhooks & panel admin

### Added
- Table `webhooks` et `webhook_deliveries` (migration 008)
- Livraison asynchrone des événements : `file.created`, `file.deleted`, `file.renamed`
- Signature HMAC-SHA256 sur les webhooks sortants (`X-Nexium-Signature`)
- Panel admin : gestion des utilisateurs, quota, promotion admin/super-admin
- Logs d'activité par utilisateur (migration tracée dans `logs`)
- Middleware `RequireAdmin` et `RequireSuperAdmin`
- Job d'inactivité : avertissement email à 90 jours, suppression des fichiers à 120 jours (migration 009)
- Indexes de performance (migration 007)

---

## [0.1.0] — 2026-08 — Plateforme initiale

### Added
- Auth : inscription, connexion, refresh token, déconnexion, vérification email, mot de passe oublié (migration 006)
- JWT (access 15 min / refresh 7 jours), Argon2id pour les mots de passe
- Projects : CRUD, suppression en cascade des buckets et fichiers associés
- Buckets : CRUD, visibilité public/privé (migration 005)
- Files : upload multipart, téléchargement, suppression, renommage, presign (upload direct R2)
- Fichiers publics accessibles sans auth via `/api/v1/public/files/:id`
- API Keys : création, liste, révocation — authentification externe via `nxm_` prefix
- Rate limiting : 20 req/min sur les routes auth, 600 req/min par projet sur l'API externe
- Usage : quota utilisé, quota disponible
- Cloudflare R2 : upload, suppression, presigned URLs
- Migrations SQL auto-appliquées au démarrage via `//go:embed`
- Super admin initial via `cmd/seed`

---

## État actuel — v0.6.0

### Fonctionnel
- Auth complète (JWT, email verification, reset password)
- Projects / Buckets / Files (upload, download, delete, rename, presign)
- API Keys (auth externe)
- Webhooks sortants
- Billing complet (plans, abonnements, add-ons, Adullam Mobile Money)
- Emails transactionnels (confirmation paiement + reçu PDF, rappels expiration)
- Job d'expiration (abonnements + inactivité utilisateur)
- Admin panel complet
- Rate limiting

### Connu / À faire
- `ADULLAM_WEBHOOK_SECRET` vide en local → vérification HMAC désactivée, à configurer en prod
- `cmd/seed` : email et mot de passe admin à passer en variables d'env (actuellement hardcodés)
- CI/CD non configuré — déploiement manuel (`git pull` + `docker compose up`)
- Bug Wave signalé à Adullam : 60 FCFA prélevés au lieu du montant réel (problème settlement)
- Renouvellement automatique non implémenté (Mobile Money ne supporte pas les prélèvements récurrents)
