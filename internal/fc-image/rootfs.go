package rootfs

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"dagger.io/dagger"
)

const defaultRootFsSize = 1024 * 1024 * 150 // 150 MiB

func Build(buildScript, baseImg, agentPath string) error {
	if os.Getuid() != 0 {
		return errors.New("Please run as root")
	}
	mkfsext4, err := exec.LookPath("mkfs.ext4")
	if err != nil {
		return err
	}

	tempdir, err := os.MkdirTemp(os.TempDir(), "dagger-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempdir)

	err = os.WriteFile(filepath.Join(tempdir, "openrc-service.sh"), []byte(openrc_service), 0644)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(tempdir, "setup-alpine.sh"), []byte(setup_alpine), 0644)
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

	err = fs.Truncate(defaultRootFsSize)
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

	orcFile := client.Host().File("openrc-service.sh")
	bootstrapScript := client.Host().File("setup-alpine.sh")
	nexagent := client.Host().File("nex-agent")
	rootfs := client.Host().Directory("rootfs-mount")

	c := client.Container().
		From(baseImg).
		WithEnvVariable("CACHEBUSTER", time.Now().String()).
		WithUser("root").
		WithDirectory("/tmp/rootfs", rootfs).
		WithMountedFile("/etc/init.d/agent", orcFile).
		WithMountedFile("/usr/local/bin/agent", nexagent).
		WithMountedFile("/setup-alpine.sh", bootstrapScript).
		WithExec([]string{"sh", "/setup-alpine.sh"}).
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
