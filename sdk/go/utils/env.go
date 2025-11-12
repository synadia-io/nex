package utils

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/synadia-io/nex/models"

	"github.com/nats-io/nkeys"
)

func NatsConnDataFromEnv() (*models.NatsConnectionData, error) {
	natsConnData := new(models.NatsConnectionData)
	natsServers, ok := os.LookupEnv("NEX_AGENT_NATS_SERVERS")
	if !ok {
		return nil, errors.New("NEX_AGENT_NATS_SERVERS is required")
	} else {
		natsConnData.NatsServers = strings.Split(natsServers, ",")
	}

	if b64Jwt, ok := os.LookupEnv("NEX_AGENT_NATS_B64_JWT"); ok {
		if jwt, err := base64.StdEncoding.DecodeString(b64Jwt); err == nil {
			natsConnData.NatsUserJwt = string(jwt)
		}
	}

	natsConnData.NatsUserSeed = os.Getenv("NEX_AGENT_NATS_USER_SEED")
	natsConnData.NatsUserName = os.Getenv("NEX_AGENT_NATS_USER")
	natsConnData.NatsUserPassword = os.Getenv("NEX_AGENT_NATS_PASSWORD")
	natsConnData.NatsUserNkey = os.Getenv("NEX_AGENT_NATS_USER_NKEY")

	return natsConnData, nil
}

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
