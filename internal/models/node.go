package models

// A set of rate limiters. These fields are identical to those in firecracker rate limiter configuration
type Limiters struct {
	Bandwidth  *TokenBucket `json:"bandwidth,omitempty"`
	Operations *TokenBucket `json:"iops,omitempty"`
}

// Defines a reference to the CNI network name, which is defined and configured in a {network}.conflist file, as per
// CNI convention
type CNIDefinition struct {
	BinPath       []string `json:"bin_path"`
	InterfaceName *string  `json:"interface_name"`
	NetworkName   *string  `json:"network_name"`
	Subnet        *string  `json:"subnet"`
}

// Defines the CPU and memory usage of a machine to be configured when it is added to the pool
type MachineTemplate struct {
	VcpuCount  *int `json:"vcpu_count"`
	MemSizeMib *int `json:"memsize_mib"`
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
