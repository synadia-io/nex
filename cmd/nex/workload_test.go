package main

import (
	"os"
	"path/filepath"
	"testing"
)

func initWorkloadCommand(t testing.TB) (string, error) {
	t.Helper()

	confDir := t.TempDir()
	f, err := os.Create(filepath.Join(confDir, "publisher.xk"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	f, err = os.Create(filepath.Join(confDir, "issuer.nk"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	wl, err := os.Create(filepath.Join(confDir, "workload"))
	if err != nil {
		return "", err
	}
	defer wl.Close()
	_, err = wl.Write([]byte("test"))
	if err != nil {
		return "", err
	}

	return confDir, nil
}

func TestStartWorkload(t *testing.T) {
	confDir, err := initWorkloadCommand(t)
	if err != nil {
		t.Fatal(err)
	}

	w := Workload{
		Run: RunWorkload{
			Devrun:       false,
			Name:         "test",
			TargetId:     "TESTNODE",
			File:         filepath.Join(confDir, "workload"),
			Xkey:         filepath.Join(confDir, "publisher.xk"),
			IssuerKey:    filepath.Join(confDir, "issuer.nk"),
			WorkloadType: "native",
		},
	}

	err = w.Run.AfterApply()
	if err != nil {
		t.Fatal(err)
	}

	if w.Run.Argv == nil || w.Run.Env == nil || w.Run.TriggerSubjects == nil {
		t.Fatal("AfterApply() failed to initialize fields in Run structure")
	}

	if err = w.Run.Validate(); err != nil {
		t.Fatalf("Run command failed to validate: %s", err)
	}

}

func TestStopWorkload(t *testing.T) {
	confDir, err := initWorkloadCommand(t)
	if err != nil {
		t.Fatal(err)
	}

	w := Workload{
		Stop: StopWorkload{
			Devrun:     false,
			WorkloadId: "somerandomid",
			TargetId:   "TESTNODE",
			IssuerKey:  filepath.Join(confDir, "issuer.nk"),
		},
	}

	err = w.Stop.Validate()
	if err != nil {
		t.Fatalf("Stop command failed to validate: %s", err)
	}
}
