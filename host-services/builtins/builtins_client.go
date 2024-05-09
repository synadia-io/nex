package builtins

import (
	"encoding/json"
	"errors"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
)

type BuiltinServicesClient struct {
	hsClient *hostservices.HostServicesClient
}

const (
	builtinServiceNameKeyValue    = "kv"
	builtinServiceNameHttpClient  = "http"
	builtinServiceNameMessaging   = "messaging"
	builtinServiceNameObjectStore = "objectstore"
)

func NewBuiltinServicesClient(hsClient *hostservices.HostServicesClient) *BuiltinServicesClient {
	return &BuiltinServicesClient{
		hsClient: hsClient,
	}
}

func (c *BuiltinServicesClient) KVGet(key string) ([]byte, error) {
	metadata := map[string]string{
		agentapi.KeyValueKeyHeader: key,
	}

	resp, err := c.hsClient.PerformRpc(builtinServiceNameKeyValue, kvServiceMethodGet, []byte{}, metadata)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, resp.Error()
	}

	return resp.Data, nil
}

func (c *BuiltinServicesClient) KVSet(key string, value []byte) (*agentapi.HostServicesKeyValueResponse, error) {
	metadata := map[string]string{
		agentapi.KeyValueKeyHeader: key,
	}

	resp, err := c.hsClient.PerformRpc(builtinServiceNameKeyValue, kvServiceMethodSet, value, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	var kvResponse agentapi.HostServicesKeyValueResponse
	err = json.Unmarshal(resp.Data, &kvResponse)
	if err != nil {
		return nil, err
	}

	return &kvResponse, nil
}

func (c *BuiltinServicesClient) KVDelete(key string) (*agentapi.HostServicesKeyValueResponse, error) {
	metadata := map[string]string{
		agentapi.KeyValueKeyHeader: key,
	}

	resp, err := c.hsClient.PerformRpc(builtinServiceNameKeyValue, kvServiceMethodDelete, []byte{}, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	var kvResponse agentapi.HostServicesKeyValueResponse
	err = json.Unmarshal(resp.Data, &kvResponse)
	if err != nil {
		return nil, err
	}

	return &kvResponse, nil
}

func (c *BuiltinServicesClient) KVKeys() ([]string, error) {
	resp, err := c.hsClient.PerformRpc(builtinServiceNameKeyValue, kvServiceMethodKeys, []byte{}, make(map[string]string))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}
	var results []string
	err = json.Unmarshal(resp.Data, &results)
	if err != nil {
		return nil, err
	}

	return results, err
}

func (c *BuiltinServicesClient) MessagingPublish(subject string, payload []byte) error {
	metadata := map[string]string{
		agentapi.MessagingSubjectHeader: subject,
	}
	resp, err := c.hsClient.PerformRpc(builtinServiceNameMessaging, messagingServiceMethodPublish, payload, metadata)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return resp.Error()
	}

	var response agentapi.HostServicesMessagingResponse
	err = json.Unmarshal(resp.Data, &response)
	if err != nil {
		return err
	}

	if !response.Success {
		es := make([]error, 0)
		for _, e := range response.Errors {
			es = append(es, errors.New(e))
		}
		return errors.Join(es...)
	}

	return nil
}

func (c *BuiltinServicesClient) MessagingRequest(subject string, payload []byte) ([]byte, error) {
	metadata := map[string]string{
		agentapi.MessagingSubjectHeader: subject,
	}
	resp, err := c.hsClient.PerformRpc(builtinServiceNameMessaging, messagingServiceMethodRequest, payload, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	return resp.Data, nil
}

func (c *BuiltinServicesClient) ObjectGet(objectName string) ([]byte, error) {
	metadata := map[string]string{
		agentapi.ObjectStoreObjectNameHeader: objectName,
	}
	resp, err := c.hsClient.PerformRpc(builtinServiceNameObjectStore, objectStoreServiceMethodGet, []byte{}, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	return resp.Data, nil
}

func (c *BuiltinServicesClient) ObjectList() ([]*nats.ObjectInfo, error) {

	resp, err := c.hsClient.PerformRpc(builtinServiceNameObjectStore, objectStoreServiceMethodList, []byte{}, make(map[string]string))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	var theList []*nats.ObjectInfo
	err = json.Unmarshal(resp.Data, &theList)
	if err != nil {
		return nil, err
	}

	return theList, nil
}

func (c *BuiltinServicesClient) ObjectPut(objectName string, payload []byte) (*nats.ObjectInfo, error) {
	metadata := map[string]string{
		agentapi.ObjectStoreObjectNameHeader: objectName,
	}
	resp, err := c.hsClient.PerformRpc(builtinServiceNameObjectStore, objectStoreServiceMethodPut, payload, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	var result nats.ObjectInfo
	err = json.Unmarshal(resp.Data, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *BuiltinServicesClient) ObjectDelete(objectName string) error {
	metadata := map[string]string{
		agentapi.ObjectStoreObjectNameHeader: objectName,
	}
	resp, err := c.hsClient.PerformRpc(builtinServiceNameObjectStore, objectStoreServiceMethodDelete, []byte{}, metadata)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return resp.Error()
	}

	var oResp agentapi.HostServicesObjectStoreResponse
	err = json.Unmarshal(resp.Data, &oResp)
	if err != nil {
		return err
	}
	if !oResp.Success {
		return mergeErrors(oResp.Errors)
	}

	return nil
}

func (c *BuiltinServicesClient) SimpleHttpRequest(method string, url string, payload []byte) (*agentapi.HostServicesHTTPResponse, error) {
	metadata := map[string]string{
		agentapi.HttpURLHeader: url,
	}
	resp, err := c.hsClient.PerformRpc(builtinServiceNameHttpClient, method, payload, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}
	var hResp agentapi.HostServicesHTTPResponse
	err = json.Unmarshal(resp.Data, &hResp)
	if err != nil {
		return nil, err
	}

	return &hResp, nil
}

func (c *BuiltinServicesClient) RawClient() *hostservices.HostServicesClient {
	return c.hsClient
}

func mergeErrors(input []string) error {
	results := make([]error, len(input))
	for k, s := range input {
		results[k] = errors.New(s)
	}
	return errors.Join(results...)
}
