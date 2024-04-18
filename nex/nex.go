package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/choria-io/fisk"
	"github.com/fatih/color"
	shandler "github.com/jordan-rash/slog-handler"
	"github.com/nats-io/nats.go"
	"github.com/synadia-io/nex/internal/models"
	nextui "github.com/synadia-io/nex/nex/tui"
)

var (
	VERSION   = "development"
	COMMIT    = ""
	BUILDDATE = ""

	blue = color.New(color.FgBlue).SprintFunc()

	ncli = fisk.New("nex", fmt.Sprintf("%s\nNATS Execution Engine CLI Version %s\n", blue(Banner), VERSION))
	_    = ncli.Author("Synadia Communications")
	_    = ncli.UsageWriter(os.Stdout)
	_    = ncli.Version(fmt.Sprintf("v%s [%s] | Built-on: %s", VERSION, COMMIT, BUILDDATE))
	_    = ncli.HelpFlag.Short('h')
	_    = ncli.WithCheats().CheatCommand.Hidden()

	tui    = ncli.Command("tui", "Start the Nex TUI [BETA]").Alias("ui")
	nodes  = ncli.Command("node", "Interact with execution engine nodes")
	run    = ncli.Command("run", "Run a workload on a target node")
	yeet   = ncli.Command("devrun", "Run a workload locating reasonable defaults (developer mode)").Alias("yeet")
	stop   = ncli.Command("stop", "Stop a running workload")
	logs   = ncli.Command("logs", "Live monitor workload log emissions")
	evts   = ncli.Command("events", "Live monitor events from nex nodes")
	rootfs = ncli.Command("rootfs", "Build custom rootfs").Alias("fs")
	lame   = ncli.Command("lameduck", "Command a node to enter lame duck mdoe")

	nodesLs   = nodes.Command("ls", "List nodes")
	nodesInfo = nodes.Command("info", "Get information for an engine node")

	// These two commands are GOOS/GOARCH dependent
	nodeUp        *fisk.CmdClause
	nodePreflight *fisk.CmdClause

	node_info_id_arg = nodesInfo.Arg("id", "Public key of the node you're interested in").Required().String()

	Opts       = &models.Options{}
	GuiOpts    = &models.UiOptions{}
	RunOpts    = &models.RunOptions{Env: make(map[string]string)}
	DevRunOpts = &models.DevRunOptions{}
	StopOpts   = &models.StopOptions{}
	WatchOpts  = &models.WatchOptions{}
	NodeOpts   = &models.NodeOptions{}
	RootfsOpts = &models.RootfsOptions{}
)

func init() {
	_ = versionCheck()

	ncli.Flag("server", "NATS server urls").Short('s').Envar("NATS_URL").Default(nats.DefaultURL).StringVar(&Opts.Servers)
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
	ncli.Flag("loglevel", "Log level").Default("info").Envar("NEX_LOGLEVEL").EnumVar(&Opts.LogLevel, "debug", "info", "warn", "error")
	ncli.Flag("logjson", "Log JSON").Default("false").Envar("NEX_LOGJSON").UnNegatableBoolVar(&Opts.LogJSON)
	ncli.Flag("logcolor", "Prints text logs with color").Envar("NEX_LOG_COLORIZED").Default("false").UnNegatableBoolVar(&Opts.LogsColorized)
	ncli.Flag("timeformat", "How time is formatted in logger").Envar("NEX_LOG_TIMEFORMAT").Default("DateTime").EnumVar(&Opts.LogTimeFormat, "DateOnly", "DateTime", "Stamp", "RFC822", "RFC3339")
	ncli.Flag("context", "Configuration context").Envar("NATS_CONTEXT").PlaceHolder("NAME").StringVar(&Opts.ConfigurationContext)
	ncli.Flag("no-context", "Disable NATS context discovery").UnNegatableBoolVar(&Opts.SkipContexts)
	ncli.Flag("conn-name", "Name of NATS connection").Default(func() string {
		if VERSION != "development" {
			return "nex-" + VERSION
		}
		return "nex"
	}()).StringVar(&Opts.ConnectionName)

	run.Arg("url", "URL pointing to the file to run").Required().URLVar(&RunOpts.WorkloadUrl)
	run.Arg("id", "Public key of the target node to run the workload").Required().StringVar(&RunOpts.TargetNode)
	run.Flag("xkey", "Path to publisher's Xkey required to encrypt environment").Required().ExistingFileVar(&RunOpts.PublisherXkeyFile)
	run.Flag("issuer", "Path to a seed key to sign the workload JWT as the issuer").Required().ExistingFileVar(&RunOpts.ClaimsIssuerFile)
	run.Arg("env", "Environment variables to pass to workload").StringMapVar(&RunOpts.Env)
	run.Flag("name", "Name of the workload. Must be alphabetic (lowercase)").Required().StringVar(&RunOpts.Name)
	run.Flag("type", "Type of workload").EnumVar(&RunOpts.WorkloadType, "elf", "v8", "wasm")
	run.Flag("description", "Description of the workload").StringVar(&RunOpts.Description)
	run.Flag("argv", "Arguments to pass to the workload, if applicable").StringVar(&RunOpts.Argv)
	run.Flag("essential", "When true, workload is redeployed if it exits with a non-zero status").BoolVar(&RunOpts.Essential)
	run.Flag("trigger_subject", "Trigger subjects to register for subsequent workload execution, if supported by the workload type").StringsVar(&RunOpts.TriggerSubjects)

	yeet.Arg("file", "File to run").Required().ExistingFileVar(&DevRunOpts.Filename)
	yeet.Arg("env", "Environment variables to pass to workload").StringMapVar(&RunOpts.Env)
	yeet.Flag("argv", "Arguments to pass to the workload, if applicable").StringVar(&RunOpts.Argv)
	yeet.Flag("essential", "When true, workload is redeployed if it exits with a non-zero status").BoolVar(&RunOpts.Essential)
	yeet.Flag("trigger_subject", "Trigger subjects to register for subsequent workload execution, if supported by the workload type").StringsVar(&RunOpts.TriggerSubjects)
	yeet.Flag("stop", "Indicates whether to stop pre-existing workloads during launch. Disable with caution").Default("true").BoolVar(&DevRunOpts.AutoStop)
	yeet.Flag("bucketmaxbytes", "Overrides the default max bytes if the dev object store bucket is created").UintVar(&DevRunOpts.DevBucketMaxBytes)

	stop.Arg("id", "Public key of the target node on which to stop the workload").Required().StringVar(&StopOpts.TargetNode)
	stop.Arg("workload_id", "Unique ID of the workload to be stopped").Required().StringVar(&StopOpts.WorkloadId)
	stop.Flag("name", "Name of the workload to stop").Required().StringVar(&StopOpts.WorkloadName)
	stop.Flag("issuer", "Path to the issuer seed key originally used to start the workload").Required().ExistingFileVar(&StopOpts.ClaimsIssuerFile)

	lame.Arg("id", "Public key of the target node to enter lame duck mode").Required().StringVar(&RunOpts.TargetNode)

	logs.Flag("node", "Public key of the nex node to filter on").Default("*").StringVar(&WatchOpts.NodeId)
	logs.Flag("workload_name", "Name of the workload to filter on").Default("*").StringVar(&WatchOpts.WorkloadName)
	logs.Flag("workload_id", "ID of the workload machine to filter on").Default("*").StringVar(&WatchOpts.WorkloadId)
	logs.Flag("level", "Log level filter").Default("debug").StringVar(&WatchOpts.LogLevel)

	rootfs.Flag("script", "Additional boot script ran during initialization").PlaceHolder("script.sh").StringVar(&RootfsOpts.BuildScriptPath)
	rootfs.Flag("image", "Base image for rootfs build").Default("synadia/nex-rootfs:alpine").StringVar(&RootfsOpts.BaseImage)
	rootfs.Flag("agent", "Path to agent binary").PlaceHolder("../path/to/nex-agent").Required().StringVar(&RootfsOpts.AgentBinaryPath)
	rootfs.Flag("size", "Size of rootfs filesystem").Default(strconv.Itoa(1024 * 1024 * 150)).IntVar(&RootfsOpts.RootFSSize) // 150MB default
}

func main() {
	setConditionalCommands()
	cmd := fisk.MustParse(ncli.Parse(os.Args[1:]))

	ctx := context.Background()

	var handlerOpts []shandler.HandlerOption
	switch Opts.LogLevel {
	case "debug":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelDebug))
	case "info":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelInfo))
	case "warn":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelWarn))
	default:
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelError))
	}

	switch Opts.LogTimeFormat {
	case "DateOnly":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.DateOnly))
	case "Stamp":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.Stamp))
	case "RFC822":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.RFC822))
	case "RFC3339":
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.RFC3339))
	default:
		handlerOpts = append(handlerOpts, shandler.WithTimeFormat(time.DateTime))
	}

	var logger *slog.Logger
	if Opts.LogJSON {
		handlerOpts = append(handlerOpts, shandler.WithJSON())
	}
	if Opts.LogsColorized {
		handlerOpts = append(handlerOpts, shandler.WithColor())
	}

	logger = slog.New(shandler.NewHandler(handlerOpts...))

	switch cmd {
	case tui.FullCommand():
		err := nextui.StartTUI(Opts.ConfigurationContext)
		if err != nil {
			fmt.Printf("Failed to start TUI: %s\n", err)
		}
	case nodesLs.FullCommand():
		err := ListNodes(ctx)
		if err != nil {
			fmt.Printf("Failed to list nodes: %s\n", err)
		}
	case nodesInfo.FullCommand():
		err := NodeInfo(ctx, *node_info_id_arg)
		if err != nil {
			fmt.Printf("Failed to get node info: %s\n", err)
		}
	case run.FullCommand():
		err := RunWorkload(ctx, logger)
		if err != nil {
			logger.Error("failed to run workload", slog.Any("err", err))
		}
	case yeet.FullCommand():
		err := RunDevWorkload(ctx, logger)
		if err != nil {
			logger.Error("failed to devrun workload", slog.Any("err", err))
		}
	case stop.FullCommand():
		err := StopWorkload(ctx, logger)
		if err != nil {
			logger.Error("failed to stop workload", slog.Any("err", err))
		}
	case logs.FullCommand():
		err := WatchLogs(ctx, logger)
		if err != nil {
			logger.Error("failed to start log watcher", slog.Any("err", err))
		}
	case evts.FullCommand():
		err := WatchEvents(ctx, logger)
		if err != nil {
			logger.Error("failed to start event watcher", slog.Any("err", err))
		}
	case nodeUp.FullCommand():
		err := RunNodeUp(ctx, logger)
		if err != nil {
			logger.Error("failed to start node", slog.Any("err", err))
		}
	case lame.FullCommand():
		err := LameDuck(ctx, logger)
		if err != nil {
			logger.Error("failed to command node to enter lame duck mode", slog.Any("err", err))
		}
	case nodePreflight.FullCommand():
		err := RunNodePreflight(ctx, logger)
		if err != nil {
			logger.Error("failed to start node", slog.Any("err", err))
		}
	case rootfs.FullCommand():
		err := CreateRootFS(ctx, logger)
		if err != nil {
			logger.Error("failed to build rootfs", slog.Any("err", err))
		}
	}
}
