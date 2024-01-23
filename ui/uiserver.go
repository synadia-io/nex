package nexui

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/nats-io/nats.go"
)

// The embed requires a pnpm build. Don't forget to do that
// before building the app

//go:embed web/dist
var app embed.FS

const (
	defaultWebServerPort = 8080
	defaultWebServerHost = "127.0.0.1"
	defaultNatServer     = nats.DefaultURL
)

type WebServerOption func(*WebServer)

type WebServer struct {
	port       int
	host       string
	natsServer string

	Logger *slog.Logger
}

func WithLogger(l *slog.Logger) WebServerOption {
	return func(w *WebServer) {
		w.Logger = l
	}
}

func WithPort(port int) WebServerOption {
	return func(w *WebServer) {
		w.port = port
	}
}

func WithHost(host string) WebServerOption {
	return func(w *WebServer) {
		w.host = host
	}
}

func WithNatsServer(natsServer string) WebServerOption {
	return func(w *WebServer) {
		w.natsServer = natsServer
	}
}

func NewWebServer(options ...WebServerOption) *WebServer {
	w := &WebServer{
		port:       defaultWebServerPort,
		host:       defaultWebServerHost,
		natsServer: defaultNatServer,

		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	for _, option := range options {
		option(w)
	}

	return w
}

func (w WebServer) ServeUI() error {
	dist, err := fs.Sub(app, "web/dist")
	if err != nil {
		return err
	}

	nc, err := nats.Connect(w.natsServer)
	if err != nil {
		w.Logger.Debug("Failed to connect to nats")
		return err
	}

	hub := newHub()
	go hub.run()

	observer, _ := newObserver(hub, nc, w.Logger)
	err = observer.run()
	if err != nil {
		w.Logger.Debug("Failed to run observer")
		return err
	}

	http.Handle("/", http.FileServer(http.FS(dist)))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	w.Logger.Info("Starting nex UI server", "host", w.host, "port", w.port)

	return http.ListenAndServe(fmt.Sprintf("%s:%d", w.host, w.port), nil)
}
