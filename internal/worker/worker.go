package worker

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"

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
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, string, error)
	Del(ctx context.Context, key string) error
	GetURL(ctx context.Context, key string) string
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
			logger.Log.Info("Остановка Uploading по контексту")
			return ctx.Err()

		case event, ok := <-events:
			if !ok {
				return nil
			}
			logger.Log.Info("Получено событие UPLOAD %v", event.Event)
			err := aw.HandleUploadEvent(ctx, event.Event)
			if err != nil {
				logger.Log.Info("Ошибка обработки %v", err.Error())
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
			logger.Log.Info("Остановка Deleting по контексту")
			return ctx.Err()

		case event, ok := <-events:
			if !ok {
				return nil
			}
			logger.Log.Info("Получено событие DELETE %v", event.Event)
			err := aw.HandleDeleteEvent(ctx, event.Event)
			if err != nil {
				logger.Log.Info("Ошибка обработки %v", err.Error())
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
	createThumbnail := func(size uint, img image.Image) ([]byte, error) {
		resized := resize.Thumbnail(size, size, img, resize.Lanczos3)
		buf := new(bytes.Buffer)
		err := jpeg.Encode(buf, resized, nil)
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
		data, err := createThumbnail(thumb.size, img)
		if err != nil {
			return err
		}
		s3Key := fmt.Sprintf("%s/%s/%s%s", avatar.UserID, avatar.ID, thumb.name, avatar.MimeType)
		thumbnailKeys[thumb.name] = s3Key
		if err := aw.fileDB.Put(ctx, s3Key, data, mime); err != nil {
			return err
		}
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
