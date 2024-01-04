package nexcli

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/choria-io/fisk"
	"github.com/masahiro331/go-ext4-filesystem/ext4"
)

func RunExtract(ctx *fisk.ParseContext) error {
	f, err := os.Open(ExtractOpts.Filename)
	if err != nil {
		log.Fatal(err)
	}
	info, _ := f.Stat()
	filesystem, err := ext4.NewFS(*io.NewSectionReader(f, 0, info.Size()), nil)
	if err != nil {
		log.Fatal(err)
	}

	files := []string{
		"/home/nex/nex.log",
		"/home/nex/err.log",
		"/home/nex/agent.log",
	}
	for _, file := range files {
		err = extractFile(filesystem, file)
		if err != nil {
			return err
		}
	}

	return nil
}

func extractFile(filesystem *ext4.FileSystem, filename string) error {
	f, err := filesystem.Open(filename)
	if err != nil {
		fmt.Printf("File %s not found in rootfs\n", filename)
		return nil
	}
	loginfo, _ := f.Stat()
	if loginfo.Size() == 0 {
		fmt.Printf("%s is empty, skipping\n", loginfo.Name())
	} else {
		logBytes := make([]byte, loginfo.Size())
		count, err := f.Read(logBytes)
		if err != nil {
			fmt.Println("Failed to read file bytes")
			return err
		}
		if int64(count) != loginfo.Size() {
			e := fmt.Sprintf("Expected to read %d bytes, read %d", loginfo.Size(), count)
			return errors.New(e)
		}
		err = os.WriteFile(fmt.Sprintf("./%s", loginfo.Name()), logBytes, 0644)
		if err != nil {
			return err
		}
		fmt.Printf("Extracted %s (%d bytes)\n", loginfo.Name(), loginfo.Size())
	}
	return nil

}
