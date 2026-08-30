package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	cfg "github.com/qesterrx/AvatarGo/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Client struct {
	otl        *slog.Logger
	s3Client   *s3.Client
	bucketName string
	tracer     trace.Tracer
}

func NewS3Client(cfg *cfg.S3Config) (*S3Client, error) {
	component := "S3Client"
	log := slog.With("component", component)
	tracer := otel.Tracer(component)

	ctx := context.Background()

	endpoint := cfg.Endpoint
	if !strings.HasPrefix(endpoint, "http://") {
		endpoint = "http://" + endpoint
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(cfg.BucketName),
	})

	if err != nil {
		// Проверяем, является ли ошибка "BucketAlreadyOwnedByYou"
		var bucketAlreadyOwned *types.BucketAlreadyOwnedByYou
		if !errors.As(err, &bucketAlreadyOwned) {
			return nil, err
		}
	}

	s3 := S3Client{
		s3Client:   s3Client,
		bucketName: cfg.BucketName,
		otl:        log,
		tracer:     tracer,
	}

	return &s3, nil
}

func (c *S3Client) Put(ctx context.Context, key string, data []byte, contentType string) error {

	ctx, span := c.tracer.Start(ctx, "Put")
	defer span.End()

	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucketName),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(data))),
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")

	return nil
}

func (c *S3Client) Exists(ctx context.Context, key string) (bool, error) {

	ctx, span := c.tracer.Start(ctx, "Exists")
	defer span.End()

	_, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		var notFound *types.NotFound
		var noSuchKey *types.NoSuchKey

		// Нет объекта или нет ключа
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
			span.SetStatus(codes.Ok, "NotFound")
			return false, nil // объект не найден
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// Другая ошибка
		return false, err
	}

	span.SetStatus(codes.Ok, "Found")

	return true, nil
}

func (c *S3Client) Get(ctx context.Context, key string) ([]byte, string, error) {

	ctx, span := c.tracer.Start(ctx, "Get")
	defer span.End()

	result, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, "", err
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, "", err
	}

	contentType := ""
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	span.SetStatus(codes.Ok, "")

	return data, contentType, nil
}

func (c *S3Client) Del(ctx context.Context, key string) error {

	ctx, span := c.tracer.Start(ctx, "Del")
	defer span.End()

	_, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})

	if err != nil {

		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			c.otl.InfoContext(ctx, "S3.Del file has deleted: "+err.Error())
			span.SetStatus(codes.Ok, "HasDeleted")
			return nil
		}
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			c.otl.InfoContext(ctx, "S3.Del file not found: "+err.Error())
			span.SetStatus(codes.Ok, "NotFound")
			return nil
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err

	}

	span.SetStatus(codes.Ok, "")

	return nil
}

func (c *S3Client) GetURL(ctx context.Context, key string) string {

	ctx, span := c.tracer.Start(ctx, "GetURL")
	defer span.End()
	defer span.SetStatus(codes.Ok, "")

	if c.s3Client.Options().BaseEndpoint != nil {
		return fmt.Sprintf("%s/%s/%s", *c.s3Client.Options().BaseEndpoint, c.bucketName, key)
	}

	return fmt.Sprintf("%s/%s/%s", "", c.bucketName, key)
}

func (c *S3Client) Check() string {
	_, err := c.s3Client.HeadBucket(context.Background(), &s3.HeadBucketInput{
		Bucket: aws.String(c.bucketName),
	})
	if err != nil {
		return err.Error()
	}
	return "ok"
}
