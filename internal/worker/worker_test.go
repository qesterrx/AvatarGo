package worker

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/qesterrx/AvatarGo/internal/models"
	"github.com/qesterrx/AvatarGo/internal/worker/mocks"
	"github.com/stretchr/testify/assert"
)

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

func TestHandleUploadEvent_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMeta := mocks.NewMockMetaDB(ctrl)
	mockFile := mocks.NewMockFileDB(ctrl)

	worker := &avatarWorker{
		metaDB: mockMeta,
		fileDB: mockFile,
	}

	ctx := context.Background()
	event := &models.AvatarUploadEvent{
		AvatarID: "1",
		UserID:   "1",
		S3Key:    "1/1/original.jpg",
	}

	// ожидаем получения метаданных
	mockMeta.EXPECT().
		GetByID(ctx, event.AvatarID).
		Return(&models.Avatar{
			ID:       event.AvatarID,
			UserID:   event.UserID,
			S3Key:    event.S3Key,
			MimeType: ".jpg",
		}, nil)

	// ожидаем загрузки оригинала
	mockFile.EXPECT().
		Get(ctx, event.S3Key).
		Return(makeTestJPEG(), "image/jpeg", nil)

	// ожидаем проверки наличия миниатюр
	mockFile.EXPECT().
		Exists(ctx, "1/1/100x100.jpg").
		Return(false, nil)

	// ожидаем проверки наличия миниатюр
	mockFile.EXPECT().
		Exists(ctx, "1/1/300x300.jpg").
		Return(false, nil)

	// ожидаем сохранения двух миниатюр
	mockFile.EXPECT().
		Put(ctx, "1/1/100x100.jpg", gomock.Any(), "image/jpeg").
		Return(nil)
	mockFile.EXPECT().
		Put(ctx, "1/1/300x300.jpg", gomock.Any(), "image/jpeg").
		Return(nil)

	// ожидаем обновления БД
	mockMeta.EXPECT().
		UpdateThumbnails(ctx, event.AvatarID, gomock.Any()).
		Return(nil)

	err := worker.HandleUploadEvent(ctx, event)
	assert.NoError(t, err)
}

func TestHandleUploadEvent_GetMetaError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMeta := mocks.NewMockMetaDB(ctrl)
	worker := &avatarWorker{metaDB: mockMeta}

	ctx := context.Background()
	event := &models.AvatarUploadEvent{AvatarID: "2"}

	mockMeta.EXPECT().
		GetByID(ctx, event.AvatarID).
		Return(nil, errors.New("db error"))

	err := worker.HandleUploadEvent(ctx, event)
	assert.Error(t, err)
}

func TestHandleDeleteEvent_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFile := mocks.NewMockFileDB(ctrl)
	worker := &avatarWorker{fileDB: mockFile}

	ctx := context.Background()
	event := &models.AvatarDeleteEvent{
		AvatarID: "avatar123",
		S3Keys:   []string{"key1", "key2"},
	}

	// ожидаем удаления каждого ключа без ошибок
	mockFile.EXPECT().Del(ctx, "key1").Return(nil)
	mockFile.EXPECT().Del(ctx, "key2").Return(nil)

	err := worker.HandleDeleteEvent(ctx, event)
	assert.NoError(t, err)
}
