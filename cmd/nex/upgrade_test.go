package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func setEnvironment(t *testing.T) {
	t.Helper()

	VERSION = "9.9.9"
	COMMIT = "abcdefg12345678test"
	BUILDDATE = "2021-01-01T00:00:00Z"
}

func TestUpdateNex(t *testing.T) {
	setEnvironment(t)
	nexPath, _ := os.Executable()
	if !strings.HasPrefix(nexPath, os.TempDir()) {
		t.Log("bailing on update nex test so real env isnt affected")
		t.SkipNow()
	}

	globals := new(Globals)
	u := Upgrade{GitTag: "0.2.7"}
	err := u.Run(context.Background(), globals)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(nexPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	buf, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	// https://github.com/synadia-io/nex/releases/download/0.2.7/nex_0.2.7_linux_amd64.sha256
	expectedSha := "143753466f83f744ccdc5dbe7e76e9539a2a5db2130cd8cb44607b55e414ee58"
	s256 := sha256.Sum256(buf)
	sSum := fmt.Sprintf("%x", s256)
	if sSum != expectedSha {
		t.Fatalf("Expected sha256 to be %s; Got %s", expectedSha, sSum)
	}
}
