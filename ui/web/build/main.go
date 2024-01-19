package main

import (
	"context"
	"fmt"
	"os"

	"dagger.io/dagger"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

func main() {
	if len(os.Args) < 1 {
		fmt.Println("Please include web root directory")
		return
	}

	if err := build(context.Background()); err != nil {
		fmt.Println(err)
	}
}

func build(ctx context.Context) error {
	client, err := dagger.Connect(
		ctx,
		dagger.WithLogOutput(os.Stderr),
		dagger.WithWorkdir(os.Args[1]),
	)
	if err != nil {
		return err
	}
	defer client.Close()
	defer stopDaggerEngine()

	webroot := client.Host().Directory(".")

	_, err = client.Container().
		From("node:20-slim").
		WithDirectory("/webroot", webroot).
		WithWorkdir("/webroot").
		WithExec([]string{"corepack", "enable"}).
		WithExec([]string{"pnpm", "install"}).
		WithExec([]string{"pnpm", "build"}).
		Directory("./dist").
		Export(ctx, "./dist")

	if err != nil {
		return err
	}

	return nil
}

func stopDaggerEngine() {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return
	}

	ctx := context.Background()

	containers, _ := cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs([]filters.KeyValuePair{
			{
				Key:   "name",
				Value: "dagger-engine",
			},
		}...),
	})

	for _, c := range containers {
		_ = cli.ContainerStop(ctx, c.ID, container.StopOptions{})
	}
}
