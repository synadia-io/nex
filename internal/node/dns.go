package nexnode

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/3th1nk/cidr"
	"github.com/miekg/dns"
	"github.com/synadia-io/nex/internal/models"
)

const defaultNameserver = "127.0.0.53:53"
const defaultDNSListenTimeoutMillis = 5000

// DNS manages the lifecycle of a local nameserver that functions as
// a resolver for local DNS names and a recursive resolver when a DNS
// query does not match
type DNS struct {
	client  *dns.Client
	closers []io.Closer
	exit    chan error
	handler *dns.ServeMux
	log     *slog.Logger
	server  *dns.Server
	tcp     net.Listener
	tcpAddr *string
	udp     net.PacketConn
	udpAddr *string
}

func NewDNS(log *slog.Logger, config *models.NodeConfiguration) (*DNS, error) {
	udp, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, err
	}

	udpAddr := udp.LocalAddr().String()

	// TODO? -- currently only udp is implemented -- reuse the ephemeral UDP port for a second TCP listener...
	// tcpAddr := tcp.Addr().String() -- this will actually be == udpAddr if implemented

	d := &DNS{
		client:  new(dns.Client),
		exit:    make(chan error, 1),
		handler: dns.NewServeMux(),
		log:     log,
		tcp:     nil,
		udp:     udp,
		udpAddr: &udpAddr,
	}

	d.handler.Handle(".", d) // recursive resolver

	// d.server = &dns.Server{
	// 	Handler:      d.handler,
	// 	Listener:     nil, // FIXME-- to support TCP, set Listener
	// 	PacketConn:   d.udp,
	// 	ReadTimeout:  time.Hour,
	// 	WriteTimeout: time.Hour,
	// }

	// d.handler.Handle(".", d) // recursive resolver

	err = d.Start()
	if err != nil {
		return nil, err
	}

	if config.CNI.Subnet != nil {
		go d.lockdown(*config.CNI.Subnet)
	}

	return d, nil
}

func (d *DNS) initServer() error {
	d.server = &dns.Server{
		Handler:      d.handler,
		Listener:     nil, // FIXME-- to support TCP, set Listener
		PacketConn:   d.udp,
		ReadTimeout:  time.Hour,
		WriteTimeout: time.Hour,
	}

	return d.Start()
}

func (d *DNS) Add(pattern string, ipaddr string) {
	d.handler.HandleFunc(pattern, func(w dns.ResponseWriter, r *dns.Msg) {
		d.log.Debug("Received DNS query for matching pattern", slog.String("msg", r.String()), slog.String("a", ipaddr))

		resp := new(dns.Msg)
		resp.SetReply(r)
		resp.RecursionAvailable = true
		resp.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   fmt.Sprintf("%s.", pattern),
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
				},
				A: net.ParseIP(ipaddr),
			},
		}

		err := w.WriteMsg(resp)
		if err != nil {
			d.log.Warn("Failed to respond to DNS query", slog.String("error", err.Error()))
		}
	})

	d.log.Debug("DNS query handler added", slog.String("pattern", pattern), slog.String("ip", ipaddr))
}

func (d *DNS) Remove(pattern string) {
	d.handler.HandleRemove(pattern)
	d.log.Debug("DNS query handler removed", slog.String("pattern", pattern))
}

func (d *DNS) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	d.log.Debug("Received DNS message and will perform recursive lookup", slog.String("msg", r.String()))

	resp, rtt, err := d.client.Exchange(r, defaultNameserver)
	if err != nil {
		d.log.Warn("Failed to perform recursive DNS lookup", slog.String("error", err.Error()))
		return
	}

	if resp == nil {
		d.log.Warn("Failed to perform recursive DNS lookup: nil response")
		return
	}

	if resp.Rcode != dns.RcodeSuccess {
		d.log.Warn("Received invalid answer to recursive DNS lookup", slog.Int("code", resp.Rcode))
	}

	d.log.Debug("Performed recursive DNS lookup", slog.Int("rtt_ms", int(rtt/time.Millisecond)))

	resp.SetReply(r)
	err = w.WriteMsg(resp)
	if err != nil {
		d.log.Warn("Failed to respond to DNS query", slog.String("error", err.Error()))
	}
}

func (d *DNS) Start() error {
	mutex := sync.Mutex{}
	mutex.Lock()

	d.server = &dns.Server{
		Handler:      d.handler,
		Listener:     nil, // FIXME-- to support TCP, set Listener
		PacketConn:   d.udp,
		ReadTimeout:  time.Hour,
		WriteTimeout: time.Hour,
	}

	d.server.NotifyStartedFunc = mutex.Unlock

	if d.tcp != nil {
		d.closers = append(d.closers, d.tcp)
		d.log.Debug("DNS nameserver listening for TCP", slog.String("addr", *d.tcpAddr))
	}

	if d.udp != nil {
		d.closers = append(d.closers, d.udp)
		d.log.Debug("DNS nameserver listening for UDP", slog.String("addr", *d.udpAddr))
	}

	go func() {
		d.exit <- d.server.ActivateAndServe()

		for _, closer := range d.closers {
			_ = closer.Close()
		}

		d.log.Debug("DNS nameserver exited")
	}()

	mutex.Lock()
	return nil
}

func (d *DNS) Stop() error {
	d.log.Info("DNS nameserver stopping")

	err := d.server.Shutdown()
	if err != nil {
		return err
	}

	return nil
}

func (d *DNS) lockdown(subnet string) error {
	ips, err := cidr.Parse(subnet)
	if err != nil {
		return err
	}

	var addr net.IP
	addr, _ = ips.IPRange()
	cidr.IPIncr(addr)

	err = d.Stop()
	if err != nil {
		d.log.Warn("failed to stop DNS", slog.String("error", err.Error()))
		return err
	}

	d.closers = make([]io.Closer, 0)
	d.exit = make(chan error, 1)
	d.server = nil

	deadline := time.Now().Add(time.Millisecond * defaultDNSListenTimeoutMillis)
	for time.Now().Before(deadline) {
		udpAddr := strings.Split(*d.udpAddr, ":")
		port, err := strconv.Atoi(udpAddr[len(udpAddr)-1])
		if err != nil {
			d.log.Warn("Failed to lockdown DNS server; failed to parse port", slog.String("error", err.Error()))
			return err
		}

		udp, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", addr.String(), port))
		if err != nil {
			d.log.Warn("Failed to lockdown DNS server", slog.String("error", err.Error()))
		}

		if err == nil {
			d.log.Debug("UDP listener bound for lockdown", slog.String("addr", udp.LocalAddr().String()))

			d.udp = udp
			udpAddr := d.udp.LocalAddr().String()
			d.udpAddr = &udpAddr

			err = d.Start()
			if err != nil {
				d.log.Warn("Failed to lockdown DNS server", slog.String("error", err.Error()))
				return err
			}

			break
		}

		// TODO? -- currently only udp is implemented -- reuse the ephemeral UDP port for a second TCP listener...
		// tcpAddr := tcp.Addr().String() -- this will actually be == udpAddr if implemented
	}

	d.log.Debug("Locked down DNS server")

	return nil
}
