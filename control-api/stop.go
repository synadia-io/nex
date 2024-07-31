package controlapi

import (
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type StopRequest struct {
	NodeID      string `json:"node_id"`
	WorkloadID  string `json:"workload_id"`
	WorkloadJWT string `json:"workload_jwt"`
}

type StopResponse struct {
	ID      string `json:"id"`
	Issuer  string `json:"issuer"`
	Stopped bool   `json:"stopped"`
}

func NewStopRequest(nodeID, workloadID string, issuer nkeys.KeyPair) (*StopRequest, error) {
	claims := jwt.NewGenericClaims(workloadID)
	jwtText, err := claims.Encode(issuer)
	if err != nil {
		return nil, err
	}

	return &StopRequest{
		NodeID:      nodeID,
		WorkloadID:  workloadID,
		WorkloadJWT: jwtText,
	}, nil
}

func (request *StopRequest) Validate(originalClaims *jwt.GenericClaims) error {
	claims, err := jwt.DecodeGeneric(request.WorkloadJWT)
	if err != nil {
		return fmt.Errorf("could not decode workload JWT: %s", err)
	}

	if claims.ID == originalClaims.ID ||
		claims.IssuedAt == originalClaims.IssuedAt {
		return fmt.Errorf("stop claims appear to be cloned or captured from the original start claims. Rejecting for security reasons")
	}

	// FIXME-- we don't have the original deploy request id... this used to compare using the workload names :/
	// if claims.Subject != originalClaims.Subject {
	// 	return fmt.Errorf("stop claims subject does not match original start claims subject")
	// }

	if claims.Issuer != originalClaims.Issuer {
		return fmt.Errorf("the only entity allowed to terminate a workload is the issuer that originally started it")
	}

	return nil
}
