package nexnode

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"

	_ "embed"
)

const (
	VM_LINUX_KERNEL_URL    string = "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.5/x86_64/vmlinux-5.10.186"
	VM_LINUX_KERNEL_SHA256 string = "d48d320e320a8cf970184e79e66a833b044a049a4c2c645b9a1abefdb2fe7b31"

	CNI_PLUGINS_TARBALL_URL    string = "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-amd64-v1.3.0.tgz"
	CNI_PLUGINS_TARBALL_SHA256 string = "https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-amd64-v1.3.0.tgz.sha256"
	// TODO: once awslabs fixes their release action, this URL needs to be changed
	TC_REDIRECT_CNI_PLUGIN_URL    string = "https://github.com/jordan-rash/tc-redirect-tap/releases/download/v0.0.1/tc-redirect-tap-amd64"
	TC_REDIRECT_CNI_PLUGIN_SHA256 string = "https://github.com/jordan-rash/tc-redirect-tap/releases/download/v0.0.1/tc-redirect-tap-amd64.sha256"

	FIRECRACKER_TARBALL_URL    string = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.5.0/firecracker-v1.5.0-x86_64.tgz"
	FIRECRACKER_TARBALL_SHA256 string = "https://github.com/firecracker-microvm/firecracker/releases/download/v1.5.0/firecracker-v1.5.0-x86_64.tgz.sha256.txt"

	ROOTFS_GZIP_URL    string = "https://synadia-nex.s3.us-east-2.amazonaws.com/rootfs.ext4.gz"
	ROOTFS_GZIP_SHA256 string = "https://synadia-nex.s3.us-east-2.amazonaws.com/rootfs.ext4.gz.sha256"
)

var (
	cyan    = color.New(color.FgCyan).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
	green   = color.New(color.FgHiGreen).SprintFunc()
)

type initFunc func(*requirement, *NodeConfiguration) error

type requirement struct {
	directory  string
	files      []fileSpec
	descriptor string
	satisfied  bool
	initFuncs  []initFunc
}
type requirements []*requirement

type fileSpec struct {
	name        string
	description string
	satisfied   bool
}

func CheckPreRequisites(config *NodeConfiguration) error {
	required := &requirements{
		{
			directory: "/opt/cni/bin",
			files: []fileSpec{
				{name: "host-local", description: "host-local CNI plugin"},
				{name: "ptp", description: "ptp CNI plugin"},
				{name: "tc-redirect-tap", description: "tc-redirect-tap CNI plugin"},
			},
			descriptor: "Required CNI Plugins",
			satisfied:  false,
			initFuncs:  []initFunc{downloadCNIPlugins, downloadTCRedirectTap},
		},
		{
			directory: "/usr/local/bin",
			files: []fileSpec{
				{name: "firecracker", description: "Firecracker VM binary"},
			},
			descriptor: "Required binaries",
			satisfied:  false,
			initFuncs:  []initFunc{downloadFirecracker},
		},
		{
			//cniConfig := fmt.Sprintf("/etc/cni/conf.d/%s.conflist", config.CNI.NetworkName)
			directory: "/etc/cni/conf.d",
			files: []fileSpec{
				{name: *config.CNI.NetworkName + ".conflist", description: "CNI Configuration"},
			},
			descriptor: "CNI configuration requirements",
			satisfied:  false,
			initFuncs:  []initFunc{writeCniConf},
		},
		{
			directory: "",
			files: []fileSpec{
				{name: config.KernelFile, description: "VMLinux Kernel"},
				{name: config.RootFsFile, description: "Root Filesystem Template"},
			},
			descriptor: "User provided files",
			satisfied:  false,
			initFuncs:  []initFunc{downloadKernel, downloadRootFS},
		},
	}

	// Verify all directories are present
	for _, r := range *required {
		fmt.Printf("Validating - %s [%s]\n", magenta(r.descriptor), cyan(r.directory))
		allDepsSatified := true
		for _, f := range r.files {
			path := func() string {
				if r.directory == "" {
					return f.name
				} else {
					return filepath.Join(r.directory, f.name)
				}
			}()
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				allDepsSatified = false
				fmt.Printf("\t⛔ Missing Dependency - %s [%s]\n", red(filepath.Join(r.directory, f.name)), cyan(f.description))
			} else {
				f.satisfied = true
				fmt.Printf("\t✅ Dependency Satisfied - %s [%s]\n", green(filepath.Join(r.directory, f.name)), cyan(f.description))
			}
		}
		fmt.Println()
		r.satisfied = allDepsSatified
	}

	for _, r := range *required {
		if r.satisfied {
			continue
		}

		var input []byte
		var err error

		if !config.ForceDepInstall {
			fmt.Printf("⛔ You are missing required dependencies for [%s], do you want to install? [y/N] ", red(r.descriptor))
			inputReader := bufio.NewReader(os.Stdin)
			input, err = inputReader.ReadSlice('\n')
			if err != nil {
				return err
			}
		}
		if config.ForceDepInstall || strings.ToUpper(string(input)) == "Y\n" {
			var path string
			for _, f := range r.files {
				if r.directory == "" {
					path = filepath.Dir(f.name)
				} else {
					path = r.directory
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
		f, err := os.Create(filepath.Join(r.directory, tF.name))
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
	for _, f := range r.files {
		// TODO: this is a hack for now
		if f.description != "VMLinux Kernel" {
			continue
		}

		respBin, err := http.Get(VM_LINUX_KERNEL_URL)

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
	// TODO: firecracker repo made the sha difficult to use
	rawData, err := decompressTarFromURL(FIRECRACKER_TARBALL_URL, "")
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

		if header.Name == "release-v1.5.0-x86_64/firecracker-v1.5.0-x86_64" {
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
	rawData, err := decompressTarFromURL(CNI_PLUGINS_TARBALL_URL, CNI_PLUGINS_TARBALL_SHA256)
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
			outFile, err := os.Create(filepath.Join(r.directory, f))
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
	respBin, err := http.Get(TC_REDIRECT_CNI_PLUGIN_URL)
	if err != nil {
		return err
	}
	defer respBin.Body.Close()

	// TODO: add sha check

	outFile, err := os.Create(filepath.Join(r.directory, "tc-redirect-tap"))
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
	for _, f := range r.files {
		// TODO: this is a hack for now
		if f.description != "Root Filesystem Template" {
			continue
		}

		respTar, err := http.Get(ROOTFS_GZIP_URL)
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

func decompressTarFromURL(url string, urlSha string) (*tar.Reader, error) {
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
