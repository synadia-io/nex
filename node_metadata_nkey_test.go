package nex_test

// External test package (nex_test), same rationale as
// node_undeploy_state_test.go: this test needs _test.StartNexusWithOptions,
// which imports the root nex package, so an internal (package nex) test file
// would create an import cycle.
//
// This test uses a real per-mint credentials.SigningKeyMinter (via
// nex.WithMinter in extraOpts) instead of the harness's default
// testminter.TestMinter, because TestMinter is a stub that never populates
// NatsConnectionData.NatsUserNkey -- it would make the nkey-validity
// assertions below vacuous. SigningKeyMinter mints a fresh random NATS user
// keypair per call, so nothing here needs to talk to a real NATS
// operator/account hierarchy: the test never authenticates a connection with
// the minted JWT, it only inspects the connection-data struct and the
// persisted KV record.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"

	nex "github.com/synadia-io/nex"
	"github.com/synadia-io/nex/_test"
	"github.com/synadia-io/nex/internal/credentials"
	"github.com/synadia-io/nex/models"
)

// newTestSigningMinter builds a credentials.SigningKeyMinter backed by a
// freshly generated NATS account keypair, suitable for a test NATS server
// that does no operator/account JWT resolution (the minted creds are never
// used to authenticate a connection in this test -- only inspected).
func newTestSigningMinter(t testing.TB, natsURL, nodeID string) *credentials.SigningKeyMinter {
	t.Helper()

	accountKp, err := nkeys.CreateAccount()
	be.NilErr(t, err)
	accountSeed, err := accountKp.Seed()
	be.NilErr(t, err)
	accountPub, err := accountKp.PublicKey()
	be.NilErr(t, err)

	return &credentials.SigningKeyMinter{
		NodeId:         nodeID,
		Nexus:          "testnexus",
		NatsServers:    []string{natsURL},
		RootAccountKey: accountPub,
		SigningSeed:    string(accountSeed),
	}
}

// TestNodeDeployPersistsMintedNkey pins the fix for the missing credential
// fencing prerequisite described in the nex-workload-verbs plan (Task 4):
// the node mints a distinct NATS user credential per workload but discarded
// the public user nkey instead of persisting it, making future revocation
// unbuildable. After ADEPLOY, the node's stored KV record must carry the
// minted workload's public nkey under Metadata["nex_minted_nkey"], and that
// value must be a well-formed NATS user public key.
func TestNodeDeployPersistsMintedNkey(t *testing.T) {
	workDir := t.TempDir()
	s := _test.StartNatsServer(t, workDir)
	defer s.Shutdown()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	nc, err := nats.Connect(s.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	minter := newTestSigningMinter(t, s.ClientURL(), _test.Node1Pub)

	// state=true: exercises the real NATS-KV-backed state wiring, same as
	// node_undeploy_state_test.go. extraOpts overrides the harness's default
	// stub minter with the real one built above (options apply in order, so
	// this wins).
	nexNodes := _test.StartNexusWithOptions(t, ctx, s.ClientURL(), 1, true, []nex.NexNodeOption{nex.WithMinter(minter)})
	be.Equal(t, 1, len(nexNodes))

	// AUCTION
	auctionReq := models.AuctionRequest{
		AgentType: "inmem",
		AuctionId: nuid.New().Next(),
	}
	auctionReqB, err := json.Marshal(auctionReq)
	be.NilErr(t, err)

	auctionRespRaw, err := nc.Request(models.AuctionRequestSubject(models.SystemNamespace), auctionReqB, time.Second*10)
	be.NilErr(t, err)

	auctionResp := models.AuctionResponse{}
	be.NilErr(t, json.Unmarshal(auctionRespRaw.Data, &auctionResp))
	be.Nonzero(t, auctionResp.BidderId)

	// ADEPLOY
	startWorkloadReq := models.StartWorkloadRequest{
		Description:       "test",
		Name:              "test",
		Namespace:         models.SystemNamespace,
		RunRequest:        "{}",
		WorkloadLifecycle: models.WorkloadLifecycleService,
		WorkloadType:      "inmem",
	}
	startWorkloadReqB, err := json.Marshal(startWorkloadReq)
	be.NilErr(t, err)

	startWorkloadRespRaw, err := nc.Request(models.AuctionDeployRequestSubject(models.SystemNamespace, auctionResp.BidderId), startWorkloadReqB, time.Second*5)
	be.NilErr(t, err)

	startWorkloadResp := models.StartWorkloadResponse{}
	be.NilErr(t, json.Unmarshal(startWorkloadRespRaw.Data, &startWorkloadResp))
	be.Nonzero(t, startWorkloadResp.Id)

	// Read the state KV bucket directly (bucket "nex-<node pubkey>", key
	// "<workload_type>_<workload_id>" -- see internal/state/nats_kv.go) and
	// confirm the persisted record's Metadata carries a valid nkey.
	js, err := jetstream.New(nc)
	be.NilErr(t, err)
	bucketName := fmt.Sprintf("nex-%s", _test.Node1Pub)
	kv, err := js.KeyValue(ctx, bucketName)
	be.NilErr(t, err)

	key := fmt.Sprintf("inmem_%s", startWorkloadResp.Id)
	entry, err := kv.Get(ctx, key)
	be.NilErr(t, err)

	var stored models.StartWorkloadRequest
	be.NilErr(t, json.Unmarshal(entry.Value(), &stored))

	be.True(t, stored.Metadata != nil)
	rawNkey, ok := stored.Metadata["nex_minted_nkey"]
	be.True(t, ok)
	deployNkey, ok := rawNkey.(string)
	be.True(t, ok)
	be.True(t, nkeys.IsValidPublicUserKey(deployNkey))

	// --- resume-on-registration path ---
	//
	// Re-registering an agent of the same RegisterType ("inmem") makes the
	// node walk its persisted state for that agent type, re-mint credentials
	// per record, and (after the fix) re-persist the refreshed nkey. Drive
	// this the same way the real agent SDK does: a raw request to the
	// node's $NEX.SVC.<nodeid>.agent.REGISTER.<agentid> subject (the agentID
	// in that subject is caller-chosen and unrelated to workload identity --
	// GetStateByAgent keys off RegisterType, not agent ID -- so a synthetic
	// second registration is sufficient to exercise the same code path a
	// real crash-restarted nexlet would hit).
	resumeXkp, err := nkeys.CreateCurveKeys()
	be.NilErr(t, err)
	resumeXkPub, err := resumeXkp.PublicKey()
	be.NilErr(t, err)

	registerReq := models.RegisterAgentRequest{
		Description:         "resume test agent",
		Name:                "inmem-resume",
		PublicXkey:          resumeXkPub,
		RegisterType:        "inmem",
		StartRequestSchema:  "{}",
		SupportedLifecycles: []models.WorkloadLifecycle{models.WorkloadLifecycleService},
		Version:             "0.0.0",
	}
	registerReqB, err := json.Marshal(registerReq)
	be.NilErr(t, err)

	resumeAgentID := nuid.New().Next()
	registerRespRaw, err := nc.Request(models.AgentAPIRegisterRequestSubject(resumeAgentID, _test.Node1Pub), registerReqB, time.Second*5)
	be.NilErr(t, err)

	registerResp := models.RegisterAgentResponse{}
	be.NilErr(t, json.Unmarshal(registerRespRaw.Data, &registerResp))
	be.True(t, registerResp.Success)

	existing, ok := registerResp.ExistingState[startWorkloadResp.Id]
	be.True(t, ok)
	resumeNkey := existing.WorkloadCreds.NatsUserNkey
	be.True(t, nkeys.IsValidPublicUserKey(resumeNkey))
	// The re-mint produces a fresh keypair (credentials.SigningKeyMinter
	// calls nkeys.CreateUser() on every Mint), so the resumed nkey must
	// differ from the original -- this is the "tracks the live credential"
	// property Task 4 requires.
	be.Unequal(t, deployNkey, resumeNkey)

	entry2, err := kv.Get(ctx, key)
	be.NilErr(t, err)
	var stored2 models.StartWorkloadRequest
	be.NilErr(t, json.Unmarshal(entry2.Value(), &stored2))

	rawNkey2, ok := stored2.Metadata["nex_minted_nkey"]
	be.True(t, ok)
	storedResumeNkey, ok := rawNkey2.(string)
	be.True(t, ok)
	be.Equal(t, resumeNkey, storedResumeNkey)
}
