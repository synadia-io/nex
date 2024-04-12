package rootfs

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"dagger.io/dagger"
)

func Build(buildScript, baseImg, agentPath string, fsSize int, systemd bool) error {
	if os.Getuid() != 0 {
		return errors.New("Please run as root")
	}

	if baseImg == "" {
		switch runtime.GOARCH {
		case "amd64":
			baseImg = "ghcr.io/synadia-io/nex/nex_alpine:latest"
		case "arm64":
			baseImg = "ghcr.io/synadia-io/nex/nex_debian:latest"
		default:
			return errors.New("please provide a base image")
		}
	}

	mkfsext4, err := exec.LookPath("mkfs.ext4")
	if err != nil {
		return errors.New("'mkfs.ext4' not found in $PATH: " + err.Error())
	}

	tempdir, err := os.MkdirTemp(os.TempDir(), "dagger-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempdir)

	// determine if using openrc or systemd
	serviceFile := "openrc-service.sh"
	serviceContent := openrc_service
	if systemd {
		serviceFile = "agent.service"
		serviceContent = systemd_service
	}
	err = os.WriteFile(filepath.Join(tempdir, serviceFile), []byte(serviceContent), 0644)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(tempdir, "copy_fs.sh"), []byte(copy_fs), 0644)
	if err != nil {
		return err
	}

	input, err := os.ReadFile(agentPath)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(tempdir, "nex-agent"), input, 0644)
	if err != nil {
		return err
	}

	fs, err := os.Create(filepath.Join(tempdir, "rootfs.ext4"))
	if err != nil {
		return err
	}

	err = os.Chmod(filepath.Join(tempdir, "rootfs.ext4"), 0777)
	if err != nil {
		return err
	}

	err = fs.Truncate(int64(fsSize))
	if err != nil {
		return err
	}

	err = fs.Close()
	if err != nil {
		return err
	}

	cmd := exec.Command(mkfsext4, filepath.Join(tempdir, "rootfs.ext4"))
	_, err = cmd.Output()
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(tempdir, "rootfs-mount"), 0777)
	if err != nil {
		return err
	}

	device := filepath.Join(tempdir, "rootfs.ext4")
	mountPoint := filepath.Join(tempdir, "rootfs-mount")

	cmd = exec.Command("mount", device, mountPoint)
	output, err := cmd.Output()
	if err != nil {
		return errors.New(string(output) + "\n\n" + err.Error())
	}

	return build(context.Background(), tempdir, mountPoint, baseImg)
}

func build(ctx context.Context, tempdir, mountPoint, baseImg string) error {
	client, err := dagger.Connect(ctx,
		dagger.WithLogOutput(os.Stderr),
		dagger.WithWorkdir(tempdir),
	)
	if err != nil {
		return err
	}
	defer client.Close()

	copyFsScript := client.Host().File("copy_fs.sh")
	nexagent := client.Host().File("nex-agent")
	rootfs := client.Host().Directory("rootfs-mount")

	c := client.Container(
		dagger.ContainerOpts{Platform: dagger.Platform(runtime.GOOS + "/" + runtime.GOARCH)},
	).
		From(baseImg).
		WithEnvVariable("CACHEBUSTER", time.Now().String()).
		WithUser("root").
		WithFile("/copy_fs.sh", copyFsScript).
		WithDirectory("/tmp/rootfs", rootfs).
		WithMountedFile("/usr/local/bin/agent", nexagent).
		WithExec([]string{"sh", "/copy_fs.sh"}).
		WithExec([]string{"chown", "1000:1000", "/etc/init.d/agent"}).
		WithExec([]string{"chown", "-R", "1000:1000", "/home/nex"}).
		WithExec([]string{"chown", "1000:1000", "/usr/local/bin/agent"})

	_, err = c.Directory("/tmp/rootfs").
		Export(ctx, "./rootfs-mount")
	if err != nil {
		return err
	}

	err = os.Chown(filepath.Join(mountPoint, "/home/nex"), 1000, 1000)
	if err != nil {
		return err
	}
	err = os.Chown(filepath.Join(mountPoint, "/etc/init.d/agent"), 1000, 1000)
	if err != nil {
		return err
	}
	err = os.Chown(filepath.Join(mountPoint, "/usr/local/bin/agent"), 1000, 1000)
	if err != nil {
		return err
	}

	_, err = c.Stdout(ctx)
	if err != nil {
		return err
	}
	_, err = c.Stderr(ctx)
	if err != nil {
		return err
	}

	cmd := exec.Command("umount", mountPoint)
	output, err := cmd.Output()
	if err != nil {
		fmt.Println(string(output), err)
		return err
	}

	input, err := os.ReadFile(filepath.Join(tempdir, "rootfs.ext4"))
	if err != nil {
		return err
	}

	rfs, err := os.Create("rootfs.ext4.gz")
	if err != nil {
		return err
	}
	defer rfs.Close()

	gw := gzip.NewWriter(rfs)
	defer gw.Close()

	_, err = gw.Write(input)
	if err != nil {
		return err
	}

	return nil
}
