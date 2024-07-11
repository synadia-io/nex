package internalnats

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nuid"
)

func TestTemplateGenerator(t *testing.T) {

	data := internalServerData{
		Credentials: map[string]*credentials{},
	}

	hostUser, _ := nkeys.CreateUser()
	hostPub, _ := hostUser.PublicKey()
	hostSeed, _ := hostUser.Seed()

	data.NexHostUserPublic = hostPub
	data.NexHostUserSeed = string(hostSeed)

	for i := 0; i < 10; i++ {
		userSeed, _ := nkeys.CreateUser()
		seed, _ := userSeed.Seed()
		seedPub, _ := userSeed.PublicKey()

		id := nuid.Next()

		data.Credentials[id] = &credentials{
			NkeySeed:   string(seed),
			NkeyPublic: seedPub,
			ID:         id,
		}
	}

	bytes, err := GenerateTemplate(slog.Default(), &data)
	if err != nil {
		t.Fatalf("failed to render template: %s", err)
	}

	fmt.Printf("----\n%s\n----\n", string(bytes))

	f, err := os.CreateTemp("", "fooconf")
	if err != nil {
		t.Fatalf("Failed to create temp file: %s", err)
	}
	defer os.Remove(f.Name()) // clean up
	if _, err := f.Write(bytes); err != nil {
		t.Fatalf("%s", err)
	}

	opts := &server.Options{
		JetStream: true,
		StoreDir:  "pnats",
		Port:      -1,
	}
	err = opts.ProcessConfigFile(f.Name())
	if err != nil {
		t.Fatalf("failed to process configuration file: %s", err)
	}
	s, err := server.NewServer(opts)
	if err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
	}
	s.ConfigureLogger()
	if err := server.Run(s); err != nil {
		server.PrintAndDie(err.Error())
	}
	ncHost, err := nats.Connect(s.ClientURL(), nats.Nkey(data.NexHostUserPublic, func(b []byte) ([]byte, error) {
		return hostUser.Sign(b)
	}))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%+v\n", ncHost.Servers())

	var eventWg sync.WaitGroup
	eventWg.Add(1)

	var id string
	for k := range data.Credentials {
		id = k
		break
	}

	_, _ = ncHost.Subscribe("*.agentevt.>", func(msg *nats.Msg) {
		tokens := strings.Split(msg.Subject, ".")
		if tokens[0] == data.Credentials[id].ID && tokens[2] == "my_event" {
			eventWg.Done()
		}
	})

	_, _ = ncHost.Subscribe("hostint.>", func(msg *nats.Msg) {
		tokens := strings.Split(msg.Subject, ".")
		fmt.Printf("-- Replying to %s\n", msg.Subject)
		if tokens[1] == data.Credentials[id].ID {
			_ = msg.Respond([]byte{42, 42, 42})
		}
	})

	ncUser1, err := nats.Connect(s.ClientURL(), nats.Nkey(data.Credentials[id].NkeyPublic, func(b []byte) ([]byte, error) {
		priv, _ := nkeys.FromSeed([]byte(data.Credentials[id].NkeySeed))
		return priv.Sign(b)
	}))
	if err != nil {
		t.Fatalf("Failed to connect as user: %s", err)
	}

	// host account should be able to see all of these
	_ = ncUser1.Publish("agentevt.my_event", []byte{1, 2, 3})
	eventWg.Wait()

	res, err := ncUser1.Request(fmt.Sprintf("hostint.%s.service", id), []byte{1, 1, 1}, 1*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Data, []byte{42, 42, 42}) {
		t.Fatalf("Failed to get reply from exported service: %+v", res.Data)
	}

	_, err = nats.Connect(s.ClientURL())
	if err == nil {
		t.Fatal("Was supposed to fail to connect with unauthorized user but didn't")
	}
}
