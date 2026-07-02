package credentials

import (
	"context"

	"github.com/synadia-io/nex/internal/retry"
	"github.com/synadia-io/nex/models"
)

// WithRetry wraps vendor so every mint is retried per policy. A transient
// vendor failure (e.g. a control-plane-backed minter hiccuping) must not
// permanently skip an agent or fail a request that would succeed moments
// later.
func WithRetry(ctx context.Context, vendor models.CredVendor, policy retry.Policy) models.CredVendor {
	return &retryingVendor{ctx: ctx, inner: vendor, policy: policy}
}

type retryingVendor struct {
	// ctx bounds retries to the owning node's lifetime; the CredVendor
	// methods take no context of their own.
	ctx    context.Context
	inner  models.CredVendor
	policy retry.Policy
}

func (v *retryingVendor) MintRegister(agentId, nodeId string) (*models.NatsConnectionData, error) {
	return retry.Do(v.ctx, v.policy, func() (*models.NatsConnectionData, error) {
		return v.inner.MintRegister(agentId, nodeId)
	})
}

func (v *retryingVendor) Mint(typ models.CredType, namespace, id string) (*models.NatsConnectionData, error) {
	return retry.Do(v.ctx, v.policy, func() (*models.NatsConnectionData, error) {
		return v.inner.Mint(typ, namespace, id)
	})
}
