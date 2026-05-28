package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/asbm77/go-musthave-diploma-tpl/internal/auth"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/models"
	"github.com/asbm77/go-musthave-diploma-tpl/internal/storage"
	"github.com/asbm77/go-musthave-diploma-tpl/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler обрабатывает запросы аутентификации
type AuthHandler struct {
	storage *storage.PostgresStorage
	jwtAuth *auth.JWTAuth
}

// NewAuthHandler создаёт новый AuthHandler
func NewAuthHandler(storage *storage.PostgresStorage, jwtAuth *auth.JWTAuth) *AuthHandler {
	return &AuthHandler{
		storage: storage,
		jwtAuth: jwtAuth,
	}
}

// Register обрабатывает регистрацию пользователя
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Logger.Errorw("Failed to hash password", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	user, err := h.storage.CreateUser(r.Context(), req.Login, string(hashedPassword))
	if err != nil {
		if errors.Is(err, storage.ErrUserExists) {
			http.Error(w, "Login already taken", http.StatusConflict)
			return
		}
		logger.Logger.Errorw("Failed to create user", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	token, err := h.jwtAuth.GenerateToken(user.ID)
	if err != nil {
		logger.Logger.Errorw("Failed to generate token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   86400,
	})

	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}

// Login обрабатывает аутентификацию пользователя
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	user, err := h.storage.GetUserByLogin(r.Context(), req.Login)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			http.Error(w, "Invalid login or password", http.StatusUnauthorized)
			return
		}
		logger.Logger.Errorw("Failed to get user", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		return
	}

	token, err := h.jwtAuth.GenerateToken(user.ID)
	if err != nil {
		logger.Logger.Errorw("Failed to generate token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   86400,
	})

	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}
