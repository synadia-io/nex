package models

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"

	"disorder.dev/shandler"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type (
	Reg struct {
		Id              string                `json:"id"`
		OriginalRequest *RegisterAgentRequest `json:"original_request"`
		Schema          *jsonschema.Schema    `json:"-"`
		Type            RegType               `json:"type"`

		Process *os.Process `json:"-"`
	}
	Regs struct {
		sync.Mutex

		logger *slog.Logger
		r      map[string]*Reg
	}
	RegType string
)

const (
	RegTypeEmbeddedAgent RegType = "embedded_agent"
	RegTypeLocalAgent    RegType = "local_agent"
)

func NewRegistrationList(logger *slog.Logger) *Regs {
	return &Regs{
		logger: logger,
		r:      make(map[string]*Reg),
	}
}

func (r *Reg) SetProcess(proc *os.Process) {
	r.Process = proc
}

func (n *Regs) Items() []*Reg {
	n.Lock()
	ret := []*Reg{}
	for _, r := range n.r {
		ret = append(ret, r)
	}
	n.Unlock()
	return ret
}

func (n *Regs) New(id string, _type RegType) {
	n.Lock()
	if n.r == nil {
		n.r = make(map[string]*Reg)
	}
	n.r[id] = new(Reg)
	n.r[id].Id = id
	n.r[id].Type = _type
	n.Unlock()
	n.logger.Log(context.TODO(), shandler.LevelTrace, "registration created", slog.String("id", id), slog.String("type", string(_type)))
}

func (n *Regs) Update(id string, r *Reg) error {
	n.Lock()
	if _, ok := n.r[id]; !ok {
		return errors.New("registration id does not exist")
	}
	n.r[id] = r
	n.Unlock()
	n.logger.Log(context.TODO(), shandler.LevelTrace, "registration updated", slog.String("id", id), slog.Any("registration", r))
	return nil
}

func (n *Regs) Remove(id string) {
	n.Lock()
	delete(n.r, id)
	n.Unlock()
}

func (n *Regs) Has(id string) bool {
	n.Lock()
	_, ok := n.r[id]
	n.Unlock()
	return ok
}

func (n *Regs) Get(id string) *Reg {
	return n.r[id]
}

func (n *Regs) Find(regType string) (string, *Reg, bool) {
	n.Lock()
	for id, r := range n.r {
		if r.OriginalRequest != nil && regType == r.OriginalRequest.RegisterType {
			n.Unlock()
			n.logger.Log(context.TODO(), shandler.LevelTrace, "registration found", slog.String("id", id), slog.String("name", r.OriginalRequest.Name), slog.String("type", regType))
			return id, r, true
		}
	}
	n.Unlock()
	n.logger.Log(context.TODO(), shandler.LevelTrace, "registration not found", slog.String("name", regType))
	return "", nil, false
}

func (n *Regs) Count() int {
	n.Lock()
	defer n.Unlock()
	return len(n.r)
}

func (n *Regs) String() string {
	n.Lock()
	defer n.Unlock()
	ret := strings.Builder{}
	counter := 0
	for _, rr := range n.r {
		if rr == nil {
			continue
		}
		if counter != len(n.r)-1 {
			ret.WriteString(rr.OriginalRequest.RegisterType + ",")
		} else {
			ret.WriteString(rr.OriginalRequest.RegisterType)
		}
		counter++
	}
	return ret.String()
}
