package services

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"mime/multipart"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/qesterrx/AvatarGo/internal/lerrors"
	"github.com/qesterrx/AvatarGo/internal/models"
	"github.com/qesterrx/AvatarGo/internal/services/mocks"
	"github.com/stretchr/testify/assert"
)

// Вспомогательный тип testFile для имитации multipart.File
type testFile struct {
	*bytes.Reader
}

func newTestFile(data []byte) *testFile {
	return &testFile{bytes.NewReader(data)}
}

func (f *testFile) Close() error {
	return nil
}

// Создание изоображения
func makeTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	// устанавливаем какой-нибудь цвет, например, белый
	// или просто закодируем
	buf := new(bytes.Buffer)
	err := jpeg.Encode(buf, img, nil)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestUploadAvatar_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMeta := mocks.NewMockMetaDB(ctrl)
	mockFile := mocks.NewMockFileDB(ctrl)
	mockBroker := mocks.NewMockBroker(ctrl)

	srv, _ := NewAvatarService(mockMeta, mockFile, mockBroker)

	ctx := context.Background()
	userID := "1"
	fileHeader := &multipart.FileHeader{Filename: "test.jpg"}
	file := newTestFile(makeTestJPEG())

	// ожидаем сохранения в S3
	mockFile.EXPECT().
		Put(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	// ожидаем  получение url
	mockFile.EXPECT().
		GetURL(ctx, gomock.Any()).
		Return("")

	// ожидаем создания записи в БД
	mockMeta.EXPECT().
		Create(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, avatar *models.Avatar) error {
			avatar.CreatedAt = time.Now()
			avatar.UpdatedAt = time.Now()
			return nil
		})

	// ожидаем публикации события
	mockBroker.EXPECT().
		PublishUnloadEvent(gomock.Any()).
		Return(nil)

	avatar, err := srv.UploadAvatar(ctx, userID, file, fileHeader)
	assert.NoError(t, err)
	assert.NotNil(t, avatar)
	assert.Equal(t, userID, avatar.UserID)
}

func TestUploadAvatar_InvalidImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv, _ := NewAvatarService(nil, nil, nil) // моки не нужны, т.к. валидация до них

	ctx := context.Background()
	invalidData := []byte("not an image")
	file := newTestFile(invalidData)
	fileHeader := &multipart.FileHeader{Filename: "fake.jpg"}

	_, err := srv.UploadAvatar(ctx, "user", file, fileHeader)
	assert.ErrorIs(t, err, lerrors.ErrInvalidFormat)
}

func TestGetAvatar_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMeta := mocks.NewMockMetaDB(ctrl)
	mockFile := mocks.NewMockFileDB(ctrl)
	srv, _ := NewAvatarService(mockMeta, mockFile, nil)

	ctx := context.Background()
	id := "avatar123"
	avatar := &models.Avatar{
		ID:     id,
		S3Key:  "original.jpg",
		UserID: "1",
	}
	mockMeta.EXPECT().GetByID(ctx, id).Return(avatar, nil)
	mockFile.EXPECT().Get(ctx, avatar.S3Key).Return(makeTestJPEG(), "image/jpeg", nil)

	data, contentType, err := srv.GetAvatar(ctx, id, "", "")
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", contentType)
	assert.NotEmpty(t, data)
}

func TestDeleteAvatar_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMeta := mocks.NewMockMetaDB(ctrl)
	mockBroker := mocks.NewMockBroker(ctrl)
	srv, _ := NewAvatarService(mockMeta, nil, mockBroker)

	ctx := context.Background()
	id := "avatar123"
	userID := "2"
	avatar := &models.Avatar{
		ID:              id,
		UserID:          userID,
		S3Key:           "original.jpg",
		ThumbnailS3Keys: models.ThumbnailKeys{"100x100": "thumb.jpg"},
	}

	mockMeta.EXPECT().GetByID(ctx, id).Return(avatar, nil)
	mockMeta.EXPECT().Delete(ctx, id).Return(nil)
	mockBroker.EXPECT().PublishDeleteEvent(gomock.Any()).Return(nil)

	err := srv.DeleteAvatar(ctx, id, userID)
	assert.NoError(t, err)
}
