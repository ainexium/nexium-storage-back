package files

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"nexium.ai/api/internal/logs"
	"nexium.ai/api/internal/webhooks"
	"nexium.ai/api/pkg/apierr"
	mw "nexium.ai/api/pkg/middleware"
	"nexium.ai/api/pkg/response"
)

const maxMemory = 32 << 20 // 32 MB multipart memory

type Handler struct {
	svc         Service
	maxFileSize int64
	log         *logs.Store
	webhook     webhooks.Service
}

func NewHandler(svc Service, maxFileSizeMB int64, log *logs.Store, wh webhooks.Service) *Handler {
	return &Handler{svc: svc, maxFileSize: maxFileSizeMB << 20, log: log, webhook: wh}
}

// Routes for /api/v1/buckets/{bucketID}/files (JWT auth)
func (h *Handler) BucketRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.upload)
	r.Get("/", h.list)
	r.Post("/presign", h.presign)
	r.Post("/confirm", h.confirm)
	return r
}

// Routes for /api/v1/files (JWT auth)
func (h *Handler) FileRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{id}/download", h.download)
	r.Get("/{id}/stream", h.streamFile)
	r.Patch("/{id}", h.rename)
	r.Delete("/{id}", h.delete)
	return r
}

// Routes for /api/v1/ext/buckets/{bucketID}/files (API key auth)
func (h *Handler) ExtBucketRoutes() chi.Router {
	return h.BucketRoutes()
}

// Routes for /api/v1/ext/files (API key auth)
func (h *Handler) ExtFileRoutes() chi.Router {
	return h.FileRoutes()
}

// PublicRoutes — pas d'authentification, uniquement pour les buckets publics.
func (h *Handler) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{id}", h.publicDownload)
	return r
}

// publicDownload sert le contenu d'un fichier public en le proxiant depuis R2.
// Pas de redirect : le contenu passe par l'API NEXIUM pour éviter les problèmes CORS côté frontend.
func (h *Handler) publicDownload(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrNotFound)
		return
	}
	body, contentType, size, err := h.svc.StreamPublicFile(r.Context(), id)
	if err != nil {
		response.Error(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", contentType)
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

// streamFile proxie le contenu d'un fichier privé depuis R2, après vérification JWT.
func (h *Handler) streamFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid file id"))
		return
	}
	body, contentType, size, err := h.svc.StreamFile(r.Context(), userID, id)
	if err != nil {
		response.Error(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", contentType)
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	bucketID, err := uuid.Parse(chi.URLParam(r, "bucketID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid bucket id"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize)
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		if err.Error() == "http: request body too large" {
			response.Error(w, apierr.ErrBadRequest(fmt.Sprintf("file exceeds the %d MB limit", h.maxFileSize>>20)))
		} else {
			response.Error(w, apierr.ErrBadRequest("invalid multipart form"))
		}
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("missing file field"))
		return
	}
	defer file.Close()

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	f, err := h.svc.Upload(r.Context(), userID, bucketID, header.Filename, mimeType, header.Size, file)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "file.upload", "file", f.ID.String())
	if projectID, ok := mw.GetProjectID(r); ok {
		h.webhook.Fire(projectID, "file.created", f)
	}
	response.Created(w, f)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	bucketID, err := uuid.Parse(chi.URLParam(r, "bucketID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid bucket id"))
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	result, err := h.svc.List(r.Context(), userID, bucketID, search, page, perPage)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, result)
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid file id"))
		return
	}
	url, err := h.svc.GetDownloadURL(r.Context(), userID, id)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, DownloadResponse{URL: url})
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid file id"))
		return
	}
	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		response.Error(w, apierr.ErrBadRequest("filename is required"))
		return
	}
	f, err := h.svc.Rename(r.Context(), userID, id, req.Filename)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "file.rename", "file", id.String())
	if projectID, ok := mw.GetProjectID(r); ok {
		h.webhook.Fire(projectID, "file.renamed", f)
	}
	response.OK(w, f)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid file id"))
		return
	}
	if err := h.svc.Delete(r.Context(), userID, id); err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "file.delete", "file", id.String())
	if projectID, ok := mw.GetProjectID(r); ok {
		h.webhook.Fire(projectID, "file.deleted", map[string]string{"id": id.String()})
	}
	response.NoContent(w)
}

type presignRequest struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
}

type confirmRequest struct {
	FileID    uuid.UUID `json:"file_id"`
	ObjectKey string    `json:"object_key"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
}

func (h *Handler) presign(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	bucketID, err := uuid.Parse(chi.URLParam(r, "bucketID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid bucket id"))
		return
	}
	var req presignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" || req.MimeType == "" {
		response.Error(w, apierr.ErrBadRequest("filename and mime_type are required"))
		return
	}
	data, err := h.svc.PresignUpload(r.Context(), userID, bucketID, req.Filename, req.MimeType)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, data)
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	bucketID, err := uuid.Parse(chi.URLParam(r, "bucketID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid bucket id"))
		return
	}
	var req confirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == uuid.Nil || req.ObjectKey == "" {
		response.Error(w, apierr.ErrBadRequest("file_id and object_key are required"))
		return
	}
	f, err := h.svc.ConfirmUpload(r.Context(), userID, bucketID, req.FileID, req.ObjectKey, req.Filename, req.MimeType, req.SizeBytes)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "file.upload", "file", f.ID.String())
	response.Created(w, f)
}
