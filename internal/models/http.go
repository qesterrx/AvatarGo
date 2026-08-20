package models

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
	MaxSize int64  `json:"max_size,omitempty"`
}

type HealthCheckResponse struct {
	MetaStorage string `json:"meta,omitempty"`
	FileStorage string `json:"file,omitempty"`
	Broker      string `json:"broker,omitempty"`
}
