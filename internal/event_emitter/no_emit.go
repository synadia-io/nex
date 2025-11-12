// Package eventemitter provides a implementations of the EventEmitter interface.
package eventemitter

import "github.com/synadia-io/nex/models"

var _ models.EventEmitter = (*NoEmit)(nil)

type NoEmit struct{}

func (NoEmit) EmitEvent(_ string, _ any) error {
	return nil
}
