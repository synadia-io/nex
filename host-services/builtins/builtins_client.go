package builtins

import (
	"context"
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

func (c *BuiltinServicesClient) KVGet(ctx context.Context, key string) ([]byte, error) {
	metadata := map[string]string{
		agentapi.KeyValueKeyHeader: key,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameKeyValue, kvServiceMethodGet, []byte{}, metadata)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, resp.Error()
	}

	return resp.Data, nil
}

func (c *BuiltinServicesClient) KVSet(ctx context.Context, key string, value []byte) (*agentapi.HostServicesKeyValueResponse, error) {
	metadata := map[string]string{
		agentapi.KeyValueKeyHeader: key,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameKeyValue, kvServiceMethodSet, value, metadata)
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

func (c *BuiltinServicesClient) KVDelete(ctx context.Context, key string) (*agentapi.HostServicesKeyValueResponse, error) {
	metadata := map[string]string{
		agentapi.KeyValueKeyHeader: key,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameKeyValue, kvServiceMethodDelete, []byte{}, metadata)
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

func (c *BuiltinServicesClient) KVKeys(ctx context.Context) ([]string, error) {
	metadata := map[string]string{}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameKeyValue, kvServiceMethodKeys, []byte{}, metadata)
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

func (c *BuiltinServicesClient) RawCall(ctx context.Context, serviceName string, methodName string, payload []byte) ([]byte, error) {
	metadata := map[string]string{}

	resp, err := c.hsClient.PerformRPC(ctx, serviceName, methodName, payload, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	return resp.Data, nil
}

func (c *BuiltinServicesClient) MessagingPublish(ctx context.Context, subject string, payload []byte) error {
	metadata := map[string]string{
		agentapi.MessagingSubjectHeader: subject,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameMessaging, messagingServiceMethodPublish, payload, metadata)
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

func (c *BuiltinServicesClient) MessagingRequest(ctx context.Context, subject string, payload []byte) ([]byte, error) {
	metadata := map[string]string{
		agentapi.MessagingSubjectHeader: subject,
	}
	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameMessaging, messagingServiceMethodRequest, payload, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	return resp.Data, nil
}

func (c *BuiltinServicesClient) ObjectGet(ctx context.Context, objectName string) ([]byte, error) {
	metadata := map[string]string{
		agentapi.ObjectStoreObjectNameHeader: objectName,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameObjectStore, objectStoreServiceMethodGet, []byte{}, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	return resp.Data, nil
}

func (c *BuiltinServicesClient) ObjectList(ctx context.Context) ([]*nats.ObjectInfo, error) {
	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameObjectStore, objectStoreServiceMethodList, []byte{}, make(map[string]string))
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

func (c *BuiltinServicesClient) ObjectPut(ctx context.Context, objectName string, payload []byte) (*nats.ObjectInfo, error) {
	metadata := map[string]string{
		agentapi.ObjectStoreObjectNameHeader: objectName,
	}
	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameObjectStore, objectStoreServiceMethodPut, payload, metadata)
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

func (c *BuiltinServicesClient) ObjectDelete(ctx context.Context, objectName string) error {
	metadata := map[string]string{
		agentapi.ObjectStoreObjectNameHeader: objectName,
	}
	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameObjectStore, objectStoreServiceMethodDelete, []byte{}, metadata)
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

func (c *BuiltinServicesClient) SimpleHttpRequest(ctx context.Context, method string, url string, payload []byte) (*agentapi.HostServicesHTTPResponse, error) {
	metadata := map[string]string{
		agentapi.HttpURLHeader: url,
	}
	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameHttpClient, method, payload, metadata)
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
