package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"

	"github.com/KarpelesLab/gowebp"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/qesterrx/AvatarGo/internal/lerrors"
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
	PublishUnloadEvent(ctx context.Context, event *models.AvatarUploadEvent) error
	PublishDeleteEvent(ctx context.Context, event *models.AvatarDeleteEvent) error
	Check() string
}

type avatarService struct {
	otl    *slog.Logger
	tracer trace.Tracer

	metaDB MetaDB
	fileDB FileDB
	asyncQ Broker

	//Метрики
	mUploadAvatarCalls      metric.Int64Counter
	mGetAvatarCalls         metric.Int64Counter
	mGetAvatarMetadataCalls metric.Int64Counter
	mDeleteAvatarCalls      metric.Int64Counter
}

func NewAvatarService(metaDB MetaDB, fileDB FileDB, asyncQ Broker) (*avatarService, error) {

	component := "AvatarService"

	log := slog.With("component", component)

	tracer := otel.Tracer(component)

	meter := otel.Meter(component)
	mUploadAvatarCalls, err := meter.Int64Counter("app.uploadavatar.calls")
	if err != nil {
		return nil, fmt.Errorf("creating metric uploadavatar: %v", err)
	}
	mGetAvatarCalls, err := meter.Int64Counter("app.getavatar.calls")
	if err != nil {
		return nil, fmt.Errorf("creating metric getavatar: %v", err)
	}
	mGetAvatarMetadataCalls, err := meter.Int64Counter("app.getavatarmetadata.calls")
	if err != nil {
		return nil, fmt.Errorf("creating metric getavatarmetadata: %v", err)
	}
	mDeleteAvatarCalls, err := meter.Int64Counter("app.deleteavatar.calls")
	if err != nil {
		return nil, fmt.Errorf("creating metric getavatarmetadata: %v", err)
	}

	srv := avatarService{
		tracer:                  tracer,
		otl:                     log,
		metaDB:                  metaDB,
		fileDB:                  fileDB,
		asyncQ:                  asyncQ,
		mUploadAvatarCalls:      mUploadAvatarCalls,
		mGetAvatarCalls:         mGetAvatarCalls,
		mGetAvatarMetadataCalls: mGetAvatarMetadataCalls,
		mDeleteAvatarCalls:      mDeleteAvatarCalls,
	}

	return &srv, nil
}

func (s *avatarService) UploadAvatar(ctx context.Context, userID string, file multipart.File, fileHeader *multipart.FileHeader) (*models.Avatar, error) {

	ctx, span := s.tracer.Start(ctx, "UploadAvatar")
	defer span.End()

	s.otl.InfoContext(ctx, "Call", slog.String("method", "UploadAvatar"), slog.String("params", fmt.Sprintf("userID %s", userID)))

	s.mUploadAvatarCalls.Add(ctx, 1)

	// Читаем файл
	data, err := io.ReadAll(file)
	if err != nil {
		s.otl.ErrorContext(ctx, "error reading file", slog.String("method", "UploadAvatar"), slog.String("params", fmt.Sprintf("userID %s", userID)), slog.Any("error", err))
		return nil, fmt.Errorf("%w: %v", lerrors.ErrUploadFailed, err)
	}

	_, _, err = image.Decode(bytes.NewReader(data))
	if err != nil {
		s.otl.ErrorContext(ctx, "error invalid image content", slog.String("method", "UploadAvatar"), slog.String("params", fmt.Sprintf("userID %s", userID)), slog.Any("error", err))
		return nil, fmt.Errorf("%w: invalid image content: %v", lerrors.ErrInvalidFormat, err)
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
		s.otl.ErrorContext(ctx, "error s3 put", slog.String("method", "UploadAvatar"), slog.String("params", fmt.Sprintf("s3Key %s, mimeType %s", s3Key, mimeType)), slog.Any("error", err))
		return nil, fmt.Errorf("%w: failed to put to s3: %v", lerrors.ErrUploadFailed, err)
	}

	// Создаем запись в БД
	avatar := &models.Avatar{
		ID:               id,
		UserID:           userID,
		FileName:         fileHeader.Filename,
		MimeType:         mimeType,
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
		s.otl.ErrorContext(ctx, "error s3 del", slog.String("method", "UploadAvatar"), slog.String("params", fmt.Sprintf("s3Key %s", s3Key)), slog.Any("error", err))
		return nil, fmt.Errorf("%w: failed to save metadata: %v", lerrors.ErrUploadFailed, err)
	}

	// Асинхронно генерируем миниатюры
	event := models.AvatarUploadEvent{
		AvatarID: id,
		UserID:   userID,
		S3Key:    s3Key,
	}

	err = s.asyncQ.PublishUnloadEvent(ctx, &event)
	if err != nil {
		s.otl.ErrorContext(ctx, "error publish event", slog.String("method", "UploadAvatar"), slog.String("params", fmt.Sprintf("event %v", event)), slog.Any("error", err))
	}

	return avatar, nil
}

func (s *avatarService) GetAvatar(ctx context.Context, id string, size string, format string) ([]byte, string, error) {

	ctx, span := s.tracer.Start(ctx, "GetAvatar")
	defer span.End()

	s.otl.InfoContext(ctx, "Call", slog.String("method", "GetAvatar"), slog.String("params", fmt.Sprintf("id %s, size %s, format %s", id, size, format)))

	s.mGetAvatarCalls.Add(ctx, 1)

	// Получаем метаданные
	avatar, err := s.metaDB.GetByID(ctx, id)
	if err != nil {
		s.otl.ErrorContext(ctx, "error get metadata", slog.String("method", "GetAvatar"), slog.String("params", fmt.Sprintf("id %s", id)), slog.Any("error", err))
		return nil, "", err
	}
	if avatar == nil {
		return nil, "", lerrors.ErrAvatarNotFound
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
			return nil, "", lerrors.ErrInvalidSize
		}
	}

	// Получаем данные из S3
	data, contentType, err := s.fileDB.Get(ctx, s3Key)
	if err != nil {
		return nil, "", err
	}

	// Конвертируем формат если нужно
	if format != "" && format != "jpeg" && format != "png" && format != "webp" {
		return nil, "", lerrors.ErrInvalidFormat
	}

	if format != "" {
		converted, err := s.convertImage(ctx, data, format)
		if err != nil {
			s.otl.ErrorContext(ctx, "error convert image", slog.String("method", "GetAvatar"), slog.String("params", fmt.Sprintf("id %s format %s", id, format)), slog.Any("error", err))
			return nil, "", err
		}
		return converted, "image/" + format, nil
	}

	return data, contentType, nil
}

func (s *avatarService) GetAvatarMetadata(ctx context.Context, id string) (*models.AvatarMetadata, error) {

	ctx, span := s.tracer.Start(ctx, "GetAvatarMetadata")
	defer span.End()

	s.otl.InfoContext(ctx, "Call", slog.String("method", "GetAvatarMetadata"), slog.String("params", fmt.Sprintf("id %s", id)))

	s.mGetAvatarMetadataCalls.Add(ctx, 1)

	avatar, err := s.metaDB.GetByID(ctx, id)
	if err != nil {
		s.otl.ErrorContext(ctx, "error get metadata", slog.String("method", "GetAvatarMetadata"), slog.String("params", fmt.Sprintf("id %s", id)), slog.Any("error", err))
		return nil, err
	}
	if avatar == nil {
		return nil, lerrors.ErrAvatarNotFound
	}

	// Получаем данные из S3 для определения размеров
	data, _, err := s.fileDB.Get(ctx, avatar.S3Key)
	if err != nil {
		s.otl.ErrorContext(ctx, "error get data", slog.String("method", "GetAvatarMetadata"), slog.String("params", fmt.Sprintf("S3Key %s", avatar.S3Key)), slog.Any("error", err))
		return nil, err
	}

	dimensions, err := s.getImageDimensions(data)
	if err != nil {
		s.otl.ErrorContext(ctx, "error image dim", slog.String("method", "GetAvatarMetadata"), slog.String("params", fmt.Sprintf("id %s", id)), slog.Any("error", err))
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

	ctx, span := s.tracer.Start(ctx, "DeleteAvatar")
	defer span.End()

	s.otl.InfoContext(ctx, "Call", slog.String("method", "DeleteAvatar"), slog.String("params", fmt.Sprintf("id %s, userID %s", id, userID)))

	s.mDeleteAvatarCalls.Add(ctx, 1)

	// Получаем метаданные
	avatar, err := s.metaDB.GetByID(ctx, id)
	if err != nil {
		s.otl.ErrorContext(ctx, "error get metadata", slog.String("method", "DeleteAvatar"), slog.String("params", fmt.Sprintf("id %s", id)), slog.Any("error", err))
		return err
	}
	if avatar == nil {
		return lerrors.ErrAvatarNotFound
	}

	// Проверяем права
	if avatar.UserID != userID {
		return lerrors.ErrAvatarForbidden
	}

	// Мягкое удаление из БД
	err = s.metaDB.Delete(ctx, id)
	if err != nil {
		s.otl.ErrorContext(ctx, "error delete metadata", slog.String("method", "DeleteAvatar"), slog.String("params", fmt.Sprintf("id %s", id)), slog.Any("error", err))
		return lerrors.ErrDeletingFailed
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

	err = s.asyncQ.PublishDeleteEvent(ctx, &event)
	if err != nil {
		s.otl.ErrorContext(ctx, "error publish event", slog.String("method", "DeleteAvatar"), slog.String("params", fmt.Sprintf("id %s, userID %s", id, userID)), slog.Any("error", err))
	}

	return nil

}

func (s *avatarService) convertImage(ctx context.Context, data []byte, toFormat string) ([]byte, error) {

	ctx, span := s.tracer.Start(ctx, "convertImage")
	defer span.End()

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
	case "webp":
		err = gowebp.Encode(buf, img, nil)
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
