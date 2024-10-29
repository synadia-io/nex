package agentcommon

import (
	"fmt"
	"os"
	"strconv"

	"github.com/nats-io/nats.go"
)

const (
	EnvNatsHost = "NEX_NODE_NATS_HOST"
	EnvNatsNkey = "NEX_NODE_NATS_NKEY_SEED"
	EnvNatsPort = "NEX_NODE_NATS_PORT"

	EnvHostServicesHost     = "NEX_HOSTSERVICES_NATS_SERVER"
	EnvHostServicesPort     = "NEX_HOSTSERVICES_NATS_PORT"
	EnvHostServicesJWT      = "NEX_HOSTSERVICES_NATS_USER_JWT"
	EnvHostServicesUserSeed = "NEX_HOSTSERVICES_NATS_USER_SEED"
)

// Creates a connection that allows the agent to communicate with the Nex node itself. This
// connection is used for the comms protocol where the Nex node can request start and stop
// and the agent can register with the Nex node (mandatory).
//
// This is NOT the connection used to expose the host services APIs
func CreateEmbeddedNatsConnection() (*nats.Conn, error) {

	host, exists := os.LookupEnv(EnvNatsHost)
	if !exists {
		host = "0.0.0.0"
	}

	port, exists := os.LookupEnv(EnvNatsPort)
	if !exists {
		port = "4222"
	}

	p, err := strconv.Atoi(port)
	if err != nil {
		p = 4222
	}

	return nats.Connect(fmt.Sprintf("nats://%s:%d", host, p),
		nats.Nkey(os.Getenv(EnvNatsNkey), nats.GetDefaultOptions().SignatureCB),
		nats.Name("Nex Node Agent Client (embedded)"))
}

// Creates a host services connection for use by an agent. This connects the agent to resources contained
// within whatever account is indicated by the JWT+seed. This is where resources for host services such as kv buckets,
// object stores, subject spaces, etc, are contained.
//
// It's over this connection that agent-managed workloads will make their RPC invocations to interact with services
// and resources in the target account.
//
// NOTE: While the embedded nats connection is per-agent, the host services connect is _per workload_. This is a
// very important distinction.
func createHostServicesConnection(workloadId string, host string, port int, jwt string, seed string) (*nats.Conn, error) {
	return nats.Connect(fmt.Sprintf("nats://%s:%d", host, port),
		nats.Name(fmt.Sprintf("Nex Host Services Client - %s", workloadId)),
		nats.UserJWTAndSeed(jwt, seed))

}
