package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	cfg "github.com/qesterrx/AvatarGo/internal/config"
	"github.com/qesterrx/AvatarGo/internal/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Client struct {
	s3Client   *s3.Client
	bucketName string
}

func NewS3Client(cfg *cfg.S3Config) (*S3Client, error) {

	ctx := context.Background()

	endpoint := cfg.Endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		if cfg.UseSSL {
			endpoint = "https://" + endpoint
		} else {
			endpoint = "http://" + endpoint
		}
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

	return &S3Client{
		s3Client:   s3Client,
		bucketName: cfg.BucketName,
	}, nil
}

func (c *S3Client) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucketName),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(len(data))),
	})

	if err != nil {
		return err
	}

	return nil
}

func (c *S3Client) Get(ctx context.Context, key string) ([]byte, string, error) {
	result, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", err
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := ""
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	return data, contentType, nil
}

func (c *S3Client) Del(ctx context.Context, key string) error {
	_, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})

	if err != nil {

		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			logger.Log.Info("Файл уже удалён или не существует: %s", key)
			return nil
		}
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			logger.Log.Info("Файл не найден: %s", key)
			return nil
		}

		return err

	}

	return nil
}

func (c *S3Client) GetURL(ctx context.Context, key string) string {
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
