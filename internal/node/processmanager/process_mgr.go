package processmanager

import (
	"time"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

const runloopSleepInterval = 100 * time.Millisecond

type ProcessManagerConfig struct {
	MachinePoolSize  int              `default:"1" json:"processmanager_machine_pool_size"`
	NoSandbox        bool             `default:"false" json:"processmanager_no_sandbox"`
	KernelFilepath   string           `placeholder:"./path/to/kernel" json:"processmanager_kernel_filepath"`
	RootFsFilepath   string           `placeholder:"./path/to/rootfs" json:"processmanager_rootfs_filepath"`
	InternalNodeHost string           `default:"192.168.127.1" json:"processmanager_internal_node_host"`
	InternalNodePort int              `default:"-1" json:"processmanager_internal_node_port"`
	PreserveNetwork  bool             `default:"true" json:"processmanager_preserve_network"`
	CNIDefinition    *CNIDefinition   `embed:"" group:"CNI Configuration"`
	MachineTemplate  *MachineTemplate `embed:"" group:"Firecracker Machine Configuration"`
}

// Defines the CPU and memory usage of a machine to be configured when it is added to the pool
type MachineTemplate struct {
	FirecrackerVcpuCount  int `default:"1" json:"machine_template_firecracker_cpu_count"`
	FirecrackerMemSizeMib int `default:"256" json:"machine_template_firecracker_mem_size_mib"`
}

// Defines a reference to the CNI network name, which is defined and configured in a {network}.conflist file, as per
// CNI convention
type CNIDefinition struct {
	CniBinPaths      []string `type:"existingdir" default:"/opt/cni/bin" json:"cni_bin_paths"`
	CniInterfaceName string   `default:"veth0" json:"cni_interface_name"`
	CniNetworkName   string   `default:"fcnet" json:"cni_network_name"`
	CniSubnet        string   `default:"192.168.127.0/24" json:"cni_subnet"`
}

// Information about an agent process without regard to the implementation of the agent process manager
type ProcessInfo struct {
	DeployRequest *agentapi.DeployRequest
	ID            string
	Name          string
	Namespace     string
}

// A process delegate is any struct that wishes to be notified when the configured agent process
// manager has successfully started an agent
type ProcessDelegate interface {
	// Indicates that an agent process with the given id has been started and is ready for workload deployment
	OnProcessStarted(id string)

	// Indicates that an agent process with the given id should exit
	// OnProcessExit(id string) error
}

// A process manager is responsible for stopping and starting a Nex Agent. It is entirely up to
// the implementation of the process manager as to whether or to what degree any kind of sandboxing
// (e.g. firecracker) is employed. Note that agent processes are created asynchronously -before- any
// workloads are deployed to them, so a workload manager can never explicitly tell a process manager
// to create an individual process
type ProcessManager interface {
	// Returns a list of agent processes in an implementation-agnostic format
	ListProcesses() ([]ProcessInfo, error)

	// Lookup a deploy request by id. Returns nil when attempting to lookup an "unprepared" workload
	Lookup(id string) (*agentapi.DeployRequest, error)

	// Associate a deploy request with the given workload id, and perform any
	// just in time initialization of resources if necessary
	PrepareWorkload(id string, request *agentapi.DeployRequest) error

	// Start the process manager and allocate a pool of agents based on an implementation-specific
	// strategy, delegating callbacks to the given delegate
	Start(delegate ProcessDelegate) error

	// Stop the process manager and gracefully shutdown all agents in the pool
	Stop() error

	// Terminate a running agent process with the given ID
	StopProcess(id string) error

	// Notifies the process manager that the node is in lame duck mode, so that the processes
	// can be treated differerently (if applicable)
	EnterLameDuck() error
}
