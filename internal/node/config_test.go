package nexnode

import (
	"testing"
)

func TestNodeConfigResolution(t *testing.T) {
	config1, err := LoadNodeConfiguration("../../examples/nodeconfigs/simple.json")
	if err != nil {
		t.Fatalf("couldn't load node config example: %s", err)
	}
	if config1.HostServicesConfig == nil {
		t.Fatal("host services configuration should not default to nil")
	}
	if config1.HostServicesConfig.Services["http"].Enabled == false {
		t.Fatal("All of the builtin host services should default to enabled")
	}

	config2, err := LoadNodeConfiguration("../../examples/nodeconfigs/hostservices_config.json")
	if err != nil {
		t.Fatalf("couldn't load node config example: %s", err)
	}
	if config2.HostServicesConfig == nil {
		t.Fatal("host services configuration should not be nil when explicitly defined")
	}
	if config2.HostServicesConfig.Services["http"].Enabled {
		t.Fatal("in custom config http service should be disabled")
	}
}
