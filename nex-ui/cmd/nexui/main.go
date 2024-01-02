package main

import nexui "github.com/ConnectEverything/nex/nex-ui"

// This is a convenience binary if you just want to run the UI. The "normal"
// way of launching the UI (unless you're testing, etc) should be to use
// the UI subcommand from the nex CLI (`nex ui`)
func main() {
	nexui.ServeUI(8080)
}
