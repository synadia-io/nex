package main

import (
	"fmt"
	"os"

	"github.com/ConnectEverything/nex/internal/models"
	"github.com/choria-io/fisk"
	"github.com/fatih/color"
)

var (
	Opts       = &models.Options{}
	GuiOpts    = &models.UiOptions{}
	RunOpts    = &models.RunOptions{Env: make(map[string]string)}
	DevRunOpts = &models.DevRunOptions{}
	StopOpts   = &models.StopOptions{}
	WatchOpts  = &models.WatchOptions{}
	NodeOpts   = &models.NodeOptions{}
)

func main() {
	blue := color.New(color.FgBlue).SprintFunc()
	help := fmt.Sprintf("%s\nNATS Execution Engine CLI Version %s\n", blue(Banner), VERSION)

	ncli := fisk.New("nex", help)
	ncli.Author("Synadia Communications")
	ncli.UsageWriter(os.Stdout)
	ncli.Version(VERSION)
	ncli.HelpFlag.Short('h')
	ncli.WithCheats().CheatCommand.Hidden()

	ncli.Flag("server", "NATS server urls").Short('s').Envar("NATS_URL").PlaceHolder("URL").StringVar(&Opts.Servers)
	ncli.Flag("user", "Username or Token").Envar("NATS_USER").PlaceHolder("USER").StringVar(&Opts.Username)
	ncli.Flag("password", "Password").Envar("NATS_PASSWORD").PlaceHolder("PASSWORD").StringVar(&Opts.Password)
	ncli.Flag("creds", "User credentials file (JWT authentication)").Envar("NATS_CREDS").PlaceHolder("FILE").StringVar(&Opts.Creds)
	ncli.Flag("nkey", "User NKEY file for single-key auth").Envar("NATS_NKEY").PlaceHolder("FILE").StringVar(&Opts.Nkey)
	ncli.Flag("tlscert", "TLS public certificate file").Envar("NATS_CERT").PlaceHolder("FILE").ExistingFileVar(&Opts.TlsCert)
	ncli.Flag("tlskey", "TLS private key file").Envar("NATS_KEY").PlaceHolder("FILE").ExistingFileVar(&Opts.TlsKey)
	ncli.Flag("tlsca", "TLS certificate authority chain file").Envar("NATS_CA").PlaceHolder("FILE").ExistingFileVar(&Opts.TlsCA)
	ncli.Flag("tlsfirst", "Perform TLS handshake before expecting the server greeting").BoolVar(&Opts.TlsFirst)
	ncli.Flag("timeout", "Time to wait on responses from NATS").Default("2s").Envar("NATS_TIMEOUT").PlaceHolder("DURATION").DurationVar(&Opts.Timeout)
	ncli.Flag("namespace", "Scoping namespace for applicable operations").Default("default").Envar("NEX_NAMESPACE").StringVar(&Opts.Namespace)

	nodes := ncli.Command("node", "Interact with execution engine nodes")
	nodes_ls := nodes.Command("ls", "List nodes")
	nodes_ls.Action(ListNodes)

	nodes_info := nodes.Command("info", "Get information for an engine node")
	nodes_info.Arg("id", "Public key of the node you're interested in").Required().String()
	nodes_info.Action(NodeInfo)

	node_up := nodes.Command("up", "Starts a nex-node")
	node_up.Flag("config", "configuration file for nex-node").Default("./config.json").StringVar(&NodeOpts.Config)
	node_preflight := nodes.Command("preflight", "Checks system for nex-node requirements and installs missing")
	node_preflight.Flag("force", "installs missing dependencies without prompt").Default("false").BoolVar(&NodeOpts.ForceDepInstall)
	node_preflight.Flag("config", "configuration file for nex-node").Default("./config.json").StringVar(&NodeOpts.Config)
	node_up.Action(RunNodeUp)
	node_preflight.Action(RunNodePreflight)

	run := ncli.Command("run", "Run a workload on a target node")
	run.Arg("url", "URL pointing to the file to run").Required().URLVar(&RunOpts.WorkloadUrl)
	run.Arg("id", "Public key of the target node to run the workload").Required().StringVar(&RunOpts.TargetNode)
	run.Flag("xkey", "Path to publisher's Xkey required to encrypt environment").Required().ExistingFileVar(&RunOpts.PublisherXkeyFile)
	run.Flag("issuer", "Path to a seed key to sign the workload JWT as the issuer").Required().ExistingFileVar(&RunOpts.ClaimsIssuerFile)
	run.Arg("env", "Environment variables to pass to workload").StringMapVar(&RunOpts.Env)
	run.Flag("name", "Name of the workload. Must be alphabetic (lowercase)").Required().StringVar(&RunOpts.Name)
	run.Flag("type", "Type of workload, e.g., \"elf\", \"v8\", \"oci\", \"wasm\"").StringVar(&RunOpts.WorkloadType)
	run.Flag("description", "Description of the workload").StringVar(&RunOpts.Description)
	run.Flag("trigger_subjects", "Trigger subjects to register for subsequent workload execution, if supported by the workload type").StringsVar(&RunOpts.TriggerSubjects)
	run.Action(RunWorkload)

	yeet := ncli.Command("devrun", "Run a workload locating reasonable defaults (developer mode)").Alias("yeet")
	yeet.Arg("file", "File to run").Required().ExistingFileVar(&DevRunOpts.Filename)
	yeet.Arg("env", "Environment variables to pass to workload").StringMapVar(&RunOpts.Env)
	yeet.Flag("trigger_subjects", "Trigger subjects to register for subsequent workload execution, if supported by the workload type").StringsVar(&RunOpts.TriggerSubjects)
	yeet.Flag("stop", "Indicates whether to stop pre-existing workloads during launch. Disable with caution").Default("true").BoolVar(&DevRunOpts.AutoStop)
	yeet.Action(RunDevWorkload)

	stop := ncli.Command("stop", "Stop a running workload")
	stop.Arg("id", "Public key of the target node on which to stop the workload").Required().StringVar(&StopOpts.TargetNode)
	stop.Arg("workload_id", "Unique ID of the workload to be stopped").Required().StringVar(&StopOpts.WorkloadId)
	stop.Flag("name", "Name of the workload to stop").Required().StringVar(&StopOpts.WorkloadName)
	stop.Flag("issuer", "Path to the issuer seed key originally used to start the workload").Required().ExistingFileVar(&StopOpts.ClaimsIssuerFile)
	stop.Action(StopWorkload)

	logs := ncli.Command("logs", "Live monitor workload log emissions")
	logs.Flag("node", "Public key of the nex node to filter on").Default("*").StringVar(&WatchOpts.NodeId)
	logs.Flag("workload_name", "Name of the workload to filter on").Default("*").StringVar(&WatchOpts.WorkloadName)
	logs.Flag("workload_id", "ID of the workload machine to filter on").Default("*").StringVar(&WatchOpts.WorkloadId)
	logs.Flag("level", "Log level filter").Default("debug").StringVar(&WatchOpts.LogLevel)
	logs.Action(WatchLogs)

	evts := ncli.Command("events", "Live monitor events from nex nodes")
	evts.Action(WatchEvents)

	ui := ncli.Command("ui", "Starts a web server for interacting with Nex")
	ui.Flag("port", "Port on which to run the UI").Default("8080").IntVar(&GuiOpts.Port)
	ui.Action(RunUI)

	ncli.MustParseWithUsage(os.Args[1:])
}
