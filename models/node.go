package models

const (
	// This is a special namespace that is used for "admin" only requests
	// The node listens on a handful of topics for and typically the
	// 3rd position is the namespace.  There are some endpoints that can be seen in
	// `node/internal/actors/subjects.go` that have "system" hardcoded in
	// the third position.  These are considered privledged endpoints
	NodeSystemNamespace string = "system"
)
