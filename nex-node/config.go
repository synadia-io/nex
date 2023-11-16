package nexnode

import (
	"encoding/json"
	"os"
	"time"
)

type NodeConfiguration struct {
	KernelPath      string          `json:"kernel_path"`
	RootFsPath      string          `json:"rootfs_path"`
	MachinePoolSize int             `json:"machine_pool_size"`
	CNI             CNIDefinition   `json:"cni"`
	MachineTemplate MachineTemplate `json:"machine_template"`
	RateLimiters    *Limiters       `json:"rate_limiters,omitempty"`
	ValidIssuers    []string        `json:"valid_issuers,omitempty"`
}

type Limiters struct {
	Bandwidth  *TokenBucket `json:"bandwidth,omitempty"`
	Operations *TokenBucket `json:"iops,omitempty"`
}

type CNIDefinition struct {
	NetworkName   string `json:"network_name"`
	InterfaceName string `json:"interface_name"`
}

type MachineTemplate struct {
	VcpuCount  int `json:"vcpu_count"`
	MemSizeMib int `json:"memsize_mib"`
}

type TokenBucket struct {

	// The initial size of a token bucket.
	// Minimum: 0
	OneTimeBurst *int64 `json:"one_time_burst,omitempty"`

	// The amount of milliseconds it takes for the bucket to refill.
	// Required: true
	// Minimum: 0
	RefillTime *int64 `json:"refill_time"`

	// The total number of tokens this bucket can hold.
	// Required: true
	// Minimum: 0
	Size *int64 `json:"size"`
}

func DefaultNodeConfiguration() NodeConfiguration {
	return NodeConfiguration{
		KernelPath:      "vmlinux-5.10",
		RootFsPath:      "rootfs.ext4",
		MachinePoolSize: 1,
		CNI: CNIDefinition{
			NetworkName:   "fcnet",
			InterfaceName: "veth0",
		},
		MachineTemplate: MachineTemplate{
			VcpuCount:  1,
			MemSizeMib: 256,
		},
		RateLimiters: nil,
	}
}

func LoadNodeConfiguration(configFilePath string) (*NodeConfiguration, error) {
	bytes, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}
	var config NodeConfiguration
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

type CliOptions struct {
	// Servers is the list of servers to connect to
	Servers string
	// Creds is nats credentials to authenticate with
	Creds string
	// TlsCert is the TLS Public Certificate
	TlsCert string
	// TlsKey is the TLS Private Key
	TlsKey string
	// TlsCA is the certificate authority to verify the connection with
	TlsCA string
	// Timeout is how long to wait for operations
	Timeout time.Duration
	// ConnectionName is the name to use for the underlying NATS connection
	ConnectionName string
	// Username is the username or token to connect with
	Username string
	// Password is the password to connect with
	Password string
	// Nkey is the file holding a nkey to connect with
	Nkey string
	// TlsFirst configures theTLSHandshakeFirst behavior in nats.go
	TlsFirst bool
	// Path to the file containing virtual machine management settings
	MachineConfigFile string
}
