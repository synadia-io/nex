package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/nats-io/nkeys"
)

func TestCLISimple(t *testing.T) {
	nex := new(NexCLI)

	parser := kong.Must(nex, kong.Vars(map[string]string{"versionOnly": "testing"}))
	kp, err := nkeys.CreatePair(nkeys.PrefixByteServer)
	if err != nil {
		t.Fatal(err)
	}
	pubKey, err := kp.PublicKey()
	if err != nil {
		t.Fatal(err)
	}

	_, err = parser.Parse([]string{"node", "info", pubKey})
	if err != nil {
		t.Fatal(err)
	}

	if nex.Node.Info.NodeID != pubKey {
		t.Fatalf("Expected %s, got %s", pubKey, nex.Node.Info.NodeID)
	}
}

func TestCLIWithConfig(t *testing.T) {
	config := `{
    "namespace": "derp",
    "nats_servers": ["nats://derp"],
    "natsServers": ["nats://foo"],
    "node": {
      "info": {
        "nodeID": "test"
      }
    }
  }`
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "nex.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(config)
	defer f.Close()

	nex := new(NexCLI)
	parser := kong.Must(nex, kong.Vars(map[string]string{"versionOnly": "testing"}), kong.Configuration(kong.JSON, f.Name()))
	parser.LoadConfig(f.Name())

	_, err = parser.Parse([]string{"node", "up", "--config", f.Name()})
	if err != nil {
		t.Fatal(err)
	}

	jsonData, _ := json.MarshalIndent(nex, "", "  ")
	fmt.Println(string(jsonData))

	if string(nex.Globals.Config) != f.Name() {
		t.Fatal("Expected config to be loaded")
	}

	if nex.Globals.Namespace != "derp" {
		t.Fatalf("Expected nats servers to be %v, got %v", "derp", nex.Globals.Namespace)
	}

	if nex.Node.Info.NodeID != "test" {
		t.Fatalf("Expected node_id to be %v, got %v", "test", nex.Node.Info.NodeID)
	}
}
