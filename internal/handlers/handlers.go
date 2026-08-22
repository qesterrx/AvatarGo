package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/qesterrx/AvatarGo/internal/lerrors"
	"github.com/qesterrx/AvatarGo/internal/middleware"
	"github.com/qesterrx/AvatarGo/internal/models"
)

const MaxFileSize = 10 * 1024 * 1024

//go:generate mockgen -source=handlers.go -destination=mocks/handlers_mocks.gen.go -package=mocks Service
type Service interface {
	UploadAvatar(ctx context.Context, userID string, file multipart.File, fileHeader *multipart.FileHeader) (*models.Avatar, error)
	GetAvatar(ctx context.Context, id string, size string, format string) ([]byte, string, error)
	GetAvatarMetadata(ctx context.Context, id string) (*models.AvatarMetadata, error)
	DeleteAvatar(ctx context.Context, id string, userID string) error
	HealthCheck() (*models.HealthCheckResponse, error)
}

type Handlers struct {
	srv Service
}

func NewHandlers(srv Service) (*Handlers, error) {

	h := Handlers{srv: srv}

	return &h, nil
}

func (h *Handlers) GetRouter() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.LoggingMiddleware)

	// Загрузка аватарки
	r.Post(`/api/v1/avatars`, h.UploadAvatar)

	// Получение аватарки
	r.Get(`/api/v1/avatars/{avatar_id}`, h.GetAvatarByID)

	// Удаление аватарки
	r.Delete(`/api/v1/avatars/{avatar_id}`, h.DeleteAvatarByID)

	// Получение метаданных аватарки
	r.Get(`/api/v1/avatars/{avatar_id}/metadata`, h.GetAvatarMetadata)

	// Проверка работоспособности
	r.Get(`/health`, h.HealthCheck)

	// Веб-интерфейс
	r.Get(`/web/upload`, h.WebUploadForm)

	return r
}

// Загрузка аватарки
func (h *Handlers) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "X-User-ID header is required"})
		return
	}

	if err := r.ParseMultipartForm(MaxFileSize); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		json.NewEncoder(w).Encode(models.ErrorResponse{
			Error:   "File too large",
			MaxSize: MaxFileSize,
		})
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: fmt.Sprintf("File is requared: %v", err)})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{
			Error:   "Invalid file format",
			Details: "Supported formats: jpeg, png, webp",
		})
		return
	}

	avatar, err := h.srv.UploadAvatar(r.Context(), userID, file, fileHeader)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: fmt.Sprintf("Failed to upload avatar: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(avatar)
}

// Получение аватарки по ИД аватарки
func (h *Handlers) GetAvatarByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "avatar_id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Avatar ID is required"})
		return
	}

	size := r.URL.Query().Get("size")
	format := r.URL.Query().Get("format")

	if size != "" && size != "100x100" && size != "300x300" && size != "original" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{
			Error:   "Invalid size parameter",
			Details: "Allowed values: 100x100, 300x300, original",
		})
		return
	}

	if format != "" && format != "jpeg" && format != "png" && format != "webp" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{
			Error:   "Invalid format parameter",
			Details: "Allowed values: jpeg, png, webp",
		})
		return
	}

	data, contentType, err := h.srv.GetAvatar(r.Context(), id, size, format)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Avatar not found"})
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// Удаление аватарки по ИД аватарки
func (h *Handlers) DeleteAvatarByID(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "X-User-ID header is required"})
		return
	}

	id := chi.URLParam(r, "avatar_id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Avatar ID is required"})
		return
	}

	err := h.srv.DeleteAvatar(r.Context(), id, userID)
	if err != nil {

		if errors.Is(err, lerrors.ErrAvatarForbidden) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error:   "Forbidden",
				Details: "You can only delete your avatars only",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Avatar not found"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Получение метаданных аватарки
func (h *Handlers) GetAvatarMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "avatar_id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Avatar ID is required"})
		return
	}

	metadata, err := h.srv.GetAvatarMetadata(r.Context(), id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Avatar not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(metadata)

}

// Проверка работоспособности
func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	check, err := h.srv.HealthCheck()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Error in healthcheck"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(check)
}

// Веб-интерфейс
func (h *Handlers) WebUploadForm(w http.ResponseWriter, r *http.Request) {
	content, err := os.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "Error reading index.html", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(content))
}
