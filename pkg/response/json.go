package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"nexium.ai/api/pkg/apierr"
)

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func OK(w http.ResponseWriter, v any) {
	JSON(w, http.StatusOK, v)
}

func Created(w http.ResponseWriter, v any) {
	JSON(w, http.StatusCreated, v)
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func Error(w http.ResponseWriter, err error) {
	var apiErr *apierr.APIError
	if errors.As(err, &apiErr) {
		JSON(w, apiErr.Code, apiErr)
		return
	}
	JSON(w, http.StatusInternalServerError, apierr.ErrInternal)
}
