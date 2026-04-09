package credentials

import (
	"slices"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-io/nex/models"
)

// TestWorkloadClaims_LogScopedToNamespace locks in the invariant that a
// workload's NATS sub permissions are scoped to its owning namespace. This
// is the counterpart to the handlers.go handleAuctionDeploy fix that mints
// credentials against req.Namespace (the workload's real namespace) rather
// than the subject namespace. Without this invariant, a cloned workload
// would receive log permissions for the caller's namespace and be unable to
// stream logs into its own.
func TestWorkloadClaims_LogScopedToNamespace(t *testing.T) {
	perms := WorkloadClaims("default", "wid123")

	expectedLogSub := models.LogAPIPrefix("default") + ".>"
	be.True(t, slices.Contains(perms.Sub.Allow, expectedLogSub))

	// Guard against the specific regression we just fixed: the system
	// namespace must not appear in a default-namespace workload's sub
	// allow list.
	systemLogSub := models.LogAPIPrefix(models.SystemNamespace) + ".>"
	be.False(t, slices.Contains(perms.Sub.Allow, systemLogSub))
}
