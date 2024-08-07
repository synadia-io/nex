package internalnats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"
)

const (
	defaultInternalNatsConfigFile             = "internalconf"
	defaultInternalNatsConnectionDrainTimeout = time.Millisecond * 5000
	workloadCacheBucketName                   = "NEXCACHE"
)

type InternalNatsServer struct {
	ncInternal       *nats.Conn
	log              *slog.Logger
	lastOpts         *server.Options
	mutex            *sync.Mutex
	server           *server.Server
	serverConfigData *internalServerData
}

func NewInternalNatsServer(log *slog.Logger, storeDir string, debug, trace bool) (*InternalNatsServer, error) {
	opts := &server.Options{
		JetStream: true,
		StoreDir:  storeDir,
		Port:      -1,
		Debug:     debug,
		Trace:     trace,
	}

	hostUser, _ := nkeys.CreateUser()
	hostPub, _ := hostUser.PublicKey()
	hostSeed, _ := hostUser.Seed()

	data := &internalServerData{
		Credentials:       map[string]*credentials{},
		Connections:       map[string]*nats.Conn{},
		NexHostUserPublic: hostPub,
		NexHostUserSeed:   string(hostSeed),
	}

	opts, err := updateNatsOptions(opts, log, data)
	if err != nil {
		return nil, err
	}

	s, err := server.NewServer(opts)
	if err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return nil, err
	}

	if debug || trace {
		s.ConfigureLogger()
	}

	if err := server.Run(s); err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
		return nil, err
	}

	// This connection uses the `nexhost` account, specifically provisioned for the node
	ncInternal, err := nats.Connect(s.ClientURL(),
		nats.DrainTimeout(defaultInternalNatsConnectionDrainTimeout),
		nats.Nkey(
			data.NexHostUserPublic,
			func(b []byte) ([]byte, error) {
				log.Debug("Attempting to sign NATS server nonce for internal host connection", slog.String("public_key", data.NexHostUserPublic))
				return hostUser.Sign(b)
			},
		),
	)
	if err != nil {
		server.PrintAndDie(err.Error())
		return nil, err
	}

	opts.Port = getPort(s.ClientURL())

	internalServer := InternalNatsServer{
		ncInternal:       ncInternal,
		log:              log,
		lastOpts:         opts,
		mutex:            &sync.Mutex{},
		server:           s,
		serverConfigData: data,
	}

	return &internalServer, nil
}

func (s *InternalNatsServer) Port() int {
	return s.lastOpts.Port
}

func (s *InternalNatsServer) Subsz(opts *server.SubszOptions) (*server.Subsz, error) {
	return s.server.Subsz(opts)
}

// Returns a user keypair that can be used to log into the internal server
func (s *InternalNatsServer) CreateCredentials(id string) (nkeys.KeyPair, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	kp, err := nkeys.CreateUser()
	if err != nil {
		s.log.Error("Failed to create nkey user", slog.Any("error", err))
		return nil, err
	}

	pk, _ := kp.PublicKey()
	seed, _ := kp.Seed()

	creds := &credentials{
		NkeySeed:   string(seed),
		NkeyPublic: pk,
		ID:         id,
	}
	s.serverConfigData.Credentials[id] = creds

	updated, err := updateNatsOptions(&server.Options{
		ConfigFile: s.lastOpts.ConfigFile,
		JetStream:  true,
		Port:       s.lastOpts.Port,
		StoreDir:   s.lastOpts.StoreDir,
	}, s.log, s.serverConfigData)
	if err != nil {
		s.log.Error("Failed to update NATS options in internal server", slog.Any("error", err))
		return nil, err
	}

	err = s.server.ReloadOptions(updated)
	if err != nil {
		s.log.Error("Failed to reload NATS internal server options", slog.Any("error", err))
		return nil, err
	}

	nc, err := s.ConnectionWithCredentials(creds)
	if err != nil {
		s.log.Error("Failed to obtain connection for given credentials", slog.Any("error", err))
		return nil, err
	}
	s.serverConfigData.Connections[id] = nc

	_, err = ensureWorkloadObjectStore(nc)
	if err != nil {
		s.log.Error("Failed to create or locate object store in internal NATS server",
			slog.Any("error", err),
		)
		return nil, err
	}

	return kp, nil
}

// Destroy previously-created credentials
func (s *InternalNatsServer) DestroyCredentials(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if nc, ok := s.serverConfigData.Connections[id]; ok {
		_ = nc.Drain()
	}

	delete(s.serverConfigData.Credentials, id)
	delete(s.serverConfigData.Connections, id)

	updated, err := updateNatsOptions(&server.Options{
		ConfigFile: s.lastOpts.ConfigFile,
		JetStream:  true,
		Port:       s.lastOpts.Port,
		StoreDir:   s.lastOpts.StoreDir,
	}, s.log, s.serverConfigData)
	if err != nil {
		s.log.Error("Failed to update NATS options in internal server", slog.Any("error", err))
		return err
	}

	err = s.server.ReloadOptions(updated)
	if err != nil {
		s.log.Error("Failed to reload NATS internal server options", slog.Any("error", err))
		return err
	}

	return nil
}

func (s *InternalNatsServer) ClientURL() string {
	return s.ncInternal.ConnectedUrl()
}

func (s *InternalNatsServer) Connection() *nats.Conn {
	return s.ncInternal
}

func (s *InternalNatsServer) Shutdown() {
	s.server.Shutdown()
	s.server.WaitForShutdown()

	_ = os.Remove(path.Join(os.TempDir(), s.lastOpts.StoreDir))
}

func (s *InternalNatsServer) StoreFileForID(id string, bytes []byte) error {
	ctx, cancelF := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelF()

	creds, err := s.FindCredentials(id)
	if err != nil {
		return err
	}

	nc, err := s.ConnectionWithCredentials(creds)
	if err != nil {
		return err
	}

	bucket, err := ensureWorkloadObjectStore(nc)
	if err != nil {
		return err
	}

	_, err = bucket.PutBytes(ctx, id, bytes)
	return err
}

func (s *InternalNatsServer) ConnectionWithID(id string) (*nats.Conn, error) {
	creds, err := s.FindCredentials(id)
	if err != nil {
		return nil, err
	}

	return s.ConnectionWithCredentials(creds)
}

func (s *InternalNatsServer) ConnectionByID(id string) (*nats.Conn, error) {
	if nc, ok := s.serverConfigData.Connections[id]; ok {
		return nc, nil
	}

	return nil, fmt.Errorf("failed to resolve internal NATS connection for id: %s", id)
}

func (s *InternalNatsServer) ConnectionWithCredentials(creds *credentials) (*nats.Conn, error) {
	pair, err := nkeys.FromSeed([]byte(creds.NkeySeed))
	if err != nil {
		return nil, err
	}

	nc, err := nats.Connect(s.server.ClientURL(),
		nats.DrainTimeout(defaultInternalNatsConnectionDrainTimeout),
		nats.Nkey(creds.NkeyPublic,
			func(b []byte) ([]byte, error) {
				s.log.Debug("Attempting to sign NATS server nonce for internal connection", slog.String("public_key", creds.NkeyPublic))
				return pair.Sign(b)
			},
		),
	)
	if err != nil {
		s.log.Error("Failed to connect to internal NATS server",
			slog.String("public_key", creds.NkeyPublic),
			slog.String("id", creds.ID),
			slog.Any("error", err),
		)
		return nil, err
	}

	return nc, nil
}

func (s *InternalNatsServer) FindCredentials(id string) (*credentials, error) {
	if creds, ok := s.serverConfigData.Credentials[id]; ok {
		return creds, nil
	}

	return nil, errors.New("No such workload")
}

func ensureWorkloadObjectStore(nc *nats.Conn) (jetstream.ObjectStore, error) {
	var err error
	ctx, cancelF := context.WithTimeout(context.Background(), 2*time.Second) // FIXME-- constantize and/or make timeout configurable
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
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return bucket, nil
}

func updateNatsOptions(opts *server.Options, log *slog.Logger, data *internalServerData) (*server.Options, error) {
	bytes, err := GenerateTemplate(log, data)
	if err != nil {
		log.Error("Failed to generate internal nats server config file", slog.Any("error", err))
		return nil, err
	}

	var f *os.File
	if len(opts.ConfigFile) == 0 {
		f, err = os.CreateTemp(os.TempDir(), defaultInternalNatsConfigFile)
	} else {
		f, err = os.Create(opts.ConfigFile)
	}
	if err != nil {
		return nil, err
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
