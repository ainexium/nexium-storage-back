package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"nexium.ai/api/internal/admin"
	"nexium.ai/api/internal/apikeys"
	"nexium.ai/api/internal/auth"
	"nexium.ai/api/internal/billing"
	"nexium.ai/api/internal/buckets"
	"nexium.ai/api/internal/expiry"
	"nexium.ai/api/internal/files"
	"nexium.ai/api/internal/logs"
	"nexium.ai/api/internal/mailer"
	migrations "nexium.ai/api/migrations"
	"nexium.ai/api/internal/projects"
	"nexium.ai/api/internal/storage"
	"nexium.ai/api/internal/usage"
	"nexium.ai/api/internal/webhooks"
	"nexium.ai/api/pkg/adullam"
	"nexium.ai/api/pkg/config"
	"nexium.ai/api/pkg/database"
	mw "nexium.ai/api/pkg/middleware"
)

func main() {
	cfg := config.Load()
	db := database.Connect(cfg.DatabaseURL)
	defer db.Close()

	if err := database.RunMigrations(db, migrations.FS); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	if email := cfg.SuperAdminEmail; email != "" {
		if _, err := db.Exec(context.Background(),
			`UPDATE users SET is_admin=true, is_super_admin=true, is_verified=true WHERE email=$1`,
			email,
		); err != nil {
			log.Printf("[superadmin] promote failed for %s: %v", email, err)
		} else {
			log.Printf("[superadmin] promoted: %s", email)
		}
	}

	r2 := storage.NewR2Client(cfg)
	mail := mailer.New(cfg.MailjetAPIKey, cfg.MailjetSecretKey, cfg.MailFrom, cfg.MailFromName)

	logStore := logs.NewStore(db)

	authStore := auth.NewStore(db)
	projectStore := projects.NewStore(db)
	bucketStore := buckets.NewStore(db)
	fileStore := files.NewStore(db)
	apiKeyStore := apikeys.NewStore(db)
	usageStore := usage.NewStore(db)

	authSvc := auth.NewService(authStore, cfg, mail)
	projectSvc := projects.NewService(projectStore, r2, func(ctx context.Context, projectID, userID uuid.UUID) ([]uuid.UUID, error) {
		return bucketStore.ListIDsByProject(ctx, projectID, userID)
	})
	bucketSvc := buckets.NewService(bucketStore, r2)

	getUserQuota := files.GetUserQuota(func(ctx context.Context, userID uuid.UUID) (int64, error) {
		return authStore.GetStorageQuota(ctx, userID)
	})
	isPlatformLocked := files.IsPlatformLocked(func(ctx context.Context) (bool, error) {
		var val string
		err := db.QueryRow(ctx, `SELECT value FROM platform_settings WHERE key = 'storage_locked'`).Scan(&val)
		return val == "true", err
	})

	// Billing initialisé avant fileSvc pour pouvoir injecter GetUserFileSizeLimit
	adullamClient := adullam.New(cfg.AdullamAPIKey)
	billingStore := billing.NewStore(db)
	billingSvc := billing.NewService(billingStore, adullamClient,
		func(ctx context.Context, userID uuid.UUID, quotaBytes *int64) error {
			return authStore.SetStorageQuota(ctx, userID, quotaBytes)
		},
		func(ctx context.Context, userID uuid.UUID) (string, string, error) {
			u, err := authStore.GetUserByID(ctx, userID)
			if err != nil || u == nil {
				return "", "", err
			}
			return u.Name, u.Email, nil
		},
		mail,
	)
	billingHandler := billing.NewHandler(billingSvc, cfg.AdullamWebhookSecret, db)

	fileSvc := files.NewService(fileStore, r2, func(ctx context.Context, bucketID, userID uuid.UUID) (bool, bool, error) {
		b, err := bucketStore.GetByIDAndUser(ctx, bucketID, userID)
		if err != nil || b == nil {
			return false, false, err
		}
		return true, b.IsPublic, nil
	}, cfg.StorageQuotaGB*1024*1024*1024, getUserQuota,
		files.GetUserFileSizeLimit(func(ctx context.Context, userID uuid.UUID) (int64, error) {
			return billingSvc.GetUserFileSizeLimit(ctx, userID)
		}),
		isPlatformLocked, cfg.AppURL)
	apiKeySvc := apikeys.NewService(apiKeyStore)
	usageSvc := usage.NewService(usageStore)
	webhookStore := webhooks.NewStore(db)
	webhookSvc := webhooks.NewService(webhookStore)

	authHandler := auth.NewHandler(authSvc, logStore)
	projectHandler := projects.NewHandler(projectSvc, logStore)
	bucketHandler := buckets.NewHandler(bucketSvc, logStore)
	fileHandler := files.NewHandler(fileSvc, cfg.MaxFileSizeMB, logStore, webhookSvc)
	apiKeyHandler := apikeys.NewHandler(apiKeySvc)
	usageHandler := usage.NewHandler(usageSvc)
	webhookHandler := webhooks.NewHandler(webhookSvc)
	adminHandler := admin.NewHandler(db, logStore, cfg.StorageQuotaGB, r2)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "https://*.nexium.ai", "https://*.nexiumai.io", "https://nexiumai.io"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Endpoint public — pas d'auth, vérifie is_public avant de servir
	r.Mount("/api/v1/public/files", fileHandler.PublicRoutes())

	// Channels de paiement — public (affiché avant login)
	r.Mount("/api/v1/billing/channels", billingHandler.ChannelRoutes())

	// Auth routes with rate limiting (20 req/min per IP)
	r.Group(func(r chi.Router) {
		r.Use(mw.RateLimitPerIP(20, time.Minute))
		r.Mount("/api/v1/auth", authHandler.Routes(cfg.JWTSecret))
	})

	// JWT-authenticated routes with 30s timeout
	r.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(30 * time.Second))
		r.Use(mw.Authenticate(cfg.JWTSecret))
		r.Use(mw.TrackActivity(db))

		r.Mount("/api/v1/projects", projectHandler.Routes(func(r chi.Router) {
			r.Mount("/buckets", bucketHandler.ProjectRoutes())
			r.Mount("/api-keys", apiKeyHandler.ProjectRoutes())
			r.Mount("/usage", usageHandler.Routes())
			r.Mount("/webhooks", webhookHandler.Routes())
		}))

		r.Mount("/api/v1/buckets", bucketHandler.BucketRoutes())
		r.Mount("/api/v1/files", fileHandler.FileRoutes())
		r.Mount("/api/v1/api-keys", apiKeyHandler.APIKeyRoutes())
	})

	// Upload routes without timeout (large files)
	r.Group(func(r chi.Router) {
		r.Use(mw.Authenticate(cfg.JWTSecret))
		r.Use(mw.TrackActivity(db))
		r.Mount("/api/v1/buckets/{bucketID}/files", fileHandler.BucketRoutes())
	})

	// External API routes authenticated via API key — 600 req/min per project
	r.Group(func(r chi.Router) {
		r.Use(mw.AuthenticateAPIKey(db))
		r.Use(mw.RateLimitPerProject(600, time.Minute))
		r.Use(mw.TrackActivity(db))
		r.Mount("/api/v1/ext/buckets/{bucketID}/files", fileHandler.ExtBucketRoutes())
		r.Mount("/api/v1/ext/files", fileHandler.ExtFileRoutes())
	})

	// Billing — Adullam webhook (no JWT, HMAC-verified) — avant le groupe auth pour éviter le conflit de préfixe
	r.Mount("/api/v1/billing/webhook", billingHandler.WebhookRoutes())

	// Billing — auth routes (JWT required)
	r.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(30 * time.Second))
		r.Use(mw.Authenticate(cfg.JWTSecret))
		r.Use(mw.TrackActivity(db))
		r.Mount("/api/v1/billing", billingHandler.AuthRoutes())
	})

	// Admin routes (admin + super admin)
	r.Group(func(r chi.Router) {
		r.Use(chimw.Timeout(30 * time.Second))
		r.Use(mw.Authenticate(cfg.JWTSecret))
		r.Use(mw.RequireAdmin(db))
		r.Mount("/api/v1/admin", adminHandler.Routes(mw.RequireSuperAdmin(db)))
	})

	// Job d'expiration : avertissement à 90j, suppression à 120j
	expiryJob := expiry.New(db, r2, mail, "NEXIUM Storage")
	go expiryJob.Start(context.Background())

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("NEXIUM Storage API listening on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
