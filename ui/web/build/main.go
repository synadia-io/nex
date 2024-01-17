package main

import (
	"context"
	"fmt"
	"os"

	"dagger.io/dagger"
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
