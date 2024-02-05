package nexnode

import "fmt"

var (
	VERSION   = "development"
	COMMIT    = ""
	BUILDDATE = ""
)

func Version() string {
	return VERSION
}

func FullVersion() string {
	return fmt.Sprintf("%s [%s] BuildDate: %s", VERSION, COMMIT, BUILDDATE)
}
