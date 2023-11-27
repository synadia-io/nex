package nexagent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

// https://github.com/firecracker-microvm/firecracker/blob/main/docs/mmds/mmds-user-guide.md#version-2

const (
	MmdsAddress = "169.254.169.254"
)

// Attempts to retrieve metadata from firecracker's MMDS. Version of 2 this service requires the acuisition of
// a token and the use of that token for all requests. Note that metadata is PUT into a running machine AFTER
// it starts. So if we have things that auto start (like this agent), then we need to ensure we avoid the race
// condition of reading metadata before it exists.
func GetMachineMetadata() (*agentapi.MachineMetadata, error) {
	token, err := acquireToken()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://%s/", MmdsAddress)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-metadata-token", token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	var metadata agentapi.MachineMetadata
	err = json.Unmarshal(bodyBytes, &metadata)
	if err != nil {
		return nil, fmt.Errorf("deserialization failure: %s: body: '%s'", err, string(bodyBytes))
	}

	return &metadata, nil
}

func acquireToken() (string, error) {
	url := fmt.Sprintf("http://%s/latest/api/token", MmdsAddress)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("X-metadata-token-ttl-seconds", "60")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}
