package controlapi

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

func CreateWorkloadJwt(filebytes []byte, name string, issuer nkeys.KeyPair) (string, error) {
	hasher := sha256.New()
	_, err := hasher.Write(filebytes)
	if err != nil {
		return "", err
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	genericClaims := jwt.NewGenericClaims(name)
	genericClaims.Data["hash"] = hash

	return genericClaims.Encode(issuer)
}

func VerifyWorkloadJwt(token string, name string, expectedHash string) (*jwt.ValidationResults, error) {
	claims, err := jwt.DecodeGeneric(token)
	if err != nil {
		return nil, err
	}
	var vr jwt.ValidationResults
	claims.Validate(&vr)
	if claims.Data["hash"] != expectedHash {
		vr.Issues = append(vr.Issues, &jwt.ValidationIssue{
			Description: "Checksum in claims does not match expected hash",
			Blocking:    true,
			TimeCheck:   false,
		})
	}
	if claims.Subject != name {
		vr.Issues = append(vr.Issues, &jwt.ValidationIssue{
			Description: "Workload name in claims does not match expected name",
			Blocking:    true,
			TimeCheck:   false,
		})
	}
	return &vr, nil
}
