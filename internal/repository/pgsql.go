package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/qesterrx/AvatarGo/internal/config"
	"github.com/qesterrx/AvatarGo/internal/models"
)

type PGClient struct {
	otl *slog.Logger
	db  *sql.DB

	tracer trace.Tracer
}

func NewPGClient(cfg *config.DatabaseConfig) (*PGClient, error) {

	component := "PGClient"

	log := slog.With("component", component)
	tracer := otel.Tracer(component)

	dbConnStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.Password,
		cfg.DBName,
		"disable",
	)

	db, err := sql.Open("postgres", dbConnStr)
	if err != nil {
		return nil, err
	}

	// Проверяем подключение к БД
	if err := db.Ping(); err != nil {
		return nil, err
	}

	//Создаем driver для migrate используя существующее подключение
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, err
	}

	//Создаем экземпляр migrate
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		return nil, err
	}

	//Запускаем миграции
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return nil, err
	}

	pg := PGClient{
		db:     db,
		otl:    log,
		tracer: tracer,
	}

	return &pg, nil
}

func (r *PGClient) Close() {
	r.db.Close()
}

func (r *PGClient) Create(ctx context.Context, avatar *models.Avatar) error {

	ctx, span := r.tracer.Start(ctx, "Create")
	defer span.End()

	query := `
        INSERT INTO avatars (
            id, user_id, file_name, mime_type, size_bytes, s3_key,
            thumbnail_s3_keys, upload_status, processing_status, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        RETURNING created_at, updated_at
    `

	now := time.Now()
	err := r.db.QueryRowContext(ctx, query,
		avatar.ID,
		avatar.UserID,
		avatar.FileName,
		avatar.MimeType,
		avatar.SizeBytes,
		avatar.S3Key,
		avatar.ThumbnailS3Keys,
		avatar.UploadStatus,
		avatar.ProcessingStatus,
		now,
		now,
	).Scan(&avatar.CreatedAt, &avatar.UpdatedAt)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")

	return err
}

func (r *PGClient) GetByID(ctx context.Context, id string) (*models.Avatar, error) {

	ctx, span := r.tracer.Start(ctx, "GetByID")
	defer span.End()

	query := `
        SELECT id, user_id, file_name, mime_type, size_bytes, s3_key,
               thumbnail_s3_keys, upload_status, processing_status,
               created_at, updated_at, deleted_at
        FROM avatars
        WHERE id = $1 AND deleted_at IS NULL
    `

	var avatar models.Avatar
	var deletedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&avatar.ID,
		&avatar.UserID,
		&avatar.FileName,
		&avatar.MimeType,
		&avatar.SizeBytes,
		&avatar.S3Key,
		&avatar.ThumbnailS3Keys,
		&avatar.UploadStatus,
		&avatar.ProcessingStatus,
		&avatar.CreatedAt,
		&avatar.UpdatedAt,
		&deletedAt,
	)

	if err == sql.ErrNoRows {
		span.SetStatus(codes.Ok, "NoRows")
		return nil, nil
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if deletedAt.Valid {
		avatar.DeletedAt = &deletedAt.Time
	}

	span.SetStatus(codes.Ok, "")

	return &avatar, nil
}

func (r *PGClient) UpdateThumbnails(ctx context.Context, id string, thumbnails models.ThumbnailKeys) error {

	ctx, span := r.tracer.Start(ctx, "UpdateThumbnails")
	defer span.End()

	query := `
        UPDATE avatars
        SET thumbnail_s3_keys = $2, processing_status = 'completed', updated_at = NOW()
        WHERE id = $1
    `
	_, err := r.db.ExecContext(ctx, query, id, thumbnails)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")

	return nil
}

func (r *PGClient) Delete(ctx context.Context, id string) error {

	ctx, span := r.tracer.Start(ctx, "Delete")
	defer span.End()

	query := `
        UPDATE avatars
        SET deleted_at = NOW(), updated_at = NOW()
        WHERE id = $1 AND deleted_at IS NULL
    `
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if rows == 0 {
		span.SetStatus(codes.Ok, "NoRows")
		return nil // не найдено
	}

	span.SetStatus(codes.Ok, "")

	return nil
}

func (r *PGClient) Check() string {
	if err := r.db.Ping(); err != nil {
		return err.Error()
	}
	return "ok"
}
