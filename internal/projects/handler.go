package projects

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

// Routes construit le router projets avec les sous-routes imbriquées.
// sub permet d'injecter les handlers enfants (buckets, api-keys, usage)
// directement sous /{projectID} pour éviter les conflits Chi.
func (h *Handler) Routes(sub func(chi.Router)) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Route("/{projectID}", func(r chi.Router) {
		r.Get("/", h.getByID)
		r.Patch("/", h.update)
		r.Delete("/", h.delete)
		if sub != nil {
			sub(r)
		}
	})
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	projects, err := h.svc.List(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, projects)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	p, err := h.svc.Create(r.Context(), userID, &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "project.create", "project", p.ID.String())
	response.Created(w, p)
}

func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid project id"))
		return
	}
	p, err := h.svc.GetByID(r.Context(), userID, id)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, p)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid project id"))
		return
	}
	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	p, err := h.svc.Update(r.Context(), userID, id, &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, p)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid project id"))
		return
	}
	if err := h.svc.Delete(r.Context(), userID, id); err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "project.delete", "project", id.String())
	response.NoContent(w)
}
