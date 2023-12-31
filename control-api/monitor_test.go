package controlapi

import (
	"encoding/json"
	"time"

	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("event monitor", func() {
	var nc *nats.Conn
	var log *logrus.Logger
	var client *apiClient
	var ch chan EmittedEvent
	var subject EmittedEvent

	type testStruct struct {
		Bob   string `json:"bob"`
		Alice string `json:"alice"`
	}

	BeforeEach(func() {
		var err error

		nc, err = nats.Connect(nats.DefaultURL)
		Expect(err).ToNot(BeNil())

		log = logrus.New()
		client = NewApiClient(nc, time.Second, log)
		ch, err = client.MonitorEvents("*", "*", 0)
		Expect(err).ToNot(BeNil())

		evt := cloudevents.NewEvent()
		evt.SetType("workload_started")
		evt.SetID("1")
		evt.SetSource("testing")
		_ = evt.SetData(testStruct{
			Bob:   "1",
			Alice: "2",
		})

		bytes, _ := json.Marshal(evt)
		_ = nc.Publish("$NEX.events.default.workload_started", bytes)

		subject = <-ch
	})

	It("maintains namespace", func(ctx SpecContext) {
		Expect(subject.Namespace).To(Equal("default"))
	})

	It("maintains event type", func(ctx SpecContext) {
		Expect(subject.EventType).To(Equal("workload_started"))
	})

	Describe("DataAs", func() {
		var _subject testStruct

		BeforeEach(func() {
			err := subject.DataAs(&_subject)
			Expect(err).To(BeNil()) // Event wrapper lost fidelity of event data
		})

		It("maintains fidelity of `Alice`", func(ctx SpecContext) {
			Expect(_subject.Alice).To(Equal("2"))
		})

		It("maintains fidelity of `Bob`", func(ctx SpecContext) {
			Expect(_subject.Bob).To(Equal("1"))
		})
	})
})

var _ = Describe("log monitor", func() {
	var nc *nats.Conn
	var log *logrus.Logger
	var client *apiClient
	var ch chan EmittedLog
	var subject EmittedLog

	var raw rawLog // FIXME...

	BeforeEach(func() {
		var err error

		nc, err = nats.Connect(nats.DefaultURL)
		Expect(err).ToNot(BeNil())

		log = logrus.New()
		client = NewApiClient(nc, time.Second, log)
		ch, err = client.MonitorLogs("*", "*", "*", "*", 0)
		Expect(err).ToNot(BeNil())

		raw = rawLog{Text: "hey from test", Level: logrus.DebugLevel, MachineId: "vm1234"}
		bytes, _ := json.Marshal(raw)

		_ = nc.Publish("$NEX.logs.default.Nxxxx.echoservice.vm1234", bytes)
		subject = <-ch
	})

	It("sets the default namespace", func(ctx SpecContext) {
		Expect(subject.Namespace).To(Equal("default"))
	})

	It("includes the node id", func(ctx SpecContext) {
		Expect(subject.NodeId).To(Equal("Nxxxx"))
	})

	It("includes the workload", func(ctx SpecContext) {
		Expect(subject.Workload).To(Equal("echoservice"))
	})

	It("includes the machine id", func(ctx SpecContext) {
		Expect(subject.MachineId).To(Equal("vm1234"))
	})

	It("wraps the raw on the wire log", func(ctx SpecContext) {
		Expect(subject.rawLog).To(Equal(raw))
	})
})
