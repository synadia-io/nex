package controlapi

import (
	"fmt"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type StopRequest struct {
	WorkloadId  string `json:"workload_id"`
	WorkloadJwt string `json:"workload_jwt"`
	TargetNode  string `json:"target_node"`
}

type StopResponse struct {
	Stopped bool   `json:"stopped"`
	ID      string `json:"id"`
	Issuer  string `json:"issuer"`
	Name    string `json:"name"`
}

func NewStopRequest(workloadId string, name string, targetNode string, issuer nkeys.KeyPair) (*StopRequest, error) {
	claims := jwt.NewGenericClaims(name)
	jwtText, err := claims.Encode(issuer)
	if err != nil {
		return nil, err
	}

	return &StopRequest{
		WorkloadId:  workloadId,
		TargetNode:  targetNode,
		WorkloadJwt: jwtText,
	}, nil
}

func (request *StopRequest) Validate(originalClaims *jwt.GenericClaims) error {
	claims, err := jwt.DecodeGeneric(request.WorkloadJwt)
	if err != nil {
		return fmt.Errorf("could not decode workload JWT: %s", err)
	}
	if claims.ID == originalClaims.ID ||
		claims.IssuedAt == originalClaims.IssuedAt {
		return fmt.Errorf("stop claims appear to be cloned or captured from the original start claims. Rejecting for security reasons")
	}
	if claims.Subject != originalClaims.Subject {
		return fmt.Errorf("stop claims subject does not match original start claims subject")
	}
	if claims.Issuer != originalClaims.Issuer {
		return fmt.Errorf("the only entity allowed to terminate a workload is the issuer that originally started it")
	}

	return nil
}
