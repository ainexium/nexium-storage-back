package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"nexium.ai/api/internal/mailer"
	"nexium.ai/api/pkg/apierr"
	"nexium.ai/api/pkg/config"
)

const codeTTL = 15 * time.Minute

type Service interface {
	Register(ctx context.Context, req *RegisterRequest) (*VerificationSentResponse, error)
	Login(ctx context.Context, req *LoginRequest) (*TokenPair, error)
	Refresh(ctx context.Context, req *RefreshRequest) (*TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	Me(ctx context.Context, userID uuid.UUID) (*User, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateProfileRequest) (*User, error)
	RequestEmailChange(ctx context.Context, userID uuid.UUID, newEmail string) error
	ConfirmEmailChange(ctx context.Context, userID uuid.UUID, code string) (*User, error)
	CancelEmailChange(ctx context.Context, userID uuid.UUID) error
	VerifyEmail(ctx context.Context, req *VerifyEmailRequest) (*TokenPair, error)
	ResendVerification(ctx context.Context, email string) error
	ForgotPassword(ctx context.Context, email string) error
	VerifyResetCode(ctx context.Context, email, code string) error
	ResetPassword(ctx context.Context, req *ResetPasswordRequest) error
}

type service struct {
	store  Store
	cfg    *config.Config
	mailer *mailer.Mailer
}

func NewService(store Store, cfg *config.Config, m *mailer.Mailer) Service {
	return &service{store: store, cfg: cfg, mailer: m}
}

func (s *service) Register(ctx context.Context, req *RegisterRequest) (*VerificationSentResponse, error) {
	if !req.TermsAccepted {
		return nil, apierr.ErrBadRequest("you must accept the terms of service")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, apierr.ErrBadRequest("name is required")
	}
	if !strings.Contains(req.Email, "@") {
		return nil, apierr.ErrBadRequest("invalid email")
	}
	if len(req.Password) < 8 {
		return nil, apierr.ErrBadRequest("password must be at least 8 characters")
	}

	existing, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, apierr.ErrConflict("email already registered")
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	user := &User{
		ID:           uuid.New(),
		Name:         strings.TrimSpace(req.Name),
		Email:        strings.ToLower(strings.TrimSpace(req.Email)),
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	if err := s.sendCode(ctx, user, "verify_email", "Verify your NEXIUM Storage account", verifyEmailHTML); err != nil {
		log.Printf("[auth] send verification failed for %s: %v", user.Email, err)
	}

	return &VerificationSentResponse{Email: user.Email, Message: "verification code sent"}, nil
}

func (s *service) Login(ctx context.Context, req *LoginRequest) (*TokenPair, error) {
	user, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		return nil, err
	}
	if user == nil || !verifyPassword(req.Password, user.PasswordHash) {
		return nil, apierr.ErrBadRequest("invalid email or password")
	}
	if !user.IsVerified {
		return nil, apierr.New(http.StatusForbidden, "email_not_verified")
	}
	return s.issueTokenPair(ctx, user)
}

func (s *service) Refresh(ctx context.Context, req *RefreshRequest) (*TokenPair, error) {
	hash := hashToken(req.RefreshToken)
	userID, err := s.store.GetRefreshToken(ctx, hash)
	if err != nil {
		return nil, apierr.ErrUnauthorized
	}
	if err := s.store.DeleteRefreshToken(ctx, hash); err != nil {
		return nil, err
	}
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, apierr.ErrUnauthorized
	}
	return s.issueTokenPair(ctx, user)
}

func (s *service) Logout(ctx context.Context, refreshToken string) error {
	return s.store.DeleteRefreshToken(ctx, hashToken(refreshToken))
}

func (s *service) Me(ctx context.Context, userID uuid.UUID) (*User, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apierr.ErrNotFound
	}
	s.applyEffectiveQuota(user)
	return user, nil
}

// applyEffectiveQuota calcule le quota réel de l'user en bytes.
// NULL en DB → défaut plateforme depuis la config.
// 0 → illimité (retourne 0).
func (s *service) applyEffectiveQuota(u *User) {
	if u.storageQuotaBytes == nil {
		u.QuotaBytes = s.cfg.StorageQuotaGB * 1024 * 1024 * 1024
	} else {
		u.QuotaBytes = *u.storageQuotaBytes
	}
}

func (s *service) UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateProfileRequest) (*User, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apierr.ErrNotFound
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = user.Name
	}

	passwordHash := user.PasswordHash
	if req.NewPassword != "" {
		if req.CurrentPassword == "" {
			return nil, apierr.ErrBadRequest("current_password is required to set a new password")
		}
		if !verifyPassword(req.CurrentPassword, user.PasswordHash) {
			return nil, apierr.ErrBadRequest("current password is incorrect")
		}
		if len(req.NewPassword) < 8 {
			return nil, apierr.ErrBadRequest("new password must be at least 8 characters")
		}
		passwordHash, err = hashPassword(req.NewPassword)
		if err != nil {
			return nil, err
		}
	}

	if err := s.store.UpdateUser(ctx, userID, name, user.Email, passwordHash); err != nil {
		return nil, err
	}
	user.Name = name
	return user, nil
}

func (s *service) RequestEmailChange(ctx context.Context, userID uuid.UUID, newEmail string) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return apierr.ErrNotFound
	}

	newEmail = strings.ToLower(strings.TrimSpace(newEmail))
	if !strings.Contains(newEmail, "@") {
		return apierr.ErrBadRequest("invalid email")
	}
	if newEmail == user.Email {
		return apierr.ErrBadRequest("this is already your current email")
	}

	existing, err := s.store.GetUserByEmail(ctx, newEmail)
	if err != nil {
		return err
	}
	if existing != nil {
		return apierr.ErrConflict("email already in use")
	}

	if err := s.store.SetPendingEmail(ctx, userID, newEmail); err != nil {
		return err
	}

	return s.sendCode(ctx,
		&User{ID: userID, Email: newEmail, Name: user.Name},
		"change_email",
		"Confirm your new email — NEXIUM Storage",
		changeEmailHTML,
	)
}

func (s *service) ConfirmEmailChange(ctx context.Context, userID uuid.UUID, code string) (*User, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apierr.ErrNotFound
	}
	if user.PendingEmail == "" {
		return nil, apierr.ErrBadRequest("no pending email change")
	}

	codeID, codeHash, err := s.store.GetLatestCode(ctx, userID, "change_email")
	if err != nil {
		return nil, err
	}
	if codeID == uuid.Nil || hashToken(code) != codeHash {
		return nil, apierr.ErrBadRequest("invalid or expired code")
	}

	if err := s.store.MarkCodeUsed(ctx, codeID); err != nil {
		return nil, err
	}
	if err := s.store.UpdateUser(ctx, userID, user.Name, user.PendingEmail, user.PasswordHash); err != nil {
		return nil, err
	}
	if err := s.store.ClearPendingEmail(ctx, userID); err != nil {
		return nil, err
	}

	user.Email = user.PendingEmail
	user.PendingEmail = ""
	s.applyEffectiveQuota(user)
	return user, nil
}

func (s *service) CancelEmailChange(ctx context.Context, userID uuid.UUID) error {
	return s.store.ClearPendingEmail(ctx, userID)
}

func (s *service) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) (*TokenPair, error) {
	user, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apierr.ErrBadRequest("invalid code")
	}
	if user.IsVerified {
		return s.issueTokenPair(ctx, user)
	}

	codeID, codeHash, err := s.store.GetLatestCode(ctx, user.ID, "verify_email")
	if err != nil {
		return nil, err
	}
	if codeID == uuid.Nil || hashToken(req.Code) != codeHash {
		return nil, apierr.ErrBadRequest("invalid or expired code")
	}

	if err := s.store.MarkCodeUsed(ctx, codeID); err != nil {
		return nil, err
	}
	if err := s.store.SetVerified(ctx, user.ID); err != nil {
		return nil, err
	}
	user.IsVerified = true
	return s.issueTokenPair(ctx, user)
}

func (s *service) ResendVerification(ctx context.Context, email string) error {
	user, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	if user == nil || user.IsVerified {
		return nil // don't reveal if email exists or already verified
	}
	return s.sendCode(ctx, user, "verify_email", "Verify your NEXIUM Storage account", verifyEmailHTML)
}

func (s *service) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}
	return s.sendCode(ctx, user, "reset_password", "Reset your NEXIUM Storage password", resetPasswordHTML)
}

func (s *service) VerifyResetCode(ctx context.Context, email, code string) error {
	user, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	if user == nil {
		return apierr.ErrBadRequest("invalid or expired code")
	}
	codeID, codeHash, err := s.store.GetLatestCode(ctx, user.ID, "reset_password")
	if err != nil {
		return err
	}
	if codeID == uuid.Nil || hashToken(code) != codeHash {
		return apierr.ErrBadRequest("invalid or expired code")
	}
	return nil
}

func (s *service) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
	if len(req.Password) < 8 {
		return apierr.ErrBadRequest("password must be at least 8 characters")
	}
	user, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		return err
	}
	if user == nil {
		return apierr.ErrBadRequest("invalid code")
	}

	codeID, codeHash, err := s.store.GetLatestCode(ctx, user.ID, "reset_password")
	if err != nil {
		return err
	}
	if codeID == uuid.Nil || hashToken(req.Code) != codeHash {
		return apierr.ErrBadRequest("invalid or expired code")
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	if err := s.store.MarkCodeUsed(ctx, codeID); err != nil {
		return err
	}
	if err := s.store.UpdatePassword(ctx, user.ID, hash); err != nil {
		return err
	}
	return s.store.DeleteUserTokens(ctx, user.ID)
}

// ─── internal helpers ─────────────────────────────────────────────────────────

func (s *service) sendCode(ctx context.Context, user *User, purpose, subject string, htmlFn func(string) string) error {
	code := generateCode()
	expiresAt := time.Now().UTC().Add(codeTTL)
	if err := s.store.CreateVerificationCode(ctx, user.ID, hashToken(code), purpose, expiresAt); err != nil {
		return err
	}
	return s.mailer.Send(ctx, user.Email, user.Name, subject, htmlFn(code))
}

func (s *service) issueTokenPair(ctx context.Context, user *User) (*TokenPair, error) {
	now := time.Now()
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID.String(),
		"iat": now.Unix(),
		"exp": now.Add(s.cfg.JWTAccessTTL).Unix(),
	}).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}

	rawRefresh, err := generateToken()
	if err != nil {
		return nil, err
	}

	expiresAt := now.Add(s.cfg.JWTRefreshTTL)
	if err := s.store.SaveRefreshToken(ctx, user.ID, hashToken(rawRefresh), expiresAt); err != nil {
		return nil, err
	}

	s.applyEffectiveQuota(user)
	return &TokenPair{AccessToken: accessToken, RefreshToken: rawRefresh, User: user}, nil
}

func generateCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=4$%s$%s",
		hex.EncodeToString(salt),
		hex.EncodeToString(hash),
	), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	salt, err := hex.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(parts[5])
	if err != nil {
		return false
	}
	computed := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	if len(computed) != len(expected) {
		return false
	}
	var diff byte
	for i := range computed {
		diff |= computed[i] ^ expected[i]
	}
	return diff == 0
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ─── Email templates ──────────────────────────────────────────────────────────

func verifyEmailHTML(code string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><body style="margin:0;font-family:sans-serif;background:#0a0a0f;color:#e5e5e5">
<div style="max-width:480px;margin:40px auto;padding:40px 32px;background:#111118;border-radius:16px;border:1px solid #222">
  <h2 style="color:#007BFF;margin:0 0 4px">NEXIUM Storage</h2>
  <h3 style="margin:0 0 24px;color:#fff">Verify your email address</h3>
  <p style="color:#999;margin:0 0 20px">Enter this code in the app to confirm your account:</p>
  <div style="background:#0a0a0f;border:1px solid #333;border-radius:12px;padding:28px;text-align:center;font-size:42px;font-weight:700;letter-spacing:14px;font-family:monospace;color:#007BFF">%s</div>
  <p style="color:#666;font-size:12px;margin:24px 0 0">Expires in 15 minutes. If you did not sign up for NEXIUM Storage, ignore this email.</p>
</div></body></html>`, code)
}

func changeEmailHTML(code string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><body style="margin:0;font-family:sans-serif;background:#0a0a0f;color:#e5e5e5">
<div style="max-width:480px;margin:40px auto;padding:40px 32px;background:#111118;border-radius:16px;border:1px solid #222">
  <h2 style="color:#007BFF;margin:0 0 4px">NEXIUM Storage</h2>
  <h3 style="margin:0 0 24px;color:#fff">Confirm your new email address</h3>
  <p style="color:#999;margin:0 0 20px">Enter this code to confirm your new email address:</p>
  <div style="background:#0a0a0f;border:1px solid #333;border-radius:12px;padding:28px;text-align:center;font-size:42px;font-weight:700;letter-spacing:14px;font-family:monospace;color:#007BFF">%s</div>
  <p style="color:#666;font-size:12px;margin:24px 0 0">Expires in 15 minutes. If you did not request this change, ignore this email.</p>
</div></body></html>`, code)
}

func resetPasswordHTML(code string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><body style="margin:0;font-family:sans-serif;background:#0a0a0f;color:#e5e5e5">
<div style="max-width:480px;margin:40px auto;padding:40px 32px;background:#111118;border-radius:16px;border:1px solid #222">
  <h2 style="color:#007BFF;margin:0 0 4px">NEXIUM Storage</h2>
  <h3 style="margin:0 0 24px;color:#fff">Reset your password</h3>
  <p style="color:#999;margin:0 0 20px">Use this code to reset your password:</p>
  <div style="background:#0a0a0f;border:1px solid #333;border-radius:12px;padding:28px;text-align:center;font-size:42px;font-weight:700;letter-spacing:14px;font-family:monospace;color:#007BFF">%s</div>
  <p style="color:#666;font-size:12px;margin:24px 0 0">Expires in 15 minutes. If you did not request a password reset, ignore this email.</p>
</div></body></html>`, code)
}
