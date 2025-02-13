package models

import (
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type (
	Reg struct {
		Id              string
		OriginalRequest *RegisterAgentRequest
		Schema          *jsonschema.Schema
		Type            RegType

		Process *os.Process
	}
	Regs struct {
		sync.Mutex
		r map[string]*Reg
	}
	RegType string
)

const (
	RegTypeEmbeddedAgent RegType = "embedded_agent"
	RegTypeLocalAgent    RegType = "local_agent"
)

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
}

func (n *Regs) Update(id string, r *Reg) error {
	n.Lock()
	if _, ok := n.r[id]; !ok {
		return errors.New("registration id does not exist")
	}
	n.r[id] = r
	n.Unlock()
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

func (n *Regs) Find(name string) (string, *Reg, bool) {
	n.Lock()
	for id, r := range n.r {
		if r.OriginalRequest != nil && name == r.OriginalRequest.Name {
			n.Unlock()
			return id, r, true
		}
	}
	n.Unlock()
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
			ret.WriteString(rr.OriginalRequest.Name + ",")
		} else {
			ret.WriteString(rr.OriginalRequest.Name)
		}
		counter++
	}
	return ret.String()
}
