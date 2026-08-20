package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/qesterrx/AvatarGo/internal/logger"
	"github.com/qesterrx/AvatarGo/internal/models"
)

//go:generate mockgen -source=services.go -destination=mocks/services_mocks.gen.go -package=mocks MetaDB, FileDB, Broker

type MetaDB interface {
	Create(ctx context.Context, avatar *models.Avatar) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*models.Avatar, error)
	Check() string
}

type FileDB interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, string, error)
	Del(ctx context.Context, key string) error
	GetURL(ctx context.Context, key string) string
	Check() string
}

type Broker interface {
	PublishUnloadEvent(event *models.AvatarUploadEvent) error
	PublishDeleteEvent(event *models.AvatarDeleteEvent) error
	Check() string
}

type avatarService struct {
	metaDB MetaDB
	fileDB FileDB
	asyncQ Broker
}

func NewAvatarService(metaDB MetaDB, fileDB FileDB, asyncQ Broker) (*avatarService, error) {
	srv := avatarService{
		metaDB: metaDB,
		fileDB: fileDB,
		asyncQ: asyncQ,
	}

	return &srv, nil
}

func (s *avatarService) UploadAvatar(ctx context.Context, userID string, file multipart.File, fileHeader *multipart.FileHeader) (*models.Avatar, error) {
	// Читаем файл
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUploadFailed, err)
	}

	_, _, err = image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid image content: %v", ErrInvalidFormat, err)
	}

	ext := filepath.Ext(fileHeader.Filename)

	// Определяем MIME-тип по расширению
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		// fallback – можно определить по содержимому через http.DetectContentType
		mimeType = http.DetectContentType(data)
	}

	// Генерируем ID
	id := uuid.New().String()

	// Генерируем S3 ключ
	s3Key := fmt.Sprintf("%s/%s/original%s", userID, id, ext)

	// Загружаем оригинал в S3
	if err := s.fileDB.Put(ctx, s3Key, data, mimeType); err != nil {
		return nil, fmt.Errorf("%w: failed to put to s3: %v", ErrUploadFailed, err)
	}

	// Создаем запись в БД
	avatar := &models.Avatar{
		ID:               id,
		UserID:           userID,
		FileName:         fileHeader.Filename,
		MimeType:         ext,
		SizeBytes:        int64(len(data)),
		URL:              s.fileDB.GetURL(ctx, s3Key),
		S3Key:            s3Key,
		ThumbnailS3Keys:  make(models.ThumbnailKeys),
		UploadStatus:     "completed",
		ProcessingStatus: "pending",
	}

	if err := s.metaDB.Create(ctx, avatar); err != nil {
		// Пытаемся удалить файл из S3 при ошибке
		_ = s.fileDB.Del(ctx, s3Key)
		return nil, fmt.Errorf("%w: failed to save metadata: %v", ErrUploadFailed, err)
	}

	// Асинхронно генерируем миниатюры
	event := models.AvatarUploadEvent{
		AvatarID: id,
		UserID:   userID,
		S3Key:    s3Key,
	}

	err = s.asyncQ.PublishUnloadEvent(&event)
	if err != nil {
		logger.Log.Error("Не удалось опубликовать событие для миниатюр: %v", err)
	}

	return avatar, nil
}

func (s *avatarService) GetAvatar(ctx context.Context, id string, size string, format string) ([]byte, string, error) {
	// Получаем метаданные
	avatar, err := s.metaDB.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if avatar == nil {
		return nil, "", ErrAvatarNotFound
	}

	// Определяем какой ключ использовать
	var s3Key string
	if size == "" || size == "original" {
		s3Key = avatar.S3Key
	} else {
		// Проверяем существование миниатюры
		if key, ok := avatar.ThumbnailS3Keys[size]; ok {
			s3Key = key
		} else {
			return nil, "", ErrInvalidSize
		}
	}

	// Получаем данные из S3
	data, contentType, err := s.fileDB.Get(ctx, s3Key)
	if err != nil {
		return nil, "", err
	}

	// Конвертируем формат если нужно
	if format != "" && format != "jpeg" && format != "png" && format != "webp" {
		return nil, "", ErrInvalidFormat
	}

	if format != "" {
		converted, err := s.convertImage(data, format)
		if err != nil {
			return nil, "", err
		}
		return converted, "image/" + format, nil
	}

	return data, contentType, nil
}

func (s *avatarService) GetAvatarMetadata(ctx context.Context, id string) (*models.AvatarMetadata, error) {
	avatar, err := s.metaDB.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if avatar == nil {
		return nil, ErrAvatarNotFound
	}

	// Получаем данные из S3 для определения размеров
	data, _, err := s.fileDB.Get(ctx, avatar.S3Key)
	if err != nil {
		return nil, err
	}

	dimensions, err := s.getImageDimensions(data)
	if err != nil {
		return nil, err
	}

	thumbnails := make([]models.Thumbnail, 0, len(avatar.ThumbnailS3Keys))
	for size, key := range avatar.ThumbnailS3Keys {
		url := s.fileDB.GetURL(ctx, key)
		thumbnails = append(thumbnails, models.Thumbnail{
			Size: size,
			URL:  url,
		})
	}

	return &models.AvatarMetadata{
		ID:       avatar.ID,
		UserID:   avatar.UserID,
		FileName: avatar.FileName,
		MimeType: avatar.MimeType,
		Size:     avatar.SizeBytes,
		Dimensions: models.Dimensions{
			Width:  dimensions.Width,
			Height: dimensions.Height,
		},
		Thumbnails: thumbnails,
		CreatedAt:  avatar.CreatedAt,
		UpdatedAt:  avatar.UpdatedAt,
	}, nil
}

func (s *avatarService) DeleteAvatar(ctx context.Context, id string, userID string) error {
	// Получаем метаданные
	avatar, err := s.metaDB.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if avatar == nil {
		return ErrAvatarNotFound
	}

	// Проверяем права
	if avatar.UserID != userID {
		return ErrAvatarForbidden
	}

	// Мягкое удаление из БД
	err = s.metaDB.Delete(ctx, id)
	if err != nil {
		return ErrDeletingFailed
	}

	//Ставим в очередь на удаление файлов
	thumbnailS3Keys := []string{avatar.S3Key}
	for _, url := range avatar.ThumbnailS3Keys {
		thumbnailS3Keys = append(thumbnailS3Keys, url)
	}

	event := models.AvatarDeleteEvent{
		AvatarID: id,
		S3Keys:   thumbnailS3Keys,
	}

	err = s.asyncQ.PublishDeleteEvent(&event)
	if err != nil {
		logger.Log.Error("Не удалось опубликовать событие для удаления: %v", err)
	}

	return nil

}

func (s *avatarService) convertImage(data []byte, toFormat string) ([]byte, error) {
	// Декодируем изображение
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	switch toFormat {
	case "jpeg":
		err = jpeg.Encode(buf, img, nil)
	case "png":
		err = png.Encode(buf, img)
	default:
		return nil, fmt.Errorf("unsupported format: %s", toFormat)
	}

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *avatarService) getImageDimensions(data []byte) (*models.Dimensions, error) {
	img, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &models.Dimensions{
		Width:  img.Width,
		Height: img.Height,
	}, nil
}
