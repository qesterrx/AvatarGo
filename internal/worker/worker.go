package worker

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log/slog"
	"path/filepath"

	"github.com/KarpelesLab/gowebp" // внешний пакет для кодирования WebP
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/nfnt/resize"
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
	otl    *slog.Logger
	metaDB MetaDB
	fileDB FileDB
	asyncQ Broker
	tracer trace.Tracer
}

func NewAvatarWorker(metaDB MetaDB, fileDB FileDB, asyncQ Broker) (*avatarWorker, error) {

	component := "AvatarWorker"
	log := slog.With("component", component)
	tracer := otel.Tracer(component)

	srv := avatarWorker{
		metaDB: metaDB,
		fileDB: fileDB,
		asyncQ: asyncQ,
		otl:    log,
		tracer: tracer,
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

			aw.otl.InfoContext(ctx, "Stopping Uploading by ctx")
			return ctx.Err()

		case event, ok := <-events:
			if !ok {
				return nil
			}

			meter := otel.Meter("Worker-Uploading")
			counter, _ := meter.Int64Counter("calls")
			counter.Add(ctx, 1)

			aw.otl.InfoContext(ctx, fmt.Sprintf("Got event UPLOAD %v", event.Event))
			err := aw.HandleUploadEvent(event.Ctx, event.Event)
			if err != nil {
				aw.otl.ErrorContext(ctx, "Error HandleUploadEvent: "+err.Error())
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
			aw.otl.InfoContext(ctx, "Stopping Deleting by ctx")
			return ctx.Err()

		case event, ok := <-events:

			if !ok {
				return nil
			}

			meter := otel.Meter("Worker-Deleting")
			counter, _ := meter.Int64Counter("calls")
			counter.Add(ctx, 1)

			aw.otl.InfoContext(ctx, fmt.Sprintf("Got event DELETE %v", event.Event))
			err := aw.HandleDeleteEvent(event.Ctx, event.Event)
			if err != nil {
				aw.otl.ErrorContext(ctx, "Error HandleDeleteEvent: "+err.Error())
				event.Nack()
				continue
			}
			event.Ack()

		}
	}

}

// Пример обработки события в worker
func (aw *avatarWorker) HandleUploadEvent(ctx context.Context, event *models.AvatarUploadEvent) error {

	ctx, span := aw.tracer.Start(ctx, "HandleUploadEvent")
	defer span.End()

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

		_, subSpan := aw.tracer.Start(ctx, fmt.Sprintf("createThumbnail size: %d, type: %s", size, mimeType))
		defer subSpan.End()

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

	ctx, span := aw.tracer.Start(ctx, "HandleDeleteEvent")
	defer span.End()

	for _, s3key := range event.S3Keys {
		err := aw.fileDB.Del(ctx, s3key)
		if err != nil {
			return err
		}
	}

	return nil
}
