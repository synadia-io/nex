package nexnode

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/cli/globals"
	"github.com/synadia-io/nex/internal/models"
	"github.com/synadia-io/nex/internal/node/observability"
	"github.com/synadia-io/nex/internal/node/processmanager"
)

type UpCmd struct {
	AgentHandshakeTimeoutMillisecond int                  `default:"5000" group:"Nex Node Up Configuration" json:"up_agent_handshake_timeout_ms"`
	DefaultResourceDir               string               `default:"./resources" group:"Nex Node Up Configuration" json:"up_default_resource_dir"`
	FirecrackerBinPath               string               `default:"/usr/local/bin/firecracker" group:"Nex Node Up Configuration" json:"up_firecracker_bin"`
	Tags                             map[string]string    `placeholder:"nex:iscool;..." group:"Nex Node Up Configuration" json:"up_tags"`
	ValidIssuers                     []string             `group:"Nex Node Up Configuration" json:"up_valid_issuers"`
	WorkloadTypes                    []models.NexWorkload `default:"native" enum:"native,function,container,wasm" group:"Nex Node Up Configuration" json:"up_workload_types"`
	FunctionLanguage                 string               `default:"javascript" enum:"javascript" hidden:"" json:"-"`
	NexusName                        string               `default:"nexus" group:"Nex Node Up Configuration" json:"up_nexus_name"`
	PublicNATSServer                 []byte               `placeholder:"./path/to/nats_server.conf" type:"filecontent" help:"Path to nats server config to be used for userland NATS server. JSON format" group:"Nex Node Up Configuration"`

	ProcessManagerConfig *processmanager.ProcessManagerConfig `embed:"" group:"Process Manager Configuration"`
	Limiters             *Limiters                            `embed:"" prefix:"limiter_" group:"Limiter Configuration"`
	HostServicesConfig   *HostServicesConfig                  `embed:"" group:"Host Services Configuration"`
	OtelConfig           *observability.OtelConfig            `embed:"" group:"OpenTelemetry Configuration"`
}

type HostServicesConfig struct {
	NatsUrl      string                   `group:"Host Services Configuration" json:"hostservices_nats_url"`
	NatsUserJwt  string                   `group:"Host Services Configuration" json:"hostservices_nats_user_jwt"`
	NatsUserSeed string                   `group:"Host Services Configuration" json:"hostservices_nats_user_seed"`
	Services     map[string]ServiceConfig `hidden:"" group:"Host Services Configuration" json:"hostservices_services_map"`
}

type ServiceConfig struct {
	Enabled       bool            `group:"Services Configuration" json:"service_enabled"`
	Configuration json.RawMessage `group:"Services Configuration" json:"service_config"`
}

type Limiters struct {
	Bandwidth  *TokenBucket `embed:"" prefix:"bandwidth_" json:"limiters_bandwidth"`
	Operations *TokenBucket `embed:"" prefix:"operations_" json:"limiters_operations"`
}

type TokenBucket struct {
	// The initial size of a token bucket.
	// Minimum: 0
	OneTimeBurst int64 `placeholder:"0" json:"token_bucket_one_time_burst"`

	// The amount of milliseconds it takes for the bucket to refill.
	// Required: true
	// Minimum: 0
	RefillTime int64 `placeholder:"0" json:"token_bucket_refill_time"`

	// The total number of tokens this bucket can hold.
	// Required: true
	// Minimum: 0
	Size int64 `placeholder:"0" json:"token_bucket_size"`
}

func (u UpCmd) Run(ctx context.Context, nc *nats.Conn, logger *slog.Logger, cfg globals.Globals, nCfg NodeOptions) error {
	if cfg.Check {
		return errors.Join(cfg.Table(), u.Table())
	}
	ctx, cancel := context.WithCancel(ctx)
	n, err := NewNode(nc, &cfg, &nCfg, ctx, cancel, logger)
	if err != nil {
		return err
	}

	go n.Start()

	<-ctx.Done()
	return nil
}

func (u *UpCmd) Validate() error {
	if u.Tags == nil {
		u.Tags = make(map[string]string, 0)
	}

	if u.ProcessManagerConfig.RootFsFilepath == "" {
		u.ProcessManagerConfig.RootFsFilepath = filepath.Join(u.DefaultResourceDir, "rootfs.ext4")
	}

	if u.ProcessManagerConfig.KernelFilepath == "" {
		u.ProcessManagerConfig.KernelFilepath = filepath.Join(u.DefaultResourceDir, "vmlinux")
	}

	return nil
}
