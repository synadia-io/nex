package main

import (
	"fmt"
	"os"

	cli "github.com/ConnectEverything/nex/nex-cli"
	"github.com/choria-io/fisk"
	"github.com/fatih/color"
)

func main() {
	opts := &cli.Options{}
	blue := color.New(color.FgBlue).SprintFunc()
	help := fmt.Sprintf("%s\nNATS Execution Engine CLI Version %s\n", blue(cli.Banner), cli.VERSION)

	ncli := fisk.New("nex", help)
	ncli.Author("Synadia Communications")
	ncli.UsageWriter(os.Stdout)
	ncli.Version(cli.VERSION)
	ncli.HelpFlag.Short('h')
	ncli.WithCheats().CheatCommand.Hidden()

	ncli.Flag("server", "NATS server urls").Short('s').Envar("NATS_URL").PlaceHolder("URL").StringVar(&opts.Servers)
	ncli.Flag("user", "Username or Token").Envar("NATS_USER").PlaceHolder("USER").StringVar(&opts.Username)
	ncli.Flag("password", "Password").Envar("NATS_PASSWORD").PlaceHolder("PASSWORD").StringVar(&opts.Password)
	ncli.Flag("creds", "User credentials file (JWT authentication)").Envar("NATS_CREDS").PlaceHolder("FILE").StringVar(&opts.Creds)
	ncli.Flag("nkey", "User NKEY file for single-key auth").Envar("NATS_NKEY").PlaceHolder("FILE").StringVar(&opts.Nkey)
	ncli.Flag("tlscert", "TLS public certificate file").Envar("NATS_CERT").PlaceHolder("FILE").ExistingFileVar(&opts.TlsCert)
	ncli.Flag("tlskey", "TLS private key file").Envar("NATS_KEY").PlaceHolder("FILE").ExistingFileVar(&opts.TlsKey)
	ncli.Flag("tlsca", "TLS certificate authority chain file").Envar("NATS_CA").PlaceHolder("FILE").ExistingFileVar(&opts.TlsCA)
	ncli.Flag("tlsfirst", "Perform TLS handshake before expecting the server greeting").BoolVar(&opts.TlsFirst)
	ncli.Flag("timeout", "Time to wait on responses from NATS").Default("1s").Envar("NATS_TIMEOUT").PlaceHolder("DURATION").DurationVar(&opts.Timeout)

	nodes := ncli.Command("node", "Interact with execution engine nodes")
	nodes_ls := nodes.Command("ls", "List nodes")
	nodes_ls.Action(cli.ListNodes(opts))

	nodes_info := nodes.Command("info", "Get information for an engine node")
	nodes_info.Arg("id", "Public key of the node you're interested in").Required().String()
	nodes_info.Action(cli.NodeInfo(opts))

	run := ncli.Command("run", "Run a workload on a target node")
	run.Arg("id", "Public key of the node to run the workload").Required().String()
	run.Arg("file", "Path to local file to upload and run").File()
	run.Arg("url", "URL pointing to the file to run").URL()
	run.Action(cli.RunWorkload(opts))

	ncli.MustParseWithUsage(os.Args[1:])
}
