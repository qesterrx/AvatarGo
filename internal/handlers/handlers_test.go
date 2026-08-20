package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/qesterrx/AvatarGo/internal/handlers/mocks"
	"github.com/qesterrx/AvatarGo/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestUploadAvatar_Handler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSrv := mocks.NewMockService(ctrl)
	h, _ := NewHandlers(mockSrv)

	router := h.GetRouter()

	// Подготовка multipart form
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.jpg")
	part.Write([]byte("fake image data"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/avatars", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", "user123")

	// Ожидаем вызов сервиса
	mockSrv.EXPECT().
		UploadAvatar(gomock.Any(), "user123", gomock.Any(), gomock.Any()).
		Return(&models.Avatar{ID: "new-id"}, nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp models.Avatar
	err := json.NewDecoder(w.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, "new-id", resp.ID)
}

func TestUploadAvatar_MissingUserID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSrv := mocks.NewMockService(ctrl)
	h, _ := NewHandlers(mockSrv)
	router := h.GetRouter()

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	writer.CreateFormFile("file", "test.jpg")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/avatars", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errResp models.ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	assert.Contains(t, errResp.Error, "X-User-ID header is required")
}

func TestGetAvatarById_Handler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSrv := mocks.NewMockService(ctrl)
	h, _ := NewHandlers(mockSrv)
	router := h.GetRouter()

	mockSrv.EXPECT().
		GetAvatar(gomock.Any(), "avatar123", "", "").
		Return([]byte("image data"), "image/jpeg", nil)

	req := httptest.NewRequest("GET", "/api/v1/avatars/avatar123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Equal(t, []byte("image data"), w.Body.Bytes())
}

func TestDeleteAvatar_Handler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSrv := mocks.NewMockService(ctrl)
	h, _ := NewHandlers(mockSrv)
	router := h.GetRouter()

	req := httptest.NewRequest("DELETE", "/api/v1/avatars/avatar123", nil)
	req.Header.Set("X-User-ID", "user123")
	mockSrv.EXPECT().
		DeleteAvatar(gomock.Any(), "avatar123", "user123").
		Return(nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}
