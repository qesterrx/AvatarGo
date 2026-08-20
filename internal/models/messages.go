package models

type AvatarUploadEvent struct {
	AvatarID string `json:"avatar_id"`
	UserID   string `json:"user_id"`
	S3Key    string `json:"s3_key"`
}

type AvatarDeleteEvent struct {
	AvatarID string   `json:"avatar_id"`
	S3Keys   []string `json:"s3_keys"`
}

type AvatarUploadEventHandle struct {
	Event *AvatarUploadEvent
	Ack   func()
	Nack  func()
}

type AvatarDeleteEventHandle struct {
	Event *AvatarDeleteEvent
	Ack   func()
	Nack  func()
}
