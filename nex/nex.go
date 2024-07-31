package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"time"

	"disorder.dev/shandler"
	"github.com/choria-io/fisk"
	"github.com/fatih/color"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	controlapi "github.com/synadia-io/nex/control-api"
	"github.com/synadia-io/nex/internal/models"
	natslogger "github.com/synadia-io/nex/internal/nats-logger"
	nextui "github.com/synadia-io/nex/nex/tui"
)

var (
	VERSION   = "development"
	COMMIT    = ""
	BUILDDATE = ""

	updatable = ""

	blue = color.New(color.FgBlue).SprintFunc()

	ncli = fisk.New("nex", fmt.Sprintf("%s\nNATS Execution Engine CLI Version %s\n", blue(Banner), VERSION))
	_    = ncli.Author("Synadia Communications")
	_    = ncli.UsageWriter(os.Stdout)
	_    = ncli.Version(fmt.Sprintf("v%s [%s] | Built-on: %s", VERSION, COMMIT, BUILDDATE))
	_    = ncli.HelpFlag.Short('h')
	_    = ncli.WithCheats().CheatCommand.Hidden()

	tui     = ncli.Command("tui", "Start the Nex TUI [BETA]").Alias("ui")
	nodes   = ncli.Command("node", "Interact with execution engine nodes").Alias("nodes")
	run     = ncli.Command("run", "Run a workload on a target node")
	yeet    = ncli.Command("devrun", "Run a workload locating reasonable defaults (developer mode)").Alias("yeet")
	stop    = ncli.Command("stop", "Stop a running workload")
	logs    = ncli.Command("logs", "Live monitor workload log emissions")
	evts    = ncli.Command("events", "Live monitor events from nex nodes")
	rootfs  = ncli.Command("rootfs", "Build custom rootfs").Alias("fs")
	lame    = ncli.Command("lameduck", "Command a node to enter lame duck mode")
	upgrade = ncli.Command("upgrade", "Upgrade the NEX CLI to the latest version")

	nodesLs   = nodes.Command("ls", "List nodes")
	nodesInfo = nodes.Command("info", "Get information for an engine node")

	nodesProbe = nodes.Command("probe", "Probe nodes for matching workloads")

	// These two commands are GOOS/GOARCH dependent
	nodeUp        *fisk.CmdClause
	nodePreflight *fisk.CmdClause

	node_info_id_arg = nodesInfo.Arg("id", "Public key of the node you're interested in").Required().String()
	node_info_full   = nodesInfo.Flag("full", "Long form output").Default("false").UnNegatableBool()

	Opts       = &models.Options{}
	GuiOpts    = &models.UiOptions{}
	RunOpts    = &models.RunOptions{Env: make(map[string]string)}
	DevRunOpts = &models.DevRunOptions{}
	StopOpts   = &models.StopOptions{}
	WatchOpts  = &models.WatchOptions{}
	NodeOpts   = &models.NodeOptions{}
	RootfsOpts = &models.RootfsOptions{}

	workloadType string
)

func init() {
	updatable, _ = versionCheck()

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
	ncli.Flag("jsdomain", "Jetsteam domain to use in nats connection").PlaceHolder("nex").StringVar(&Opts.JsDomain)
	ncli.Flag("namespace", "Scoping namespace for applicable operations").Default("default").Envar("NEX_NAMESPACE").StringVar(&Opts.Namespace)
	ncli.Flag("logger", "How to log").Default("std").Envar("NEX_LOGGER").StringsVar(&Opts.Logger) // Valid options: "std", "file", "nats"
	ncli.Flag("loglevel", "Log level").Default("info").Envar("NEX_LOGLEVEL").EnumVar(&Opts.LogLevel, "none", "trace", "debug", "info", "warn", "error")
	ncli.Flag("logjson", "Log JSON").Default("false").Envar("NEX_LOGJSON").UnNegatableBoolVar(&Opts.LogJSON)
	ncli.Flag("logcolor", "Prints text logs with color").Envar("NEX_LOG_COLORIZED").Default("false").UnNegatableBoolVar(&Opts.LogsColorized)
	ncli.Flag("timeformat", "How time is formatted in logger").Envar("NEX_LOG_TIMEFORMAT").Default("DateTime").EnumVar(&Opts.LogTimeFormat, "DateOnly", "DateTime", "Stamp", "RFC822", "RFC3339")
	ncli.Flag("context", "Configuration context").Envar("NATS_CONTEXT").PlaceHolder("NAME").StringVar(&Opts.ConfigurationContext)
	ncli.Flag("conn-name", "Name of NATS connection").Default(func() string {
		if VERSION != "development" {
			return "nex-" + VERSION
		}
		return "nex"
	}()).StringVar(&Opts.ConnectionName)

	run.Arg("url", "URL pointing to the file to run").Required().URLVar(&RunOpts.WorkloadURL)
	run.Arg("id", "Public key of the target node to run the workload").Required().StringVar(&RunOpts.TargetNode)
	run.Flag("xkey", "Path to publisher's Xkey required to encrypt environment").Required().ExistingFileVar(&RunOpts.PublisherXkeyFile)
	run.Flag("issuer", "Path to a seed key to sign the workload JWT as the issuer").Required().ExistingFileVar(&RunOpts.ClaimsIssuerFile)
	run.Arg("env", "Environment variables to pass to workload").StringMapVar(&RunOpts.Env)
	run.Flag("name", "Name of the workload. Must be alphabetic (lowercase)").Required().StringVar(&RunOpts.Name)
	run.Flag("type", "Type of workload").Default("native").EnumVar(&workloadType, "native", "v8", "wasm")
	run.Flag("description", "Description of the workload").StringVar(&RunOpts.Description)
	run.Flag("argv", "Arguments to pass to the workload, if applicable").StringVar(&RunOpts.Argv)
	run.Flag("essential", "When true, workload is redeployed if it exits with a non-zero status").BoolVar(&RunOpts.Essential)
	run.Flag("trigger_subject", "Trigger subjects to register for subsequent workload execution, if supported by the workload type").StringsVar(&RunOpts.TriggerSubjects)
	run.Flag("hs_url", "Override the URL used for host services for this workload").StringVar(&RunOpts.HsUrl)
	run.Flag("hs_jwt", "Set the user JWT for override host services connection").StringVar(&RunOpts.HsUserJwt)
	run.Flag("hs_seed", "Set the user seed for override host services connection").StringVar(&RunOpts.HsUserSeed)

	yeet.Arg("file", "File to run").Required().ExistingFileVar(&DevRunOpts.Filename)
	yeet.Arg("env", "Environment variables to pass to workload").StringMapVar(&RunOpts.Env)
	yeet.Flag("argv", "Arguments to pass to the workload, if applicable").StringVar(&RunOpts.Argv)
	yeet.Flag("essential", "When true, workload is redeployed if it exits with a non-zero status").BoolVar(&RunOpts.Essential)
	yeet.Flag("trigger_subject", "Trigger subjects to register for subsequent workload execution, if supported by the workload type").StringsVar(&RunOpts.TriggerSubjects)
	yeet.Flag("stop", "Indicates whether to stop pre-existing workloads during launch. Disable with caution").Default("true").BoolVar(&DevRunOpts.AutoStop)
	yeet.Flag("bucketmaxbytes", "Overrides the default max bytes if the dev object store bucket is created").UintVar(&DevRunOpts.DevBucketMaxBytes)
	yeet.Flag("type", "Type of workload").Default("native").StringVar(&workloadType)

	stop.Arg("id", "Public key of the target node on which to stop the workload").Required().StringVar(&StopOpts.TargetNode)
	stop.Arg("workload_id", "Unique ID of the workload to be stopped").Required().StringVar(&StopOpts.WorkloadId)
	stop.Flag("issuer", "Path to the issuer seed key originally used to start the workload").Required().ExistingFileVar(&StopOpts.ClaimsIssuerFile)

	lame.Arg("id", "Public key of the target node to enter lame duck mode").Required().StringVar(&RunOpts.TargetNode)

	logs.Flag("node", "Public key of the nex node to filter on").Default("*").StringVar(&WatchOpts.NodeId)
	logs.Flag("workload_name", "Name of the workload to filter on").Default("*").StringVar(&WatchOpts.WorkloadName)
	logs.Flag("workload_id", "ID of the workload machine to filter on").Default("*").StringVar(&WatchOpts.WorkloadId)
	logs.Flag("level", "Log level filter").Default("debug").StringVar(&WatchOpts.LogLevel)

	rootfs.Flag("output", "Output name").Short('o').Default("rootfs.ext4.gz").StringVar(&RootfsOpts.OutName)
	rootfs.Flag("script", "Additional boot script ran during initialization").PlaceHolder("script.sh").StringVar(&RootfsOpts.BuildScriptPath)
	rootfs.Flag("image", "Base image for rootfs build").Default("synadia/nex-rootfs:alpine").StringVar(&RootfsOpts.BaseImage)
	rootfs.Flag("agent", "Path to agent binary").PlaceHolder("../path/to/nex-agent").Required().StringVar(&RootfsOpts.AgentBinaryPath)
	rootfs.Flag("size", "Size of rootfs filesystem").Default(strconv.Itoa(1024 * 1024 * 150)).IntVar(&RootfsOpts.RootFSSize) // 150MB default

	nodesLs.Flag("full", "List more detailed table").Default("false").UnNegatableBoolVar(&NodeOpts.ListFull)

	// one day when we refactor, let's get rid of all of these global structs. Such ugly
	nodesProbe.Flag("workload", "Only query nodes currently running the given workload (id or name)").StringVar(&RunOpts.Name)
}

func main() {
	setConditionalCommands()
	setDebuggerCommands()

	cmd := fisk.MustParse(ncli.Parse(os.Args[1:]))

	switch workloadType {
	case "native":
		RunOpts.WorkloadType = controlapi.NexWorkloadNative
	case "v8":
		RunOpts.WorkloadType = controlapi.NexWorkloadV8
	case "oci":
		RunOpts.WorkloadType = controlapi.NexWorkloadOCI
	case "wasm":
		RunOpts.WorkloadType = controlapi.NexWorkloadWasm
	default:
		RunOpts.WorkloadType = controlapi.NexWorkload(workloadType)
	}

	ctx := context.Background()

	var handlerOpts []shandler.HandlerOption
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

	if Opts.LogJSON {
		handlerOpts = append(handlerOpts, shandler.WithJSON())
	}
	if Opts.LogsColorized {
		handlerOpts = append(handlerOpts, shandler.WithColor())
	}

	handlerOpts = append(handlerOpts, shandler.WithShortLevels())

	logger := slog.New(shandler.NewHandler(handlerOpts...))
	keypair, err := nkeys.CreateServer()
	if err != nil {
		panic(err)
	}
	pk, _ := keypair.PublicKey()

	stdoutWriters := []io.Writer{}
	stderrWriters := []io.Writer{}
	if slices.Contains(Opts.Logger, "std") {
		stdoutWriters = append(stdoutWriters, os.Stdout)
		stderrWriters = append(stderrWriters, os.Stderr)
	}
	if slices.Contains(Opts.Logger, "file") {
		stdout, err := os.Create("nex.log")
		if err == nil {
			stderr, err := os.Create("nex.err")
			if err == nil {
				stdoutWriters = append(stdoutWriters, stdout)
				stderrWriters = append(stderrWriters, stderr)
			}
		}
	}
	if slices.Contains(Opts.Logger, "nats") {
		natsLogSubject := fmt.Sprintf("$NEX.logs.system.%s.stdout", pk)
		natsErrLogSubject := fmt.Sprintf("$NEX.logs.system.%s.stderr", pk)
		nc, err := models.GenerateConnectionFromOpts(Opts, logger)
		if err == nil {
			defer func() {
				err := nc.Drain()
				if err != nil {
					logger.Error("Drain error", slog.Any("err", err))
				}
				for !nc.IsClosed() {
					time.Sleep(time.Millisecond * 25)
				}
			}()
			stdoutWriters = append(stdoutWriters, natslogger.NewNatsLogger(nc, natsLogSubject))
			stderrWriters = append(stderrWriters, natslogger.NewNatsLogger(nc, natsErrLogSubject))
		}
	}

	switch Opts.LogLevel {
	case "none":
		stdoutWriters = []io.Writer{io.Discard}
		stderrWriters = []io.Writer{io.Discard}
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(12))
	case "trace":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(shandler.LevelTrace))
	case "debug":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelDebug))
	case "info":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelInfo))
	case "warn":
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelWarn))
	default:
		handlerOpts = append(handlerOpts, shandler.WithLogLevel(slog.LevelError))
	}

	handlerOpts = append(handlerOpts, shandler.WithStdOut(stdoutWriters...))
	handlerOpts = append(handlerOpts, shandler.WithStdErr(stderrWriters...))

	logger = slog.New(shandler.NewHandler(handlerOpts...))

	switch cmd {
	case tui.FullCommand():
		err := nextui.StartTUI(Opts.ConfigurationContext)
		if err != nil {
			logger.Error("Failed to start TUI", slog.Any("err", err))
		}
	case nodesLs.FullCommand():
		err := PingNodes(ctx)
		if err != nil {
			logger.Error("Failed to list nodes", slog.Any("err", err))
		}
	case nodesProbe.FullCommand():
		err := ListWorkloads(ctx)
		if err != nil {
			logger.Error("Failed to list workloads", slog.Any("err", err))
		}
	case nodesInfo.FullCommand():
		err := NodeInfo(ctx, *node_info_id_arg, *node_info_full)
		if err != nil {
			logger.Error("Failed to get node info", slog.Any("err", err))
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
		err := RunNodeUp(ctx, logger, keypair)
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
			logger.Error("failed to run preflight", slog.Any("err", err))
		}
	case rootfs.FullCommand():
		err := CreateRootFS(ctx, logger)
		if err != nil {
			logger.Error("failed to build rootfs", slog.Any("err", err))
		}
	case upgrade.FullCommand():
		if updatable != "" {
			_, err := UpgradeNex(ctx, logger, updatable)
			if err != nil {
				logger.Error("failed to upgrade nex", slog.Any("err", err))
			}
		} else {
			logger.Info("no new version found")
		}
	}
}
