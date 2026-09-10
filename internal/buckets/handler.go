package buckets

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"nexium.ai/api/internal/logs"
	"nexium.ai/api/pkg/apierr"
	mw "nexium.ai/api/pkg/middleware"
	"nexium.ai/api/pkg/response"
)

type Handler struct {
	svc Service
	log *logs.Store
}

func NewHandler(svc Service, log *logs.Store) *Handler { return &Handler{svc, log} }

// Routes for /api/v1/projects/{projectID}/buckets
func (h *Handler) ProjectRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	return r
}

// Routes for /api/v1/buckets
func (h *Handler) BucketRoutes() chi.Router {
	r := chi.NewRouter()
	r.Patch("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid project id"))
		return
	}
	buckets, err := h.svc.List(r.Context(), userID, projectID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, buckets)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid project id"))
		return
	}
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	b, err := h.svc.Create(r.Context(), userID, projectID, &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "bucket.create", "bucket", b.ID.String())
	response.Created(w, b)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid bucket id"))
		return
	}
	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	b, err := h.svc.Update(r.Context(), userID, id, &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, b)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid bucket id"))
		return
	}
	if err := h.svc.Delete(r.Context(), userID, id); err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "bucket.delete", "bucket", id.String())
	response.NoContent(w)
}
