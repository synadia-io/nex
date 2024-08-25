package rootfs

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"dagger.io/dagger"
)

func Build(outname, buildScript, baseImg, agentPath string, plugins []string, fsSize int) error {
	if os.Getuid() != 0 {
		return errors.New("Please run as root")
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

	var bS *os.File
	if buildScript != "" {
		var err error
		bS, err = os.Open(buildScript)
		if err != nil {
			return err
		}
		bS_r, err := io.ReadAll(bS)
		if err != nil {
			return nil
		}
		err = os.WriteFile(filepath.Join(tempdir, "buildscript.sh"), bS_r, 0644)
		if err != nil {
			return err
		}
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

	for _, runnerPlugin := range plugins {
		input, err := os.ReadFile(runnerPlugin)
		if err != nil {
			return err
		}

		err = os.WriteFile(filepath.Join(tempdir, filepath.Base(runnerPlugin)), input, 0644)
		if err != nil {
			return err
		}
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

	return build(context.Background(), tempdir, mountPoint, baseImg, outname, plugins, bS != nil)
}

func build(ctx context.Context, tempdir, mountPoint, baseImg, outname string, plugins []string, withBuildScript bool) error {
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

	pluginFiles := make([]*dagger.File, 0)
	for _, pluginFile := range plugins {
		pluginFiles = append(pluginFiles, client.Host().File(filepath.Base(pluginFile)))
	}

	var c *dagger.Container
	c = client.Container(
		dagger.ContainerOpts{
			Platform: dagger.Platform(runtime.GOOS + "/" + runtime.GOARCH),
		},
	).From(baseImg).
		WithEnvVariable("CACHEBUSTER", time.Now().String()).
		WithUser("root").
		WithDirectory("/tmp/rootfs", rootfs).
		WithMountedFile("/usr/local/bin/agent", nexagent)
	for idx, pf := range pluginFiles {
		fn := filepath.Base(plugins[idx])
		c = c.WithMountedFile(fmt.Sprintf("/usr/local/bin/%s", fn), pf)
	}

	if !withBuildScript {
		c = c.
			WithFile("/copy_fs.sh", copyFsScript).
			WithExec([]string{"sh", "/copy_fs.sh"}).
			WithExec([]string{"chown", "1000:1000", "/etc/init.d/agent"}).
			WithExec([]string{"chown", "-R", "1000:1000", "/home/nex"}).
			WithExec([]string{"chown", "1000:1000", "/usr/local/bin/agent"})

	} else {
		buildScript := client.Host().File("buildscript.sh")
		c = c.
			WithFile("/buildscript.sh", buildScript).
			WithExec([]string{"sh", "/buildscript.sh"}).
			WithFile("/copy_fs.sh", copyFsScript).
			WithExec([]string{"sh", "/copy_fs.sh"}).
			WithExec([]string{"chown", "1000:1000", "/etc/init.d/agent"}).
			WithExec([]string{"chown", "-R", "1000:1000", "/home/nex"}).
			WithExec([]string{"chown", "1000:1000", "/usr/local/bin/agent"})

	}

	_, err = c.Directory("/tmp/rootfs").
		Export(ctx, "./rootfs-mount")
	if err != nil {
		return err
	}

	err = os.Chmod(filepath.Join(mountPoint, "/usr/local/bin/agent"), 0775)
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

	err = os.Remove(filepath.Join(mountPoint, "/etc/resolv.conf"))
	if err != nil {
		return err
	}

	err = os.Symlink("/proc/net/pnp", filepath.Join(mountPoint, "/etc/resolv.conf"))
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

	rfs, err := os.Create(outname)
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
