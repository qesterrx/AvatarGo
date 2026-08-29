package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

type Avatar struct {
	ID               string        `json:"id" db:"id"`
	UserID           string        `json:"user_id" db:"user_id"`
	FileName         string        `json:"file_name" db:"file_name"`
	MimeType         string        `json:"mime_type" db:"mime_type"`
	SizeBytes        int64         `json:"size_bytes" db:"size_bytes"`
	S3Key            string        `json:"s3_key" db:"s3_key"`
	URL              string        `json:"url"`
	ThumbnailS3Keys  ThumbnailKeys `json:"thumbnail_s3_keys" db:"thumbnail_s3_keys"`
	UploadStatus     string        `json:"upload_status" db:"upload_status"`
	ProcessingStatus string        `json:"processing_status" db:"processing_status"`
	CreatedAt        time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at" db:"updated_at"`
	DeletedAt        *time.Time    `json:"deleted_at,omitempty" db:"deleted_at"`
}

type ThumbnailKeys map[string]string // "100x100" -> "s3/key/100x100.jpg"

// Для чтения JSONB
func (t ThumbnailKeys) Value() (driver.Value, error) {
	if t == nil {
		return nil, nil
	}
	return json.Marshal(t)
}

// Для чтения JSONB
func (t *ThumbnailKeys) Scan(value interface{}) error {
	if value == nil {
		*t = make(ThumbnailKeys)
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return nil
	}

	return json.Unmarshal(bytes, t)
}

type AvatarMetadata struct {
	ID         string      `json:"id"`
	UserID     string      `json:"user_id"`
	FileName   string      `json:"file_name"`
	MimeType   string      `json:"mime_type"`
	Size       int64       `json:"size"`
	Dimensions Dimensions  `json:"dimensions"`
	Thumbnails []Thumbnail `json:"thumbnails"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

type Dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Thumbnail struct {
	Size string `json:"size"`
	URL  string `json:"url"`
}
