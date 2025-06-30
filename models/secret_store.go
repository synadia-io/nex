package models

const (
	NexSecretPrefix = "secret://"
)

type SecretStore interface {
	GetSecret(namespace string, secretID string) ([]byte, error)
}
