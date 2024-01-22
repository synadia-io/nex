package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"dagger.io/dagger"
)

func main() {
	if os.Getuid() != 0 {
		fmt.Println("Please run as root")
		return
	}

	if len(os.Args) < 1 {
		fmt.Println("Please provide path the agent binary")
		return
	}

	mkfsext4, err := exec.LookPath("mkfs.ext4")
	if err != nil {
		fmt.Println(err)
		return
	}

	tempdir, err := os.MkdirTemp(os.TempDir(), "dagger-*")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(tempdir)

	err = os.WriteFile(filepath.Join(tempdir, "openrc-service.sh"), []byte(openrc_service), 0644)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = os.WriteFile(filepath.Join(tempdir, "setup-alpine.sh"), []byte(setup_alpine), 0644)
	if err != nil {
		fmt.Println(err)
		return
	}

	input, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		return
	}

	err = os.WriteFile(filepath.Join(tempdir, "nex-agent"), input, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}

	fs, err := os.Create(filepath.Join(tempdir, "rootfs.ext4"))
	if err != nil {
		fmt.Println(err)
		return
	}

	err = os.Chmod(filepath.Join(tempdir, "rootfs.ext4"), 0777)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = fs.Truncate(1 * 1024 * 1024 * 100)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = fs.Close()
	if err != nil {
		fmt.Println(err)
		return
	}

	cmd := exec.Command(mkfsext4, filepath.Join(tempdir, "rootfs.ext4"))
	output, err := cmd.Output()
	if err != nil {
		fmt.Println(string(output), err)
		return
	}

	err = os.MkdirAll(filepath.Join(tempdir, "rootfs-mount"), 0777)
	if err != nil {
		fmt.Println(err)
		return
	}

	device := filepath.Join(tempdir, "rootfs.ext4")
	mountPoint := filepath.Join(tempdir, "rootfs-mount")

	cmd = exec.Command("mount", device, mountPoint)
	output, err = cmd.Output()
	if err != nil {
		fmt.Println(string(output), err)
		return
	}

	if err := build(context.Background(), tempdir, mountPoint); err != nil {
		fmt.Println(err)
	}
}

func build(ctx context.Context, tempdir string, mountPoint string) error {
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
		From("alpine:latest").
		WithEnvVariable("CACHEBUSTER", time.Now().String()).
		WithUser("root").
		WithDirectory("/tmp/rootfs", rootfs).
		WithMountedFile("/etc/init.d/agent", orcFile).
		WithMountedFile("/usr/local/bin/agent", nexagent).
		WithMountedFile("/setup-alpine.sh", bootstrapScript).
		WithExec([]string{"sh", "/setup-alpine.sh"})

	_, err = c.Directory("/tmp/rootfs").
		Export(ctx, "./rootfs-mount")
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
