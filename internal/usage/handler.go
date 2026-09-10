package usage

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"nexium.ai/api/pkg/apierr"
	mw "nexium.ai/api/pkg/middleware"
	"nexium.ai/api/pkg/response"
)

type Handler struct{ svc Service }

func NewHandler(svc Service) *Handler { return &Handler{svc} }

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.getSummary)
	return r
}

func (h *Handler) getSummary(w http.ResponseWriter, r *http.Request) {
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
	sum, err := h.svc.GetSummary(r.Context(), userID, projectID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, sum)
}
