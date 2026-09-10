package auth

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
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

func (h *Handler) Routes(jwtSecret string) chi.Router {
	r := chi.NewRouter()
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/verify-email", h.verifyEmail)
	r.Post("/resend-verification", h.resendVerification)
	r.Post("/forgot-password", h.forgotPassword)
	r.Post("/verify-reset-code", h.verifyResetCode)
	r.Post("/reset-password", h.resetPassword)
	r.Group(func(r chi.Router) {
		r.Use(mw.Authenticate(jwtSecret))
		r.Post("/logout", h.logout)
		r.Get("/me", h.me)
		r.Patch("/me", h.updateMe)
	})
	return r
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	res, err := h.svc.Register(r.Context(), &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.Created(w, res)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	pair, err := h.svc.Login(r.Context(), &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(pair.User.ID, "auth.login", "user", pair.User.ID.String())
	response.OK(w, pair)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	pair, err := h.svc.Refresh(r.Context(), &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, pair)
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req VerifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Code == "" {
		response.Error(w, apierr.ErrBadRequest("email and code are required"))
		return
	}
	pair, err := h.svc.VerifyEmail(r.Context(), &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(pair.User.ID, "auth.verify_email", "user", pair.User.ID.String())
	response.OK(w, pair)
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req ResendVerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		response.Error(w, apierr.ErrBadRequest("email is required"))
		return
	}
	if err := h.svc.ResendVerification(r.Context(), req.Email); err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, map[string]string{"message": "code sent if applicable"})
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		response.Error(w, apierr.ErrBadRequest("email is required"))
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email); err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, map[string]string{"message": "if this email exists, a reset code was sent"})
}

func (h *Handler) verifyResetCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Code == "" {
		response.Error(w, apierr.ErrBadRequest("email and code are required"))
		return
	}
	if err := h.svc.VerifyResetCode(r.Context(), req.Email, req.Code); err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, map[string]string{"message": "code valid"})
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Code == "" || req.Password == "" {
		response.Error(w, apierr.ErrBadRequest("email, code and password are required"))
		return
	}
	if err := h.svc.ResetPassword(r.Context(), &req); err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, map[string]string{"message": "password updated"})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "auth.logout", "user", userID.String())
	response.NoContent(w)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	user, err := h.svc.Me(r.Context(), userID)
	if err != nil {
		response.Error(w, err)
		return
	}
	response.OK(w, user)
}

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.GetUserID(r)
	if !ok {
		response.Error(w, apierr.ErrUnauthorized)
		return
	}
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, apierr.ErrBadRequest("invalid JSON"))
		return
	}
	user, err := h.svc.UpdateProfile(r.Context(), userID, &req)
	if err != nil {
		response.Error(w, err)
		return
	}
	h.log.Async(userID, "auth.profile.update", "user", userID.String())
	response.OK(w, user)
}
