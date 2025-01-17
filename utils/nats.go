package utils

import (
	"fmt"
	"github.com/nats-io/nats.go"
	"os"
	"strings"
)

func NatsFromEnv(prefix string) (*nats.Conn, error) {
	opts := []nats.Option{}

	if !strings.HasSuffix(prefix, "_") {
		prefix = fmt.Sprintf("%s_", prefix)
	}

	// These ENV vars are passed to the agent by
	// the Nex node
	natsURL, found := os.LookupEnv(fmt.Sprintf("%sNATS_URL", prefix))
	if !found {
		natsURL = nats.DefaultURL
	}

	natsNkey, found := os.LookupEnv(fmt.Sprintf("%sNATS_NKEY", prefix))
	if found {
		opts = append(opts, nats.Nkey(natsNkey, nats.GetDefaultOptions().SignatureCB))
	}

	return nats.Connect(natsURL, opts...)
}
