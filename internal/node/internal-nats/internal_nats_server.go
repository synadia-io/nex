package internalnats

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"
)

const (
	workloadCacheBucketName = "NEXCACHE"
	workloadCacheFileKey    = "workload"
)

type InternalNatsServer struct {
	ncInternal       *nats.Conn
	serverConfigData internalServerData
	log              *slog.Logger
	lastOpts         *server.Options
	server           *server.Server
}

func NewInternalNatsServer(log *slog.Logger) (*InternalNatsServer, error) {
	opts := &server.Options{
		JetStream: true,
		StoreDir:  "pnats",
		Port:      -1,
		// Uncomment this when you want internal NATS server logs to be suppressed
		// NoLog: true
	}

	data := internalServerData{
		Users:             make([]userData, 0),
		NexHostUserPublic: "",
		NexHostUserSeed:   "",
	}
	hostUser, _ := nkeys.CreateUser()
	hostPub, _ := hostUser.PublicKey()
	hostSeed, _ := hostUser.Seed()

	data.NexHostUserPublic = hostPub
	data.NexHostUserSeed = string(hostSeed)

	opts, err := updateNatsOptions(opts, log, data)

	s, err := server.NewServer(opts)
	if err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return nil, err
	}

	s.ConfigureLogger()
	if err := server.Run(s); err != nil {
		server.PrintAndDie(err.Error())
		return nil, err
	}

	ncInternal, err := nats.Connect(s.ClientURL(), nats.Nkey(data.NexHostUserPublic, func(b []byte) ([]byte, error) {
		return hostUser.Sign(b)
	}))
	if err != nil {
		return nil, err
	}

	opts.Port = getPort(s.ClientURL())

	internalServer := InternalNatsServer{
		ncInternal:       ncInternal,
		serverConfigData: data,
		log:              log,
		lastOpts:         opts,
		server:           s,
	}

	go s.WaitForShutdown()

	return &internalServer, nil
}

// Returns a user keypair that can be used to log into the internal server
// as the given workload
func (s *InternalNatsServer) CreateNewWorkloadUser(workloadID string) (nkeys.KeyPair, error) {
	userPair, err := nkeys.CreateUser()
	if err != nil {
		return nil, err
	}
	pk, _ := userPair.PublicKey()
	seed, _ := userPair.Seed()
	s.serverConfigData.Users = append(s.serverConfigData.Users, userData{
		WorkloadID: workloadID,
		NkeySeed:   string(seed),
		NkeyPublic: pk,
	})

	opts := &server.Options{
		JetStream:  true,
		StoreDir:   "pnats",
		Port:       s.lastOpts.Port,
		ConfigFile: s.lastOpts.ConfigFile,
	}

	updated, err := updateNatsOptions(opts, s.log, s.serverConfigData)
	if err != nil {
		return nil, err
	}
	err = s.server.ReloadOptions(updated)
	if err != nil {
		return nil, err
	}
	//s.lastOpts = updated

	return userPair, nil
}

func (s *InternalNatsServer) Connection() *nats.Conn {
	return s.ncInternal
}

func (s *InternalNatsServer) Shutdown() {
	s.server.Shutdown()
}

func (s *InternalNatsServer) WaitForShutdown() {
	s.server.WaitForShutdown()
}

func (s *InternalNatsServer) StoreFileForWorkload(workloadId string, bytes []byte) error {
	ctx, cancelF := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelF()

	ud, err := s.FindWorkload(workloadId)
	if err != nil {
		return err
	}
	nc, err := s.ConnectionForUser(ud)
	if err != nil {
		return err
	}
	bucket, err := ensureWorkloadObjectStore(nc)
	if err != nil {
		return err
	}

	_, err = bucket.PutBytes(ctx, workloadCacheFileKey, bytes)
	return err
}

func (s *InternalNatsServer) ConnectionForUser(ud *userData) (*nats.Conn, error) {
	pair, err := nkeys.FromSeed([]byte(ud.NkeySeed))
	if err != nil {
		return nil, err
	}
	nc, err := nats.Connect(s.server.ClientURL(), nats.Nkey(ud.NkeyPublic, func(b []byte) ([]byte, error) {
		return pair.Sign(b)
	}))
	if err != nil {
		return nil, err
	}
	return nc, nil
}

func (s *InternalNatsServer) FindWorkload(workloadId string) (*userData, error) {
	for _, v := range s.serverConfigData.Users {
		if v.WorkloadID == workloadId {
			return &v, nil
		}
	}

	return nil, errors.New("No such workload")
}

func ensureWorkloadObjectStore(nc *nats.Conn) (jetstream.ObjectStore, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelF()

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	var bucket jetstream.ObjectStore

	bucket, err = js.ObjectStore(ctx, workloadCacheBucketName)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			bucket, err = js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
				Bucket:      workloadCacheBucketName,
				Description: "Cache for workload images to be executed by agent",
				Storage:     jetstream.MemoryStorage,
			})
		} else {
			return nil, err
		}
	}
	return bucket, nil
}

func updateNatsOptions(opts *server.Options, log *slog.Logger, data internalServerData) (*server.Options, error) {
	bytes, err := GenerateFile(log, data)
	if err != nil {
		log.Error("Failed to generate internal nats server config file", slog.Any("error", err))
		return nil, err
	}

	var f *os.File
	if len(opts.ConfigFile) == 0 {
		f, err = os.CreateTemp("", "internalconf")
	} else {
		f, err = os.Create(opts.ConfigFile)
	}
	defer os.Remove(f.Name()) // clean up

	if _, err := f.Write(bytes); err != nil {
		log.Error("Failed to write internal nats server config file", slog.Any("error", err))
		return nil, err
	}

	err = opts.ProcessConfigFile(f.Name())
	if err != nil {
		log.Error("Failed to process configuration file", slog.Any("error", err))
		return nil, err
	}

	return opts, nil
}

func getPort(clientUrl string) int {
	u, err := url.Parse(clientUrl)
	if err != nil {
		return -1
	}
	res, err := strconv.Atoi(u.Port())
	if err != nil {
		return -1
	}
	return res
}
