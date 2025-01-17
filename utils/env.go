package utils

import (
	"encoding/base64"
	"encoding/json"

	"github.com/synadia-labs/nex/models"

	"github.com/nats-io/nkeys"
)

func DecryptEnv(xPair nkeys.KeyPair, eEnv models.EncEnv) (map[string]string, error) {
	eenv, err := base64.StdEncoding.DecodeString(string(eEnv.Base64EncryptedEnv))
	if err != nil {
		return nil, err
	}

	clearEnv, err := xPair.Open(eenv, eEnv.EncryptedBy)
	if err != nil {
		return nil, err
	}

	env := make(map[string]string)
	if len(clearEnv) != 0 {
		err = json.Unmarshal(clearEnv, &env)
		if err != nil {
			return nil, err
		}
	}

	return env, nil
}
