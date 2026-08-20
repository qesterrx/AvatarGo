package services

import (
	"github.com/qesterrx/AvatarGo/internal/models"
)

func (s *avatarService) HealthCheck() (*models.HealthCheckResponse, error) {
	hck := models.HealthCheckResponse{}

	hck.MetaStorage = s.metaDB.Check()
	hck.FileStorage = s.fileDB.Check()
	hck.Broker = s.asyncQ.Check()

	return &hck, nil
}
