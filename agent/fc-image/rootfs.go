package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"dagger.io/dagger"
)

func main() {
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
	//defer os.RemoveAll(tempdir)

	fmt.Println(tempdir)

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

	input, err := os.ReadFile("../nex-agent/cmd/nex-agent/nex-agent")
	if err != nil {
		fmt.Println("+", err)
		return
	}

	err = os.WriteFile(filepath.Join(tempdir, "nex-agent"), input, 0644)
	if err != nil {
		fmt.Println("++", err)
		return
	}

	fs, err := os.Create(filepath.Join(tempdir, "rootfs.ext4"))
	if err != nil {
		fmt.Println("+++", err)
		return
	}
	defer fs.Close()

	err = fs.Truncate(1 * 1024 * 1024 * 100)
	if err != nil {
		fmt.Println(err)
		return
	}

	_, err = exec.Command(mkfsext4, filepath.Join(tempdir, "rootfs.ext4")).
		CombinedOutput()
	if err != nil {
		fmt.Println(err)
		return
	}

	err = os.MkdirAll(filepath.Join(tempdir, "rootfs-mount"), 0777)
	if err != nil {
		fmt.Println(err)
		return
	}

	//syscall.MS_NOATIME|syscall.MS_SILENT|syscall.MS_SYNCHRONOUS|syscall.MS_DIRSYNC,
	err = syscall.Mount(
		filepath.Join(tempdir, "rootfs-mount"),
		filepath.Join(tempdir, "rootfs.ext4"),
		"ext4",
		syscall.MS_NOATIME|syscall.MS_SILENT,
		"journal_checksum,journal_ioprio=0,barrier=1,data=ordered,errors=remount-ro",
	)
	if err != nil {
		fmt.Println("+++++", filepath.Join(tempdir, "rootfs.ext4"), filepath.Join(tempdir, "rootfs-mount"), err)
		return
	}
	defer func() {
		err := syscall.Unmount(filepath.Join(tempdir, "rootfs-mount"), 0)
		if err != nil {
			fmt.Println(err)
			return
		}
	}()

	if err := build(context.Background(), tempdir); err != nil {
		fmt.Println(err)
	}
}

func build(ctx context.Context, tempdir string) error {
	fmt.Println("asdf")
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		return err
	}
	defer client.Close()

	fmt.Println("asdf")
	orcFile := client.Host().File(filepath.Join(tempdir, "openrc-service.sh"))
	nexagent := client.Host().File(filepath.Join(tempdir, "nex-agent"))
	rootfs := client.Host().Directory(filepath.Join(tempdir, "rootfs.ext4"))
	fmt.Println("asdf")
	_ = client.Container().
		From("alpine:latest").
		WithMountedFile("/etc/init.d/agent", orcFile).
		WithMountedFile("/usr/local/bin/agent", nexagent).
		WithMountedDirectory("/tmp/rootfs", rootfs).
		WithExec([]string{"alpine", "sh", "<", "setup-alpine.sh"})

	fmt.Println("asdf")
	input, err := os.ReadFile(filepath.Join(tempdir, "rootfs.ext4"))
	if err != nil {
		return err
	}
	err = os.WriteFile("rootfs.ext4", input, 0644)
	if err != nil {
		return err
	}

	return nil
}
