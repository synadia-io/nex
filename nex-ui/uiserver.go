package nexui

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// The embed requires a pnpm build. Don't forget to do that
// before building the app

//go:embed web/dist
var app embed.FS

func ServeUI(port int) {
	dist, err := fs.Sub(app, "web/dist")
	if err != nil {
		log.Fatalf("sub error: %s", err)
		return
	}

	// TODO: allow this to change via env vars
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		log.Fatalf("Failed to connect to nats: %s", err)
	}

	log := logrus.New()

	hub := newHub()
	go hub.run()

	observer, _ := newObserver(hub, nc, log)
	err = observer.run()
	if err != nil {
		log.WithError(err).Fatal("Failed to run observer")
	}

	http.Handle("/", http.FileServer(http.FS(dist)))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	log.WithField("port", port).Info("Starting nex UI server")

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
