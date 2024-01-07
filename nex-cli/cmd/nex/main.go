package main

import (
	"fmt"
	"os"

	cli "github.com/ConnectEverything/nex/nex-cli"
	"github.com/choria-io/fisk"
	"github.com/fatih/color"
)

func main() {
	blue := color.New(color.FgBlue).SprintFunc()
	help := fmt.Sprintf("%s\nNATS Execution Engine CLI Version %s\n", blue(cli.Banner), cli.VERSION)

	ncli := fisk.New("nex", help)
	ncli.Author("Synadia Communications")
	ncli.UsageWriter(os.Stdout)
	ncli.Version(cli.VERSION)
	ncli.HelpFlag.Short('h')
	ncli.WithCheats().CheatCommand.Hidden()

	ncli.Flag("server", "NATS server urls").Short('s').Envar("NATS_URL").PlaceHolder("URL").StringVar(&cli.Opts.Servers)
	ncli.Flag("user", "Username or Token").Envar("NATS_USER").PlaceHolder("USER").StringVar(&cli.Opts.Username)
	ncli.Flag("password", "Password").Envar("NATS_PASSWORD").PlaceHolder("PASSWORD").StringVar(&cli.Opts.Password)
	ncli.Flag("creds", "User credentials file (JWT authentication)").Envar("NATS_CREDS").PlaceHolder("FILE").StringVar(&cli.Opts.Creds)
	ncli.Flag("nkey", "User NKEY file for single-key auth").Envar("NATS_NKEY").PlaceHolder("FILE").StringVar(&cli.Opts.Nkey)
	ncli.Flag("tlscert", "TLS public certificate file").Envar("NATS_CERT").PlaceHolder("FILE").ExistingFileVar(&cli.Opts.TlsCert)
	ncli.Flag("tlskey", "TLS private key file").Envar("NATS_KEY").PlaceHolder("FILE").ExistingFileVar(&cli.Opts.TlsKey)
	ncli.Flag("tlsca", "TLS certificate authority chain file").Envar("NATS_CA").PlaceHolder("FILE").ExistingFileVar(&cli.Opts.TlsCA)
	ncli.Flag("tlsfirst", "Perform TLS handshake before expecting the server greeting").BoolVar(&cli.Opts.TlsFirst)
	ncli.Flag("timeout", "Time to wait on responses from NATS").Default("2s").Envar("NATS_TIMEOUT").PlaceHolder("DURATION").DurationVar(&cli.Opts.Timeout)
	ncli.Flag("namespace", "Scoping namespace for applicable operations").Default("default").Envar("NEX_NAMESPACE").StringVar(&cli.Opts.Namespace)

	nodes := ncli.Command("node", "Interact with execution engine nodes")
	nodes_ls := nodes.Command("ls", "List nodes")
	nodes_ls.Action(cli.ListNodes)

	nodes_info := nodes.Command("info", "Get information for an engine node")
	nodes_info.Arg("id", "Public key of the node you're interested in").Required().String()
	nodes_info.Action(cli.NodeInfo)

	run := ncli.Command("run", "Run a workload on a target node")
	run.Arg("url", "URL pointing to the file to run").Required().URLVar(&cli.RunOpts.WorkloadUrl)
	run.Arg("id", "Public key of the target node to run the workload").Required().StringVar(&cli.RunOpts.TargetNode)
	run.Flag("xkey", "Path to publisher's Xkey required to encrypt environment").Required().ExistingFileVar(&cli.RunOpts.PublisherXkeyFile)
	run.Flag("issuer", "Path to a seed key to sign the workload JWT as the issuer").Required().ExistingFileVar(&cli.RunOpts.ClaimsIssuerFile)
	run.Arg("env", "Environment variables to pass to workload").StringMapVar(&cli.RunOpts.Env)
	run.Flag("name", "Name of the workload. Must be alphabetic (lowercase)").Required().StringVar(&cli.RunOpts.Name)
	run.Flag("type", "Type of workload, e.g., \"elf\", \"v8\", \"oci\", \"wasm\"").StringVar(&cli.RunOpts.WorkloadType)
	run.Flag("description", "Description of the workload").StringVar(&cli.RunOpts.Description)
	run.Flag("trigger_subjects", "Trigger subjects to register for subsequent workload execution, if supported by the workload type").StringsVar(&cli.RunOpts.TriggerSubjects)
	run.Action(cli.RunWorkload)

	yeet := ncli.Command("devrun", "Run a workload locating reasonable defaults (developer mode)").Alias("yeet")
	yeet.Arg("file", "File to run").Required().ExistingFileVar(&cli.DevRunOpts.Filename)
	yeet.Arg("env", "Environment variables to pass to workload").StringMapVar(&cli.RunOpts.Env)
	yeet.Flag("trigger_subjects", "Trigger subjects to register for subsequent workload execution, if supported by the workload type").StringsVar(&cli.RunOpts.TriggerSubjects)
	yeet.Flag("stop", "Indicates whether to stop pre-existing workloads during launch. Disable with caution").Default("true").BoolVar(&cli.DevRunOpts.AutoStop)
	yeet.Action(cli.RunDevWorkload)

	stop := ncli.Command("stop", "Stop a running workload")
	stop.Arg("id", "Public key of the target node on which to stop the workload").Required().StringVar(&cli.StopOpts.TargetNode)
	stop.Arg("workload_id", "Unique ID of the workload to be stopped").Required().StringVar(&cli.StopOpts.WorkloadId)
	stop.Flag("name", "Name of the workload to stop").Required().StringVar(&cli.StopOpts.WorkloadName)
	stop.Flag("issuer", "Path to the issuer seed key originally used to start the workload").Required().ExistingFileVar(&cli.StopOpts.ClaimsIssuerFile)
	stop.Action(cli.StopWorkload)

	logs := ncli.Command("logs", "Live monitor workload log emissions")
	logs.Flag("node", "Public key of the nex node to filter on").Default("*").StringVar(&cli.WatchOpts.NodeId)
	logs.Flag("workload_name", "Name of the workload to filter on").Default("*").StringVar(&cli.WatchOpts.WorkloadName)
	logs.Flag("workload_id", "ID of the workload machine to filter on").Default("*").StringVar(&cli.WatchOpts.WorkloadId)
	logs.Flag("level", "Log level filter").Default("debug").StringVar(&cli.WatchOpts.LogLevel)
	logs.Action(cli.WatchLogs)

	evts := ncli.Command("events", "Live monitor events from nex nodes")
	evts.Action(cli.WatchEvents)

	ui := ncli.Command("ui", "Starts a web server for interacting with Nex")
	ui.Flag("port", "Port on which to run the UI").Default("8080").IntVar(&cli.GuiOpts.Port)
	ui.Action(cli.RunUI)

	ncli.MustParseWithUsage(os.Args[1:])
}
