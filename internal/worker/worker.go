package worker

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"

	"github.com/KarpelesLab/gowebp" // внешний пакет для кодирования WebP

	"github.com/nfnt/resize"
	"github.com/qesterrx/AvatarGo/internal/logger"
	"github.com/qesterrx/AvatarGo/internal/models"
)

//go:generate mockgen -source=worker.go -destination=mocks/worker_mocks.gen.go -package=mocks MetaDB, FileDB, Broker

type MetaDB interface {
	GetByID(ctx context.Context, id string) (*models.Avatar, error)
	UpdateThumbnails(ctx context.Context, id string, thumbnails models.ThumbnailKeys) error
}

type FileDB interface {
	Exists(ctx context.Context, key string) (bool, error)
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, string, error)
	Del(ctx context.Context, key string) error
}

type Broker interface {
	GetChUnloadEvent(ctx context.Context) (<-chan *models.AvatarUploadEventHandle, error)
	GetChDeleteEvent(ctx context.Context) (<-chan *models.AvatarDeleteEventHandle, error)
}

type avatarWorker struct {
	metaDB MetaDB
	fileDB FileDB
	asyncQ Broker
}

func NewAvatarWorker(metaDB MetaDB, fileDB FileDB, asyncQ Broker) (*avatarWorker, error) {
	srv := avatarWorker{
		metaDB: metaDB,
		fileDB: fileDB,
		asyncQ: asyncQ,
	}

	return &srv, nil
}

func (aw *avatarWorker) Uploading(ctx context.Context) error {
	events, err := aw.asyncQ.GetChUnloadEvent(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("Stopping Uploading by ctx")
			return ctx.Err()

		case event, ok := <-events:
			if !ok {
				return nil
			}
			logger.Log.Info("Got event UPLOAD %v", event.Event)
			err := aw.HandleUploadEvent(ctx, event.Event)
			if err != nil {
				logger.Log.Error("Error HandleUploadEvent %v", err.Error())
				event.Nack()
				continue
			}
			event.Ack()

		}
	}

}

func (aw *avatarWorker) Deleting(ctx context.Context) error {
	events, err := aw.asyncQ.GetChDeleteEvent(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("Stopping Deleting by ctx")
			return ctx.Err()

		case event, ok := <-events:
			if !ok {
				return nil
			}
			logger.Log.Info("Got event DELETE %v", event.Event)
			err := aw.HandleDeleteEvent(ctx, event.Event)
			if err != nil {
				logger.Log.Error("Error HandleDeleteEvent %v", err.Error())
				event.Nack()
				continue
			}
			event.Ack()

		}
	}

}

// Пример обработки события в worker
func (aw *avatarWorker) HandleUploadEvent(ctx context.Context, event *models.AvatarUploadEvent) error {
	// Получаем метаданные из БД
	avatar, err := aw.metaDB.GetByID(ctx, event.AvatarID)
	if err != nil {
		return err
	}

	ext := filepath.Ext(avatar.S3Key)

	// Загружаем оригинал из S3
	data, mime, err := aw.fileDB.Get(ctx, avatar.S3Key)
	if err != nil {
		return err
	}

	// Преобразуем к картинке
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}

	// Функция для создания миниатюры
	createThumbnail := func(size uint, mimeType string, img image.Image) ([]byte, error) {
		resized := resize.Thumbnail(size, size, img, resize.Lanczos3)
		buf := new(bytes.Buffer)

		switch mimeType {
		case "image/jpeg":
			err = jpeg.Encode(buf, resized, nil)
		case "image/png":
			err = png.Encode(buf, img)
		case "image/webp":
			err = gowebp.Encode(buf, img, nil)
		default:
			return nil, fmt.Errorf("unsupported format: %s", mimeType)
		}

		return buf.Bytes(), err
	}

	// Создаем миниатюры
	thumbnails := []struct {
		name string
		size uint
		data []byte
	}{
		{"100x100", 100, nil},
		{"300x300", 300, nil},
	}

	thumbnailKeys := make(models.ThumbnailKeys)

	// Сохраняем миниатюры в S3
	for _, thumb := range thumbnails {
		s3Key := fmt.Sprintf("%s/%s/%s%s", avatar.UserID, avatar.ID, thumb.name, ext)

		//Проверка наличия файла
		exists, err := aw.fileDB.Exists(ctx, s3Key)
		if err != nil {
			return err
		}

		//Если файла нет -
		if !exists {

			data, err := createThumbnail(thumb.size, mime, img)
			if err != nil {
				return err
			}

			if err := aw.fileDB.Put(ctx, s3Key, data, mime); err != nil {
				return err
			}

		}

		thumbnailKeys[thumb.name] = s3Key

	}

	//Записываем ссылки на миниатюры + статус
	return aw.metaDB.UpdateThumbnails(ctx, event.AvatarID, thumbnailKeys)

}

// Пример обработки события в worker
func (aw *avatarWorker) HandleDeleteEvent(ctx context.Context, event *models.AvatarDeleteEvent) error {

	for _, s3key := range event.S3Keys {
		err := aw.fileDB.Del(ctx, s3key)
		if err != nil {
			return err
		}
	}

	return nil
}
