package controlapi

import (
	"encoding/base64"
	"encoding/json"

	"github.com/nats-io/nkeys"
)

func (request *RunRequest) DecryptRequestEnvironment(xk nkeys.KeyPair) error {
	data, err := base64.StdEncoding.DecodeString(request.Environment)
	if err != nil {
		return err
	}
	unencrypted, err := xk.Open(data, request.SenderPublicKey)
	if err != nil {
		return err
	}
	var cleanEnv map[string]string
	err = json.Unmarshal(unencrypted, &cleanEnv)
	if err != nil {
		return err
	}
	// "I can't believe I can do this" - Every Rust developer ever.
	request.workloadEnvironment = cleanEnv
	return nil
}
