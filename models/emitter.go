package models

type EventEmitter interface {
	EmitEvent(from string, e any) error
}
