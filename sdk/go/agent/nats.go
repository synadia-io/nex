package agent

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/synadia-io/nex/models"
)

func natsConnectionOptions(connData models.NatsConnectionData) []nats.Option {
	if connData.ConnName == "" {
		connData.ConnName = "nexlet_go"
	}

	opts := []nats.Option{
		nats.Name(connData.ConnName),
		nats.MaxReconnects(-1),
		nats.Timeout(10 * time.Second),
		// Keep reconnecting through auth errors (e.g. an expired or rotated
		// JWT) instead of permanently aborting the reconnect loop. This only
		// affects reconnect behavior; the initial Connect still fails fast.
		nats.IgnoreAuthErrorAbort(),
		// Don't fire the disconnect/closed handlers for an intentional
		// Close()/Drain(); otherwise routine shutdown logs a misleading
		// "disconnected err=<nil>" / "closed" warning.
		nats.NoCallbacksAfterClientClose(),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Default().Warn("nats connection disconnected", slog.Any("err", err))
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Default().Info("nats connection reconnected", slog.String("url", nc.ConnectedUrl()))
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			slog.Default().Warn("nats connection closed")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			slog.Default().Error("nats connection error", slog.Any("err", err))
		}),
	}

	if connData.TlsCert != "" && connData.TlsKey != "" {
		opts = append(opts, nats.ClientCert(connData.TlsCert, connData.TlsKey))
	}
	if connData.TlsCa != "" {
		opts = append(opts, nats.RootCAs(connData.TlsCa))
	}
	if connData.TlsFirst {
		opts = append(opts, nats.TLSHandshakeFirst())
	}

	// A reloadable NATS creds file takes precedence over the static in-memory
	// credentials. nats.go re-reads the file on every (re)connect, so a
	// refreshed/re-minted credential written before the current one expires is
	// picked up automatically, letting the connection self-heal across
	// expiry/rotation without a restart. Falls back to the static creds
	// (backward compatible) when the env var is unset.
	if credsFile := os.Getenv("NEX_AGENT_NATS_CREDS_FILE"); credsFile != "" {
		opts = append(opts, nats.UserCredentials(credsFile))
		return opts
	}

	switch {
	case connData.NatsUserSeed != "" && connData.NatsUserJwt != "": // Use seed + jwt
		opts = append(opts, nats.UserJWTAndSeed(connData.NatsUserJwt, connData.NatsUserSeed))
	case connData.NatsUserNkey != "" && connData.NatsUserSeed != "": // User nkey
		opts = append(opts, nats.Nkey(connData.NatsUserNkey, func(nonce []byte) ([]byte, error) {
			kp, err := nkeys.FromSeed([]byte(connData.NatsUserSeed))
			if err != nil {
				return nil, err
			}
			return kp.Sign(nonce)
		}))
	case connData.NatsUserName != "" && connData.NatsUserPassword != "": // Use user + password
		opts = append(opts, nats.UserInfo(connData.NatsUserName, connData.NatsUserPassword))
	}

	return opts
}

func configureNatsConnection(connData models.NatsConnectionData) (*nats.Conn, error) {
	opts := natsConnectionOptions(connData)

	if len(connData.NatsServers) == 0 {
		connData.NatsServers = []string{nats.DefaultURL}
	}

	nc, err := nats.Connect(strings.Join(connData.NatsServers, ","), opts...)
	if err != nil {
		return nil, err
	}

	return nc, nil
}
