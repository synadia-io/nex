package nexnode

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fatih/color"

	_ "embed"
)

func init() {
	rootFSVersion := func() string {
		if VERSION == "development" {
			res, err := http.Get("https://api.github.com/repos/synadia-io/nex/releases/latest")
			if err != nil {
				fmt.Printf("error making http request: %s\n", err)
				return ""
			}
			defer res.Body.Close()

			b, err := io.ReadAll(res.Body)
			if err != nil {
				fmt.Printf("error reading body: %s\n", err)
				return ""
			}

			payload := make(map[string]interface{})
			err = json.Unmarshal(b, &payload)
			if err != nil {
				fmt.Printf("error parsing json: %s\n", err)
				return ""
			}

			latestTag, ok := payload["tag_name"].(string)
			if !ok {
				fmt.Println("error parsing tag_name")
				return ""
			}
			return latestTag
		} else {
			return VERSION
		}
	}()

	switch runtime.GOARCH {
	case "amd64":
		vmLinuxKernelURL = "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.5/x86_64/vmlinux-5.10.186"
		vmLinuxKernelSHA256 = "d48d320e320a8cf970184e79e66a833b044a049a4c2c645b9a1abefdb2fe7b31"

		cniPluginsTarballURL = "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-amd64-v1.3.0.tgz"
		cniPluginsTarballSHA256 = "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-amd64-v1.3.0.tgz.sha256"
		// TODO: once awslabs fixes their release action, this URL needs to be changed
		tcRedirectCNIPluginURL = "https://github.com/jordan-rash/tc-redirect-tap/releases/download/v0.0.1/tc-redirect-tap-amd64"
		tcRedirectCNIPluginSHA256 = "https://github.com/jordan-rash/tc-redirect-tap/releases/download/v0.0.1/tc-redirect-tap-amd64.sha256"

		firecrackerTarballURL = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.5.0/firecracker-v1.5.0-x86_64.tgz"
		firecrackerTarballSHA256 = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.5.0/firecracker-v1.5.0-x86_64.tgz.sha256.txt"

		rootfsGzipURL = fmt.Sprintf("https://github.com/synadia-io/nex/releases/download/%s/rootfs.ext4.gz", rootFSVersion)
		rootfsGzipSHA256 = fmt.Sprintf("https://github.com/synadia-io/nex/releases/download/%s/rootfs.ext4.gz.sha256", rootFSVersion)
	case "arm64":
		vmLinuxKernelURL = "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.5/aarch64/vmlinux-5.10.186"
		vmLinuxKernelSHA256 = ""

		cniPluginsTarballURL = "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-arm64-v1.3.0.tgz"
		cniPluginsTarballSHA256 = "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-arm64-v1.3.0.tgz.sha256"

		tcRedirectCNIPluginURL = "https://github.com/jordan-rash/tc-redirect-tap/releases/download/v0.0.1/tc-redirect-tap-arm64"
		tcRedirectCNIPluginSHA256 = "https://github.com/jordan-rash/tc-redirect-tap/releases/download/v0.0.1/tc-redirect-tap-arm64.sha256"

		firecrackerTarballURL = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.5.0/firecracker-v1.5.0-aarch64.tgz"
		firecrackerTarballSHA256 = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.5.0/firecracker-v1.5.0-aarch64.tgz.sha256.txt"

		rootfsGzipURL = fmt.Sprintf("https://github.com/synadia-io/nex/releases/download/%s/rootfs_arm64.ext4.gz", rootFSVersion)
		rootfsGzipSHA256 = fmt.Sprintf("https://github.com/synadia-io/nex/releases/download/%s/rootfs_arm64.ext4.gz.sha256", rootFSVersion)
	}
}

var (
	cyan    = color.New(color.FgCyan).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
	green   = color.New(color.FgHiGreen).SprintFunc()

	vmLinuxKernelURL    string
	vmLinuxKernelSHA256 string

	cniPluginsTarballURL    string
	cniPluginsTarballSHA256 string

	tcRedirectCNIPluginURL    string
	tcRedirectCNIPluginSHA256 string

	firecrackerTarballURL    string
	firecrackerTarballSHA256 string

	rootfsGzipURL    string
	rootfsGzipSHA256 string
)

type initFunc func(*requirement, *NodeConfiguration) error

type requirement struct {
	directories []string
	files       []*fileSpec
	descriptor  string
	satisfied   bool
	initFuncs   []initFunc
}
type requirements []*requirement

type fileSpec struct {
	name        string
	description string
	satisfied   bool
}

func CheckPrerequisites(config *NodeConfiguration, readonly bool) error {
	var sb strings.Builder

	required := &requirements{
		{
			directories: config.CNI.BinPath,
			files: []*fileSpec{
				{name: "host-local", description: "host-local CNI plugin"},
				{name: "ptp", description: "ptp CNI plugin"},
				{name: "tc-redirect-tap", description: "tc-redirect-tap CNI plugin"},
			},
			descriptor: "Required CNI Plugins",
			satisfied:  false,
			initFuncs:  []initFunc{downloadCNIPlugins, downloadTCRedirectTap},
		},
		{
			directories: config.BinPath,
			files: []*fileSpec{
				{name: "firecracker", description: "Firecracker VM binary"},
			},
			descriptor: "Required binaries",
			satisfied:  false,
			initFuncs:  []initFunc{downloadFirecracker},
		},
		{
			//cniConfig := fmt.Sprintf("/etc/cni/conf.d/%s.conflist", config.CNI.NetworkName)
			directories: []string{"/etc/cni/conf.d"},
			files: []*fileSpec{
				{name: *config.CNI.NetworkName + ".conflist", description: "CNI Configuration"},
			},
			descriptor: "CNI configuration requirements",
			satisfied:  false,
			initFuncs:  []initFunc{writeCniConf},
		},
		{
			directories: []string{""},
			files: []*fileSpec{
				{name: config.KernelFilepath, description: "VMLinux Kernel"},
			},
			descriptor: "VMLinux Kernel",
			satisfied:  false,
			initFuncs:  []initFunc{downloadKernel},
		},
		{
			directories: []string{""},
			files: []*fileSpec{
				{name: config.RootFsFilepath, description: "Root Filesystem Template"},
			},
			descriptor: "Root Filesystem Template",
			satisfied:  false,
			initFuncs:  []initFunc{downloadRootFS},
		},
	}

	// Verify all directories are present
	for _, r := range *required {
		sb.WriteString(fmt.Sprintf("Validating - %s\n", magenta(r.descriptor)))

		depsFound := 0
		for _, dir := range r.directories {
			if dir != "" {
				sb.WriteString(fmt.Sprintf("\t  🔎Searching - %s \n", cyan(dir)))
			}

			for _, f := range r.files {
				path := func() string {
					if dir == "" {
						return f.name
					} else {
						return filepath.Join(dir, f.name)
					}
				}()

				if _, err := os.Stat(path); err == nil {
					depsFound += 1
					f.satisfied = true
					sb.WriteString(fmt.Sprintf("\t  ✅ Dependency Satisfied - %s [%s]\n", green(filepath.Join(dir, f.name)), cyan(f.description)))
				}
			}

			if depsFound == len(r.files) {
				break
			}
		}

		for _, f := range r.files {
			if f.satisfied {
				continue
			}
			sb.WriteString(fmt.Sprintf("\t⛔ Missing Dependency - %s\n", cyan(f.description)))
		}

		sb.WriteString(fmt.Sprintln())
		r.satisfied = depsFound == len(r.files)
	}

	if !readonly {
		fmt.Print(sb.String())
	}

	for _, r := range *required {
		if r.satisfied {
			continue
		}

		var input []byte
		var err error

		if !config.ForceDepInstall {
			if readonly {
				return fmt.Errorf("configuration prerequisites not met")
			}

			fmt.Printf("⛔ You are missing required dependencies for [%s], do you want to install? [y/N] ", red(r.descriptor))
			inputReader := bufio.NewReader(os.Stdin)
			input, err = inputReader.ReadSlice('\n')
			if err != nil {
				return err
			}
		}
		if config.ForceDepInstall || strings.ToUpper(string(input)) == "Y\n" {
			var path string
			dir := ""
			if len(r.directories) > 0 {
				// we install into the first directory if specified
				dir = r.directories[0]
			}
			for _, f := range r.files {
				if dir == "" {
					path = filepath.Dir(f.name)
				} else {
					path = dir
				}
				err = os.MkdirAll(path, 0755)
				if err != nil {
					return err
				}
			}

			for _, iF := range r.initFuncs {
				err := iF(r, config)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// func writeCniConf(fileName string, networkName string) error {
func writeCniConf(r *requirement, c *NodeConfiguration) error {
	for _, tF := range r.files {
		f, err := os.Create(filepath.Join(r.directories[0], tF.name))
		if err != nil {
			return err
		}
		defer f.Close()

		var fcConfig map[string]interface{}
		err = json.Unmarshal(defaultFcNetConf, &fcConfig)
		if err != nil {
			return err
		}

		fcConfig["name"] = c.CNI.NetworkName
		raw, err := json.Marshal(fcConfig)
		if err != nil {
			return err
		}

		_, err = f.Write(raw)
		if err != nil {
			return nil
		}
	}

	return nil
}

func downloadKernel(r *requirement, _ *NodeConfiguration) error {
	_ = vmLinuxKernelSHA256 // TODO: implement sha verification
	for _, f := range r.files {
		// TODO: this is a hack for now
		if f.description != "VMLinux Kernel" {
			continue
		}

		respBin, err := http.Get(vmLinuxKernelURL)

		if err != nil {
			return err
		}
		defer respBin.Body.Close()

		// TODO: add sha check

		outFile, err := os.Create(f.name)
		if err != nil {
			fmt.Println(err)
			return err
		}
		_, err = io.Copy(outFile, respBin.Body)
		if err != nil {
			return err
		}
		outFile.Close()
	}

	return nil
}

func downloadFirecracker(_ *requirement, _ *NodeConfiguration) error {
	_ = firecrackerTarballSHA256
	// TODO: firecracker repo made the sha difficult to use
	rawData, err := decompressTarFromURL(firecrackerTarballURL, "")
	if err != nil {
		return err
	}

	for {
		header, err := rawData.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Name == "release-v1.5.0-x86_64/firecracker-v1.5.0-x86_64" || header.Name == "release-v1.5.0-aarch64/firecracker-v1.5.0-aarch64" {
			outFile, err := os.Create("/usr/local/bin/firecracker")
			if err != nil {
				fmt.Println(err)
				return err
			}
			_, err = io.Copy(outFile, rawData)
			if err != nil {
				fmt.Println(err)
				return err
			}
			outFile.Close()

			err = os.Chmod(outFile.Name(), 0755)
			if err != nil {
				fmt.Println(err)
				return err
			}
		}

	}
	return nil
}

func downloadCNIPlugins(r *requirement, c *NodeConfiguration) error {
	rawData, err := decompressTarFromURL(cniPluginsTarballURL, cniPluginsTarballSHA256)
	if err != nil {
		return err
	}

	for {
		header, err := rawData.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		f := strings.TrimPrefix(strings.TrimSpace(header.Name), "./")

		if f == "ptp" || f == "host-local" {
			outFile, err := os.Create(filepath.Join(r.directories[0], f))
			if err != nil {
				fmt.Println(err)
				return err
			}
			_, err = io.Copy(outFile, rawData)
			if err != nil {
				return err
			}
			outFile.Close()

			err = os.Chmod(outFile.Name(), 0755)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func downloadTCRedirectTap(r *requirement, _ *NodeConfiguration) error {
	_ = tcRedirectCNIPluginSHA256
	respBin, err := http.Get(tcRedirectCNIPluginURL)
	if err != nil {
		return err
	}
	defer respBin.Body.Close()

	// TODO: add sha check

	outFile, err := os.Create(filepath.Join(r.directories[0], "tc-redirect-tap"))
	if err != nil {
		fmt.Println(err)
		return err
	}
	_, err = io.Copy(outFile, respBin.Body)
	if err != nil {
		return err
	}
	outFile.Close()

	err = os.Chmod(outFile.Name(), 0755)
	if err != nil {
		return err
	}
	return nil
}

func downloadRootFS(r *requirement, _ *NodeConfiguration) error {
	_ = rootfsGzipSHA256
	for _, f := range r.files {
		// TODO: this is a hack for now
		if f.description != "Root Filesystem Template" {
			continue
		}

		respTar, err := http.Get(rootfsGzipURL)
		if err != nil {
			return err
		}
		defer respTar.Body.Close()

		uncompressedFile, err := gzip.NewReader(respTar.Body)
		if err != nil {
			return err
		}
		outFile, err := os.Create(f.name)
		if err != nil {
			fmt.Println(err)
			return err
		}
		_, err = io.Copy(outFile, uncompressedFile)
		if err != nil {
			return err
		}
		outFile.Close()
	}
	return nil
}

func decompressTarFromURL(url string, _ string) (*tar.Reader, error) {
	respTar, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	//	defer respTar.Body.Close()

	// 	if urlSha != "" {
	// 		respSha, err := http.Get(urlSha)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		defer respSha.Body.Close()
	// 		sha256_b, err := io.ReadAll(respSha.Body)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	//
	// //		sha256_g := sha256.Sum256(tar_b)
	//
	// 		if string(sha256_b) != string(sha256_g[:]) {
	// 			fmt.Println(string(sha256_b))
	// 			fmt.Println(string(sha256_g[:]))
	// 			return nil, errors.New("downloaded tar did not match provided SHA256")
	// 		}
	// 	}

	uncompressedTar, err := gzip.NewReader(respTar.Body)
	if err != nil {
		return nil, err
	}

	rawData := tar.NewReader(uncompressedTar)
	return rawData, nil
}

//go:embed templates/fcnet.conflist
var defaultFcNetConf []byte
