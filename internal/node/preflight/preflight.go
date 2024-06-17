package preflight

import (
	// "archive/tar"
	// "bufio"
	// "bytes"
	// "compress/gzip"
	// "crypto/sha256"
	// "encoding/hex"
	// "errors"
	"fmt"
	// "html/template"
	// "io"
	"log/slog"
	//	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/synadia-io/nex/internal/models"
	// "github.com/synadia-io/nex/internal/node/templates"
)

type initFunc func(*requirement, *models.NodeConfiguration, *slog.Logger) error

//	type requirement struct {
//		directories []string
//		files       []*fileSpec
//		descriptor  string
//		satisfied   bool
//		initFuncs   []initFunc
//	}
type requirements []*requirement

type requirement struct {
	name        string
	path        []string
	description string
	nosandbox   bool
	satisfied   bool
	iF          initFunc
}

func Preflight(config *models.NodeConfiguration, logger *slog.Logger) PreflightError {
	err := preflightInit(config, logger)
	if err != nil {
		return err
	}

	var sb strings.Builder
	required := requirements{
		{name: "host-local", path: config.CNI.BinPath, nosandbox: false, description: "host-local CNI plugin", iF: downloadCNIPlugins},
		{name: "ptp", path: config.CNI.BinPath, nosandbox: false, description: "ptp CNI plugin", iF: downloadCNIPlugins},
		{name: "tc-redirect-tap", path: config.CNI.BinPath, nosandbox: false, description: "tc-redirect-tap CNI plugin", iF: downloadTCRedirectTap},
		{name: "firecracker", path: config.BinPath, nosandbox: false, description: "Firecracker VM binary", iF: downloadFirecracker},
		{name: "nex-agent", path: config.BinPath, nosandbox: true, description: "Nex-agent binary", iF: installNexAgent},
		{name: *config.CNI.NetworkName + ".conflist", path: []string{"/etc/cni/conf.d"}, nosandbox: false, description: "CNI Configuration", iF: writeCniConf},
		{name: config.KernelFilepath, path: []string{config.KernelFilepath}, nosandbox: false, description: "VMLinux Kernel", iF: downloadKernel},
		{name: config.RootFsFilepath, path: []string{config.RootFsFilepath}, nosandbox: false, description: "Root Filesystem Template", iF: downloadRootFS},
	}

	// TODO: move this to a flag
	verbose := false

	satisfiedCount := 0
	for _, r := range required {
		for di, dir := range r.path {
			if r.nosandbox == config.NoSandbox {
				if di == 0 && verbose {
					sb.WriteString(fmt.Sprintf("Validating - %s\n", magenta(r.description)))
				}

				if dir != "" && verbose {
					sb.WriteString(fmt.Sprintf("\t  🔎 Searching - %s \n", cyan(dir)))
				}

				path := func() string {
					if dir == "" {
						return r.name
					} else {
						return filepath.Join(dir, r.name)
					}
				}()

				if _, err := os.Stat(path); err == nil {
					r.satisfied = true
					satisfiedCount++
					sb.WriteString(fmt.Sprintf("✅ Dependency Satisfied - %s [%s]\n", green(filepath.Join(dir, r.name)), cyan(r.description)))
					continue
				} else {
					sb.WriteString(fmt.Sprintf("⛔ Dependency Missing - %s\n", cyan(r.description)))
				}
			} else {
				// Satisfied by not being required
				r.satisfied = true
				satisfiedCount++
				continue
			}
		}
	}

	fmt.Print(sb.String())

	// for _, r := range required {
	// 	if r.satisfied {
	// 		continue
	// 	}
	//
	// 	var input []byte
	// 	var err error
	//
	// 	if !config.ForceDepInstall {
	// 		fmt.Printf("⛔ You are missing required dependencies for [%s], do you want to install? [y/N] ", red(r.descriptor))
	// 		inputReader := bufio.NewReader(os.Stdin)
	// 		input, err = inputReader.ReadSlice('\n')
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	//
	// 	if config.ForceDepInstall || strings.ToUpper(string(input)) == "Y\n" {
	// 		var path string
	// 		dir := ""
	// 		if len(r.directories) > 0 {
	// 			// we install into the first directory if specified
	// 			dir = r.directories[0]
	// 		}
	// 		for _, f := range r.files {
	// 			if dir == "" {
	// 				path = filepath.Dir(f.name)
	// 			} else {
	// 				path = dir
	// 			}
	// 			err = os.MkdirAll(path, 0755)
	// 			if err != nil {
	// 				return err
	// 			}
	//
	// 			if f.iF != nil {
	// 				err := f.iF(r, config, logger)
	// 				if err != nil {
	// 					logger.Error("Failed to run initialize function", slog.String("step", f.description), slog.Any("err", err))
	// 					return err
	// 				}
	// 			}
	// 		}
	//
	// 		for _, iF := range r.initFuncs {
	// 			err := iF(r, config, logger)
	// 			if err != nil {
	// 				logger.Error("Failed to run initialize function", slog.String("step", r.descriptor), slog.Any("err", err))
	// 				return err
	// 			}
	// 		}
	// 	}
	// }

	return nil
}

func writeCniConf(r *requirement, c *models.NodeConfiguration, logger *slog.Logger) error {
	// 	for _, tF := range r.files {
	// 		f, err := os.Create(filepath.Join(r.directories[0], tF.name))
	// 		if err != nil {
	// 			return err
	// 		}
	// 		defer f.Close()
	//
	// 		tmpl, err := template.New("fcnet_conf").
	// 			Funcs(template.FuncMap{
	// 				"AddQuotes": addQuotes,
	// 			}).
	// 			Parse(templates.FcnetConfig)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		var buffer bytes.Buffer
	// 		err = tmpl.Execute(&buffer, c.CNI)
	// 		if err != nil {
	// 			return err
	// 		}
	//
	// 		_, err = f.Write(buffer.Bytes())
	// 		if err != nil {
	// 			return nil
	// 		}
	// 	}
	//
	return nil
}

func downloadKernel(r *requirement, _ *models.NodeConfiguration, logger *slog.Logger) error {
	// 	_ = vmLinuxKernelSHA256 // TODO: implement sha verification
	// 	for _, f := range r.files {
	// 		respBin, err := http.Get(vmLinuxKernelURL)
	//
	// 		if err != nil {
	// 			return err
	// 		}
	// 		defer respBin.Body.Close()
	//
	// 		// TODO: add sha check
	//
	// 		outFile, err := os.Create(f.name)
	// 		if err != nil {
	// 			fmt.Println(err)
	// 			return err
	// 		}
	// 		_, err = io.Copy(outFile, respBin.Body)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		outFile.Close()
	// 	}
	//
	return nil
}

func downloadFirecracker(_ *requirement, _ *models.NodeConfiguration, logger *slog.Logger) error {
	// 	_ = firecrackerTarballSHA256
	// 	// TODO: firecracker repo made the sha difficult to use
	// 	rawData, err := decompressTarFromURL(firecrackerTarballURL, "")
	// 	if err != nil {
	// 		return err
	// 	}
	//
	// 	for {
	// 		header, err := rawData.Next()
	// 		if err == io.EOF {
	// 			break
	// 		}
	// 		if err != nil {
	// 			return err
	// 		}
	//
	// 		if header.Name == "release-v1.5.0-x86_64/firecracker-v1.5.0-x86_64" || header.Name == "release-v1.5.0-aarch64/firecracker-v1.5.0-aarch64" {
	// 			outFile, err := os.Create(filepath.Join(r.directories[0], "firecracker"))
	// 			if err != nil {
	// 				fmt.Println(err)
	// 				return err
	// 			}
	// 			_, err = io.Copy(outFile, rawData)
	// 			if err != nil {
	// 				fmt.Println(err)
	// 				return err
	// 			}
	// 			outFile.Close()
	//
	// 			err = os.Chmod(outFile.Name(), 0755)
	// 			if err != nil {
	// 				fmt.Println(err)
	// 				return err
	// 			}
	// 		}
	//
	// 	}
	return nil
}

func downloadCNIPlugins(_ *requirement, c *models.NodeConfiguration, logger *slog.Logger) error {
	// 	rawData, err := decompressTarFromURL(cniPluginsTarballURL, cniPluginsTarballSHA256)
	// 	if err != nil {
	// 		return err
	// 	}
	//
	// 	for {
	// 		header, err := rawData.Next()
	// 		if err == io.EOF {
	// 			break
	// 		}
	// 		if err != nil {
	// 			return err
	// 		}
	//
	// 		f := strings.TrimPrefix(strings.TrimSpace(header.Name), "./")
	//
	// 		if f == "ptp" || f == "host-local" {
	// 			outFile, err := os.Create(filepath.Join(r.directories[0], f))
	// 			if err != nil {
	// 				fmt.Println(err)
	// 				return err
	// 			}
	// 			_, err = io.Copy(outFile, rawData)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			outFile.Close()
	//
	// 			err = os.Chmod(outFile.Name(), 0755)
	// 			if err != nil {
	// 				return err
	// 			}
	// 		}
	// 	}
	//
	return nil
}

func downloadTCRedirectTap(r *requirement, _ *models.NodeConfiguration, logger *slog.Logger) error {
	// 	_ = tcRedirectCNIPluginSHA256
	// 	respBin, err := http.Get(tcRedirectCNIPluginURL)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	defer respBin.Body.Close()
	//
	// 	// TODO: add sha check
	//
	// 	outFile, err := os.Create(filepath.Join(r.directories[0], "tc-redirect-tap"))
	// 	if err != nil {
	// 		fmt.Println(err)
	// 		return err
	// 	}
	// 	_, err = io.Copy(outFile, respBin.Body)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	outFile.Close()
	//
	// 	err = os.Chmod(outFile.Name(), 0755)
	// 	if err != nil {
	// 		return err
	// 	}
	return nil
}

func downloadRootFS(_ *requirement, _ *models.NodeConfiguration, logger *slog.Logger) error {
	// 	_ = fmt.Sprintf(rootfsGzipSHA256Template, nexLatestVersion)
	// 	for _, f := range r.files {
	//
	// 		respTar, err := http.Get(fmt.Sprintf(rootfsGzipURLTemplate, nexLatestVersion))
	// 		if err != nil {
	// 			return err
	// 		}
	// 		defer respTar.Body.Close()
	// 		if respTar.StatusCode != 200 {
	// 			return fmt.Errorf("failed to download rootfs. Response Code: %s", respTar.Status)
	// 		}
	//
	// 		uncompressedFile, err := gzip.NewReader(respTar.Body)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		outFile, err := os.Create(f.name)
	// 		if err != nil {
	// 			fmt.Println(err)
	// 			return err
	// 		}
	// 		_, err = io.Copy(outFile, uncompressedFile)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		outFile.Close()
	// 	}
	return nil
}

//	func decompressTarFromURL(url string, _ string) (*tar.Reader, error) {
//		respTar, err := http.Get(url)
//		if err != nil {
//			return nil, err
//		}
//
//		uncompressedTar, err := gzip.NewReader(respTar.Body)
//		if err != nil {
//			return nil, err
//		}
//
//		rawData := tar.NewReader(uncompressedTar)
//		return rawData, nil
//	}
//
//	func addQuotes(urls []string) string {
//		ret := strings.Builder{}
//		ret.WriteRune('[')
//		for i, url := range urls {
//			ret.WriteString(fmt.Sprintf("\"%s\"", url))
//			if i < len(urls)-1 {
//				ret.WriteRune(',')
//			}
//		}
//		ret.WriteRune(']')
//		return ret.String()
//	}
func installNexAgent(r *requirement, _ *models.NodeConfiguration, logger *slog.Logger) error {
	// var errs PreflightError
	//
	// respBin, err := http.Get(fmt.Sprintf(nexAgentDarwinTemplate, nexLatestVersion, nexLatestVersion))
	//
	//	if err != nil || respBin.StatusCode != 200 {
	//		errs = errors.Join(errs, ErrFailedToDownloadNexAgent)
	//	}
	//
	// defer respBin.Body.Close()
	//
	// nexAgentBinary, err := io.ReadAll(respBin.Body)
	//
	//	if err != nil {
	//		logger.Debug("failed to download nex-agent", slog.Any("err", err))
	//		errs = errors.Join(errs, err)
	//	}
	//
	// respSha, err := http.Get(fmt.Sprintf(nexAgentDarwinURLTemplateSHA256, nexLatestVersion, nexLatestVersion))
	//
	//	if err != nil || respSha.StatusCode != 200 {
	//		errs = errors.Join(errs, ErrFailedToDownloadNexAgentSha)
	//	}
	//
	// defer respSha.Body.Close()
	//
	// nexAgentBinarySha, err := io.ReadAll(respSha.Body)
	//
	//	if err != nil {
	//		logger.Debug("failed to download nex-agent sha", slog.Any("err", err))
	//		errs = errors.Join(errs, err)
	//	}
	//
	// nexAgentPath := filepath.Join(r.directories[0], "nex-agent")
	// nexAgent, err := os.Create(nexAgentPath + ".new")
	//
	//	if err != nil {
	//		logger.Debug("failed to create temp nex-agent file", slog.String("path", nexAgentPath+".new"))
	//		errs = errors.Join(errs, ErrFailedToCreateTempFile)
	//	}
	//
	// defer os.Remove(nexAgentPath + ".new")
	//
	// _, err = nexAgent.Write(nexAgentBinary)
	//
	//	if err != nil {
	//		logger.Debug("failed to write temp nex-agent file", slog.String("path", nexAgentPath+".new"))
	//		errs = errors.Join(errs, ErrFailedToWriteTempFile)
	//	}
	//
	// h := sha256.New()
	//
	//	if _, err := io.Copy(h, nexAgent); err != nil {
	//		logger.Debug("failed to calculate sha256", slog.Any("err", err))
	//		errs = errors.Join(errs, ErrFailedToCalculateSha256)
	//	}
	//
	// shasum := hex.EncodeToString(h.Sum(nil))
	//
	//	if shasum != string(nexAgentBinarySha) {
	//		logger.Debug("sha256 mismatch", slog.String("expected", string(nexAgentBinarySha)), slog.String("actual", shasum))
	//		errs = errors.Join(errs, ErrSha256Mismatch)
	//	} else {
	//
	//		err = os.Rename(nexAgentPath+".new", nexAgentPath)
	//		if err != nil {
	//			logger.Debug("failed to rename nex-agent", slog.String("from", nexAgentPath+".new"), slog.String("to", nexAgentPath))
	//			errs = errors.Join(errs, err)
	//		}
	//
	//		err = os.Chmod(nexAgentPath, 0755)
	//		if err != nil {
	//			logger.Debug("Failed to update nex binary permissions", slog.Any("err", err))
	//			errs = errors.Join(errs, err)
	//		}
	//	}
	//
	// return errs
	return nil
}
