package nexnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"

	_ "embed"
)

var (
	requiredPlugins = [...]fileSpec{
		{name: "/opt/cni/bin/host-local", description: "host-local CNI plugin"},
		{name: "/opt/cni/bin/ptp", description: "ptp CNI plugin"},
		{name: "/opt/cni/bin/tc-redirect-tap", description: "tc-redirect-tap CNI plugin"},
	}
)

type fileSpec struct {
	name        string
	description string
}

func CheckPreRequisites(config *NodeConfiguration) {
	cyan := color.New(color.FgCyan).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	fmt.Printf("\nChecking for required files and dependencies...\n\n")
	for _, p := range requiredPlugins {
		if _, err := os.Stat(p.name); errors.Is(err, os.ErrNotExist) {
			fmt.Printf("⛔ %s was not found (%s)\n", p.description, red(p.name))
		} else {
			fmt.Printf("✅ %s found (%s)\n", p.description, cyan(p.name))
		}
	}

	cniConfig := fmt.Sprintf("/etc/cni/conf.d/%s.conflist", *config.CNI.NetworkName)
	if _, err := os.Stat(cniConfig); errors.Is(err, os.ErrNotExist) {
		err = writeCniConf(cniConfig, *config.CNI.NetworkName)
		if err != nil {
			fmt.Printf("⛔ CNI network configuration file not found (%s), Unable to write a default\n", red(cniConfig))
		} else {
			fmt.Printf("✅ Created a default CNI configuration for %s (%s)\n", cyan(config.CNI.NetworkName), cyan(cniConfig))
		}

	} else {
		fmt.Printf("✅ CNI network configuration file found (%s)\n", cyan(cniConfig))
	}

	firecrackerBinary, err := exec.LookPath("firecracker")
	if err != nil {
		fmt.Println("⛔ Could not locate the 'firecracker' binary in path")
	} else {
		fmt.Printf("✅ Located the firecracker executable in path (%s)\n", cyan(firecrackerBinary))
	}

	if kInfo, err := os.Stat(*config.KernelPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("⛔ Could not access the virtual machine kernel (%s)\n", red(config.KernelPath))
	} else {
		fmt.Printf("✅ Able to access the virtual machine kernel (%s) Last Modified %s\n", cyan(config.KernelPath), cyan(humanize.Time(kInfo.ModTime().Local())))
	}

	if rfsInfo, err := os.Stat(*config.RootFsPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("⛔ Could not access the virtual machine root fs image (%s)\n", red(config.RootFsPath))
	} else {
		fmt.Printf("✅ Able to access the virtual machine root fs image (%s) Last Modified %s\n", cyan(config.RootFsPath), cyan(humanize.Time(rfsInfo.ModTime().Local())))
	}
}

func writeCniConf(fileName string, networkName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}

	var fcConfig map[string]interface{}
	err = json.Unmarshal(defaultFcNetConf, &fcConfig)
	if err != nil {
		return err
	}
	fcConfig["name"] = networkName
	raw, _ := json.Marshal(fcConfig)

	_, err = f.Write(raw)
	if err != nil {
		return nil
	}
	f.Close()
	return nil
}

//go:embed templates/fcnet.conflist
var defaultFcNetConf []byte
