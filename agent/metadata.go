package nexagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

// MmdsAddress is the address used by the agent to query firecracker MMDS
// (see https://github.com/firecracker-microvm/firecracker/blob/main/docs/mmds/mmds-user-guide.md#version-2)
const MmdsAddress = "169.254.169.254"

const nexEnvSandbox = "NEX_SANDBOX"
const nexEnvVmID = "NEX_VMID"
const nexEnvNodeNatsHost = "NEX_NODENATSHOST"
const nexEnvNodeNatsPort = "NEX_NODENATSPORT"

const metadataClientTimeoutMillis = 50
const metadataPollingTimeoutMillis = 5000

// GetMachineMetadata attempts to retrieve metadata from firecracker's MMDS.
// Version of 2 this service requires the acuisition of a token and the use
// of that token for all requests. Note that metadata is PUT into a running
// machine AFTER it starts. So if we have things that auto start (like this
// agent), then we need to ensure we avoid the race condition of reading
// metadata before it exists.
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

	client := &http.Client{
		Timeout: metadataClientTimeoutMillis * time.Millisecond,
	}

	timeoutAt := time.Now().UTC().Add(metadataPollingTimeoutMillis * time.Millisecond)

	for {
		metadata, err := performMetadataQuery(req, client)
		if err != nil {
			if time.Now().UTC().After(timeoutAt) {
				break
			}

			continue
		}

		return metadata, nil
	}

	return nil, fmt.Errorf("failed to obtain metadata after %dms", metadataPollingTimeoutMillis)
}

func GetMachineDataFromEnv() (*agentapi.MachineMetadata, error) {
	vmid := os.Getenv(nexEnvVmID)
	host := os.Getenv(nexEnvNodeNatsHost)
	port := os.Getenv(nexEnvNodeNatsPort)
	msg := "Metadata obtained from no-sandbox environment"
	p, err := strconv.Atoi(port)
	if err != nil {
		return nil, err
	}

	return &agentapi.MachineMetadata{
		VmID:         &vmid,
		NodeNatsHost: &host,
		NodeNatsPort: &p,
		Message:      &msg,
	}, nil
}

func performMetadataQuery(req *http.Request, client *http.Client) (*agentapi.MachineMetadata, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New("metadata not found")
	}

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

	client := &http.Client{
		Timeout: 1 * time.Second,
	}

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
