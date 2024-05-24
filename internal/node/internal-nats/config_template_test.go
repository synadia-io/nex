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
		Users: make([]userData, 0),
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

		data.Users = append(data.Users, userData{
			WorkloadID: nuid.Next(),
			NkeySeed:   string(seed),
			NkeyPublic: seedPub,
		})
	}

	bytes, err := GenerateFile(slog.Default(), data)
	if err != nil {
		t.Fatalf("failed to render template: %s", err)
	}

	fmt.Printf("----\n%s\n----\n", string(bytes))

	f, err := os.CreateTemp("", "fooconf")
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

	_, _ = ncHost.Subscribe("*.agentevt.>", func(msg *nats.Msg) {
		tokens := strings.Split(msg.Subject, ".")
		if tokens[0] == data.Users[0].WorkloadID && tokens[2] == "my_event" {
			eventWg.Done()
		}
	})

	_, _ = ncHost.Subscribe("agentint.>", func(msg *nats.Msg) {
		tokens := strings.Split(msg.Subject, ".")
		fmt.Printf("-- Replying to %s\n", msg.Subject)
		if tokens[1] == data.Users[0].WorkloadID {
			_ = msg.Respond([]byte{42, 42, 42})
		}
	})

	ncUser1, err := nats.Connect(s.ClientURL(), nats.Nkey(data.Users[0].NkeyPublic, func(b []byte) ([]byte, error) {
		priv, _ := nkeys.FromSeed([]byte(data.Users[0].NkeySeed))
		return priv.Sign(b)
	}))

	// host account should be able to see all of these
	_ = ncUser1.Publish("agentevt.my_event", []byte{1, 2, 3})
	eventWg.Wait()

	res, err := ncUser1.Request("agentint.my.service", []byte{1, 1, 1}, 1*time.Second)
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

	go s.WaitForShutdown()

}

func startNatsServer(t *testing.T) (func(), *nats.Conn) {
	t.Helper()
	opts := &server.Options{
		JetStream: true,
		Port:      -1,
	}
	s, err := server.NewServer(opts)
	if err != nil {
		server.PrintAndDie("nats-server: " + err.Error())
	}
	s.ConfigureLogger()
	if err := server.Run(s); err != nil {
		server.PrintAndDie(err.Error())
	}

	go s.WaitForShutdown()
	nc, _ := nats.Connect(s.ClientURL())
	return s.Shutdown, nc
}
