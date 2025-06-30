package secretstore

import (
	"errors"

	"github.com/synadia-labs/nex/models"
)

var _ models.SecretStore = (*NoStore)(nil)

type NoStore struct{}

func (s *NoStore) GetSecret(namespace string, secretName string) ([]byte, error) {
	return nil, errors.New("no secret store configured")
}
