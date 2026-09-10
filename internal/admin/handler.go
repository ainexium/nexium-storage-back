package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
	"nexium.ai/api/internal/logs"
	"nexium.ai/api/internal/storage"
	"nexium.ai/api/pkg/apierr"
	mw "nexium.ai/api/pkg/middleware"
	"nexium.ai/api/pkg/response"
)

type Stats struct {
	UserCount     int   `json:"user_count"`
	ProjectCount  int   `json:"project_count"`
	BucketCount   int   `json:"bucket_count"`
	FileCount     int   `json:"file_count"`
	StorageBytes  int64 `json:"storage_bytes"`
	QuotaBytes    int64 `json:"quota_bytes"`
	StorageLocked bool  `json:"storage_locked"`
}

type AdminUser struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	Email             string     `json:"email"`
	IsAdmin           bool       `json:"is_admin"`
	IsSuperAdmin      bool       `json:"is_super_admin"`
	CreatedAt         time.Time  `json:"created_at"`
	LastActiveAt      time.Time  `json:"last_active_at"`
	ProjectCount      int        `json:"project_count"`
	FileCount         int        `json:"file_count"`
	StorageBytes      int64      `json:"storage_bytes"`
	StorageQuotaBytes *int64     `json:"storage_quota_bytes"`
}

type CreateAdminRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UpdateRoleRequest struct {
	IsAdmin bool `json:"is_admin"`
}

type Handler struct {
	db      *pgxpool.Pool
	log     *logs.Store
	quotaGB int64
	r2      *storage.R2Client
}

func NewHandler(db *pgxpool.Pool, log *logs.Store, quotaGB int64, r2 *storage.R2Client) *Handler {
	return &Handler{db: db, log: log, quotaGB: quotaGB, r2: r2}
}

func (h *Handler) Routes(superAdminMW func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()

	// All admins (admin + super admin)
	r.Get("/stats", h.stats)
	r.Get("/users", h.users)
	r.Get("/logs", h.listLogs)

	// Super admin only
	r.Group(func(r chi.Router) {
		r.Use(superAdminMW)
		r.Post("/users", h.createAdmin)
		r.Patch("/users/{id}/role", h.updateRole)
		r.Patch("/users/{id}/quota", h.setUserQuota)
		r.Post("/platform/lock", h.lockPlatform)
		r.Delete("/platform/lock", h.unlockPlatform)
		// Billing management
		r.Get("/billing/plans", h.listPlans)
		r.Patch("/billing/plans/{id}", h.updatePlan)
		r.Get("/billing/channels", h.listAllChannels)
		r.Post("/billing/channels", h.createChannel)
		r.Patch("/billing/channels/{id}", h.updateChannel)
		r.Delete("/billing/channels/{id}", h.deleteChannel)
		r.Post("/billing/channels/{id}/logo", h.uploadChannelLogo)
	})

	return r
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	s := &Stats{QuotaBytes: h.quotaGB * 1024 * 1024 * 1024}
	h.db.QueryRow(r.Context(), `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM projects),
			(SELECT COUNT(*) FROM buckets),
			(SELECT COUNT(*) FROM files),
			(SELECT COALESCE(SUM(size_bytes), 0) FROM files),
			(SELECT value = 'true' FROM platform_settings WHERE key = 'storage_locked')
	`).Scan(&s.UserCount, &s.ProjectCount, &s.BucketCount, &s.FileCount, &s.StorageBytes, &s.StorageLocked)
	response.OK(w, s)
}

func (h *Handler) users(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `
		SELECT u.id, u.name, u.email, u.is_admin, u.is_super_admin, u.created_at,
		       u.last_active_at, u.storage_quota_bytes,
		       COUNT(DISTINCT p.id) AS project_count,
		       COUNT(DISTINCT f.id) AS file_count,
		       COALESCE(SUM(f.size_bytes), 0) AS storage_bytes
		FROM users u
		LEFT JOIN projects p ON p.user_id = u.id
		LEFT JOIN buckets b ON b.project_id = p.id
		LEFT JOIN files f ON f.bucket_id = b.id
		GROUP BY u.id
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		response.Error(w, err)
		return
	}
	defer rows.Close()
	var list []AdminUser
	for rows.Next() {
		var u AdminUser
		rows.Scan(&u.ID, &u.Name, &u.Email, &u.IsAdmin, &u.IsSuperAdmin, &u.CreatedAt,
			&u.LastActiveAt, &u.StorageQuotaBytes,
			&u.ProjectCount, &u.FileCount, &u.StorageBytes)
		list = append(list, u)
	}
	if list == nil {
		list = []AdminUser{}
	}
	response.OK(w, list)
}

func (h *Handler) createAdmin(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)

	var req CreateAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Name == "" || !strings.Contains(req.Email, "@") {
		response.Error(w, apierr.ErrBadRequest("name and valid email are required"))
		return
	}
	if len(req.Password) < 8 {
		response.Error(w, apierr.ErrBadRequest("password must be at least 8 characters"))
		return
	}

	var exists bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, req.Email).Scan(&exists)
	if exists {
		response.Error(w, apierr.ErrConflict("email already registered"))
		return
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		response.Error(w, err)
		return
	}

	id := uuid.New()
	now := time.Now().UTC()
	_, err = h.db.Exec(r.Context(),
		`INSERT INTO users (id, name, email, password_hash, is_admin, is_super_admin, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, true, false, $5, $5)`,
		id, req.Name, req.Email, hash, now,
	)
	if err != nil {
		response.Error(w, err)
		return
	}

	h.log.Async(actorID, "admin.user.create", "user", id.String())
	response.Created(w, map[string]any{
		"id": id, "name": req.Name, "email": req.Email,
		"is_admin": true, "is_super_admin": false, "created_at": now,
	})
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)

	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid user id"))
		return
	}
	if targetID == actorID {
		response.Error(w, apierr.ErrBadRequest("cannot change your own role"))
		return
	}

	// Cannot touch another super admin
	var targetSuperAdmin bool
	h.db.QueryRow(r.Context(), `SELECT is_super_admin FROM users WHERE id=$1`, targetID).Scan(&targetSuperAdmin)
	if targetSuperAdmin {
		response.Error(w, apierr.ErrForbidden)
		return
	}

	var req UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE users SET is_admin=$2, updated_at=now() WHERE id=$1`,
		targetID, req.IsAdmin,
	)
	if err != nil {
		response.Error(w, err)
		return
	}

	action := "admin.role.grant"
	if !req.IsAdmin {
		action = "admin.role.revoke"
	}
	h.log.Async(actorID, action, "user", targetID.String())
	response.NoContent(w)
}

func (h *Handler) listLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := logs.Filter{
		Action: q.Get("action"),
		Limit:  200,
	}
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			filter.Limit = n
		}
	}
	if uid := q.Get("user_id"); uid != "" {
		if id, err := uuid.Parse(uid); err == nil {
			filter.UserID = &id
		}
	}

	logStore := logs.NewStore(h.db)
	entries, err := logStore.List(r.Context(), filter)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, entries)
}

// setUserQuota définit le quota de stockage d'un user en GB.
// Body: {"quota_gb": 50} — null pour revenir au défaut plateforme, 0 pour illimité.
func (h *Handler) setUserQuota(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)
	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid user id"))
		return
	}

	var req struct {
		QuotaGB *int64 `json:"quota_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}

	var quotaBytes *int64
	if req.QuotaGB != nil {
		gb := *req.QuotaGB * 1024 * 1024 * 1024
		quotaBytes = &gb
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE users SET storage_quota_bytes = $2, updated_at = now() WHERE id = $1`,
		targetID, quotaBytes,
	)
	if err != nil {
		response.Error(w, err)
		return
	}

	h.log.Async(actorID, "admin.user.quota", "user", targetID.String())
	response.NoContent(w)
}

// lockPlatform verrouille tous les uploads sur la plateforme (super admin uniquement).
func (h *Handler) lockPlatform(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)
	_, err := h.db.Exec(r.Context(),
		`INSERT INTO platform_settings (key, value) VALUES ('storage_locked', 'true')
		 ON CONFLICT (key) DO UPDATE SET value = 'true'`,
	)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(actorID, "admin.platform.lock", "platform", "storage")
	response.OK(w, map[string]string{"status": "locked"})
}

// unlockPlatform déverrouille les uploads.
func (h *Handler) unlockPlatform(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)
	_, err := h.db.Exec(r.Context(),
		`INSERT INTO platform_settings (key, value) VALUES ('storage_locked', 'false')
		 ON CONFLICT (key) DO UPDATE SET value = 'false'`,
	)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(actorID, "admin.platform.unlock", "platform", "storage")
	response.OK(w, map[string]string{"status": "unlocked"})
}

// ── Billing — Plans ──────────────────────────────────────────────────────────

func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT id, name, slug, storage_bytes, price_xof, max_projects, max_file_bytes, addons_enabled, is_active
		 FROM plans ORDER BY price_xof ASC`)
	if err != nil {
		response.Error(w, err)
		return
	}
	defer rows.Close()
	type Plan struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Slug          string `json:"slug"`
		StorageBytes  int64  `json:"storage_bytes"`
		PriceXOF      int    `json:"price_xof"`
		MaxProjects   int    `json:"max_projects"`
		MaxFileBytes  int64  `json:"max_file_bytes"`
		AddonsEnabled bool   `json:"addons_enabled"`
		IsActive      bool   `json:"is_active"`
	}
	var list []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.StorageBytes, &p.PriceXOF, &p.MaxProjects, &p.MaxFileBytes, &p.AddonsEnabled, &p.IsActive); err != nil {
			response.Error(w, err)
			return
		}
		list = append(list, p)
	}
	if list == nil {
		list = []Plan{}
	}
	response.OK(w, list)
}

func (h *Handler) updatePlan(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid plan id"))
		return
	}
	var req struct {
		PriceXOF      *int   `json:"price_xof"`
		StorageBytes  *int64 `json:"storage_bytes"`
		MaxFileBytes  *int64 `json:"max_file_bytes"`
		MaxProjects   *int   `json:"max_projects"`
		AddonsEnabled *bool  `json:"addons_enabled"`
		IsActive      *bool  `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	if req.PriceXOF != nil {
		h.db.Exec(r.Context(), `UPDATE plans SET price_xof=$2, updated_at=now() WHERE id=$1`, id, *req.PriceXOF)
	}
	if req.StorageBytes != nil {
		h.db.Exec(r.Context(), `UPDATE plans SET storage_bytes=$2, updated_at=now() WHERE id=$1`, id, *req.StorageBytes)
	}
	if req.MaxFileBytes != nil {
		h.db.Exec(r.Context(), `UPDATE plans SET max_file_bytes=$2, updated_at=now() WHERE id=$1`, id, *req.MaxFileBytes)
	}
	if req.MaxProjects != nil {
		h.db.Exec(r.Context(), `UPDATE plans SET max_projects=$2, updated_at=now() WHERE id=$1`, id, *req.MaxProjects)
	}
	if req.AddonsEnabled != nil {
		h.db.Exec(r.Context(), `UPDATE plans SET addons_enabled=$2, updated_at=now() WHERE id=$1`, id, *req.AddonsEnabled)
	}
	if req.IsActive != nil {
		h.db.Exec(r.Context(), `UPDATE plans SET is_active=$2, updated_at=now() WHERE id=$1`, id, *req.IsActive)
	}
	h.log.Async(actorID, "admin.billing.plan.update", "plan", id.String())
	response.NoContent(w)
}

// ── Billing — Channels ───────────────────────────────────────────────────────

type channelRow struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	LogoURL         string `json:"logo_url"`
	IsActive        bool   `json:"is_active"`
	MaintenanceNote string `json:"maintenance_note"`
	DisplayOrder    int    `json:"display_order"`
}

func (h *Handler) listAllChannels(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		`SELECT id, name, slug, COALESCE(logo_url,''), is_active, COALESCE(maintenance_note,''), display_order
		 FROM payment_channels ORDER BY display_order ASC, name ASC`)
	if err != nil {
		response.Error(w, err)
		return
	}
	defer rows.Close()
	var list []channelRow
	for rows.Next() {
		var c channelRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.LogoURL, &c.IsActive, &c.MaintenanceNote, &c.DisplayOrder); err != nil {
			response.Error(w, err)
			return
		}
		list = append(list, c)
	}
	if list == nil {
		list = []channelRow{}
	}
	response.OK(w, list)
}

func (h *Handler) createChannel(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)
	var req struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		LogoURL      string `json:"logo_url"`
		DisplayOrder int    `json:"display_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
		response.Error(w, apierr.ErrBadRequest("name and slug are required"))
		return
	}
	id := uuid.New()
	_, err := h.db.Exec(r.Context(),
		`INSERT INTO payment_channels (id, name, slug, logo_url, is_active, display_order, created_at, updated_at)
		 VALUES ($1,$2,$3,NULLIF($4,''),true,$5,now(),now())`,
		id, req.Name, req.Slug, req.LogoURL, req.DisplayOrder,
	)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(actorID, "admin.billing.channel.create", "channel", id.String())
	response.Created(w, map[string]string{"id": id.String()})
}

func (h *Handler) updateChannel(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid channel id"))
		return
	}
	var req struct {
		Name            *string `json:"name"`
		LogoURL         *string `json:"logo_url"`
		IsActive        *bool   `json:"is_active"`
		MaintenanceNote *string `json:"maintenance_note"`
		DisplayOrder    *int    `json:"display_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	if req.Name != nil {
		h.db.Exec(r.Context(), `UPDATE payment_channels SET name=$2, updated_at=now() WHERE id=$1`, id, *req.Name)
	}
	if req.LogoURL != nil {
		h.db.Exec(r.Context(), `UPDATE payment_channels SET logo_url=NULLIF($2,''), updated_at=now() WHERE id=$1`, id, *req.LogoURL)
	}
	if req.IsActive != nil {
		h.db.Exec(r.Context(), `UPDATE payment_channels SET is_active=$2, updated_at=now() WHERE id=$1`, id, *req.IsActive)
	}
	if req.MaintenanceNote != nil {
		h.db.Exec(r.Context(), `UPDATE payment_channels SET maintenance_note=NULLIF($2,''), updated_at=now() WHERE id=$1`, id, *req.MaintenanceNote)
	}
	if req.DisplayOrder != nil {
		h.db.Exec(r.Context(), `UPDATE payment_channels SET display_order=$2, updated_at=now() WHERE id=$1`, id, *req.DisplayOrder)
	}
	h.log.Async(actorID, "admin.billing.channel.update", "channel", id.String())
	response.NoContent(w)
}

func (h *Handler) deleteChannel(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid channel id"))
		return
	}
	if _, err := h.db.Exec(r.Context(), `DELETE FROM payment_channels WHERE id=$1`, id); err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(actorID, "admin.billing.channel.delete", "channel", id.String())
	response.NoContent(w)
}

func (h *Handler) uploadChannelLogo(w http.ResponseWriter, r *http.Request) {
	actorID, _ := mw.GetUserID(r)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid channel id"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20) // 2 MB max
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		response.Error(w, apierr.ErrBadRequest("file too large or invalid form"))
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("missing logo file"))
		return
	}
	defer file.Close()

	ct := header.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	key := fmt.Sprintf("system/logos/%s", id.String())
	if err := h.r2.Put(r.Context(), key, ct, file, header.Size); err != nil {
		response.Error(w, err)
		return
	}
	logoURL := h.r2.PublicURL(key)
	if _, err := h.db.Exec(r.Context(),
		`UPDATE payment_channels SET logo_url=$2, updated_at=now() WHERE id=$1`, id, logoURL,
	); err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(actorID, "admin.billing.channel.logo", "channel", id.String())
	response.OK(w, map[string]string{"logo_url": logoURL})
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
