package apikeys

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"nexium.ai/api/pkg/apierr"
	mw "nexium.ai/api/pkg/middleware"
	"nexium.ai/api/pkg/response"
)

type Handler struct{ svc Service }

func NewHandler(svc Service) *Handler { return &Handler{svc} }

// Routes for /api/v1/projects/{projectID}/api-keys
func (h *Handler) ProjectRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	return r
}

// Routes for /api/v1/api-keys
func (h *Handler) APIKeyRoutes() chi.Router {
	r := chi.NewRouter()
	r.Delete("/{id}", h.revoke)
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
	keys, err := h.svc.List(r.Context(), userID, projectID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, keys)
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
	res, err := h.svc.Create(r.Context(), userID, projectID, &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, res)
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid api key id"))
		return
	}
	if err := h.svc.Revoke(r.Context(), userID, id); err != nil {
		response.Error(w, err)
		return
	}
	response.NoContent(w)
}
