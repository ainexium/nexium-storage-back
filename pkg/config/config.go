package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL          string
	JWTSecret            string
	JWTAccessTTL         time.Duration
	JWTRefreshTTL        time.Duration
	Port                 string
	AppURL               string // URL publique de l'API, ex: https://api.nexium.ai
	R2AccountID          string
	R2AccessKeyID        string
	R2SecretKey          string
	R2BucketName         string
	R2PublicURL          string // deprecated — ne plus utiliser pour les URLs de fichiers
	MaxFileSizeMB        int64
	StorageQuotaGB       int64
	MailjetAPIKey        string
	MailjetSecretKey     string
	MailFrom             string
	MailFromName         string
	AdullamAPIKey        string
	AdullamWebhookSecret string
	SuperAdminEmail      string
}

func Load() *Config {
	// Cherche .env dans le dossier courant, puis dans les parents (monorepo)
	for _, path := range []string{".env", "../.env", "../../.env"} {
		if err := godotenv.Load(path); err == nil {
			break
		}
	}

	accessTTL, err := time.ParseDuration(getEnv("JWT_ACCESS_TTL", "15m"))
	if err != nil {
		log.Fatal("invalid JWT_ACCESS_TTL:", err)
	}
	refreshTTL, err := time.ParseDuration(getEnv("JWT_REFRESH_TTL", "168h"))
	if err != nil {
		log.Fatal("invalid JWT_REFRESH_TTL:", err)
	}
	maxMB, _ := strconv.ParseInt(getEnv("MAX_FILE_SIZE_MB", "100"), 10, 64)
	quotaGB, _ := strconv.ParseInt(getEnv("STORAGE_QUOTA_GB", "10"), 10, 64)

	return &Config{
		DatabaseURL:      mustEnv("DATABASE_URL"),
		JWTSecret:        mustEnv("JWT_SECRET"),
		JWTAccessTTL:     accessTTL,
		JWTRefreshTTL:    refreshTTL,
		Port:             getEnv("PORT", "8080"),
		AppURL:           getEnv("APP_URL", "http://localhost:8080"),
		R2AccountID:      mustEnv("R2_ACCOUNT_ID"),
		R2AccessKeyID:    mustEnv("R2_ACCESS_KEY_ID"),
		R2SecretKey:      mustEnv("R2_SECRET_ACCESS_KEY"),
		R2BucketName:     mustEnv("R2_BUCKET_NAME"),
		R2PublicURL:      getEnv("R2_PUBLIC_URL", ""),
		MaxFileSizeMB:    maxMB,
		StorageQuotaGB:   quotaGB,
		MailjetAPIKey:        getEnv("MAILJET_API_KEY", ""),
		MailjetSecretKey:     getEnv("MAILJET_SECRET_KEY", ""),
		MailFrom:             getEnv("MAIL_FROM", ""),
		MailFromName:         getEnv("MAIL_FROM_NAME", "NEXIUM Storage"),
		AdullamAPIKey:        getEnv("ADULLAM_API_KEY", ""),
		AdullamWebhookSecret: getEnv("ADULLAM_WEBHOOK_SECRET", ""),
		SuperAdminEmail:      getEnv("SUPER_ADMIN_EMAIL", ""),
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
