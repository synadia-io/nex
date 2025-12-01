package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/carlmjohnson/be"
)

var kongVars = map[string]string{
	"versionOnly":         "testing",
	"defaultResourcePath": ".",
	"adminNamespace":      "system",
}

func TestCLISimple(t *testing.T) {
	nex := NexCLI{}

	parser := kong.Must(&nex,
		kong.Vars(kongVars),
		kong.Bind(&nex.Globals),
	)

	_, err := parser.Parse([]string{"node", "info", TestServerPublicKey})
	be.NilErr(t, err)
	be.Equal(t, TestServerPublicKey, nex.Node.Info.NodeID)
}

func TestCLIWithConfig(t *testing.T) {
	config := `{
    "namespace": "derp",
    "agents": [
      {
        "uri": "nats://uri/config",
        "argv": [],
        "env": {
          "FOO": "BAR"
        }
      },
      {
        "uri": "nats://uri/config2",
        "argv": ["--arg", "foo"],
        "env": {}
      }
    ]
  }`

	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "nex.json"))
	be.NilErr(t, err)

	_, _ = f.WriteString(config)
	defer f.Close()

	nex := NexCLI{}
	parser := kong.Must(&nex,
		kong.Vars(kongVars),
		kong.Configuration(kong.JSON, f.Name()),
		kong.Bind(&nex.Globals),
	)
	_, err = parser.LoadConfig(f.Name())
	be.NilErr(t, err)

	_, err = parser.Parse([]string{"node", "up", "--config", f.Name()})
	be.NilErr(t, err)

	be.Equal(t, f.Name(), string(nex.Globals.Config))
	be.Equal(t, "derp", nex.Globals.Namespace)
	be.Equal(t, 2, len(nex.Node.Up.Agents))
}
