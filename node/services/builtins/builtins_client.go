package builtins

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/node/services"
)

const BucketContextHeader = "x-context-bucket"

type HostServicesHTTPRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`

	Body    *string          `json:"body,omitempty"`
	Headers *json.RawMessage `json:"headers,omitempty"`

	// FIXME-- this is very poorly named currently...
	//these params are parsed as an object and serialized as part of the query string
	Params *json.RawMessage `json:"params,omitempty"`
}

type HostServicesHTTPResponse struct {
	Status  int              `json:"status"`
	Headers *json.RawMessage `json:"headers,omitempty"`
	Body    string           `json:"body"`

	Error *string `json:"error,omitempty"`
}

type HostServicesKeyValueResponse struct {
	Revision int64 `json:"revision,omitempty"`
	Success  *bool `json:"success,omitempty"`

	Errors []string `json:"errors,omitempty"`
}

type HostServicesObjectStoreResponse struct {
	Errors  []string `json:"errors,omitempty"`
	Success bool     `json:"success,omitempty"`
}

type HostServicesMessagingRequest struct {
	Subject *string          `json:"key"`
	Payload *json.RawMessage `json:"payload,omitempty"`
}

type HostServicesMessagingResponse struct {
	Errors  []string `json:"errors,omitempty"`
	Success bool     `json:"success,omitempty"`
}

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

func (c *BuiltinServicesClient) KVGet(ctx context.Context, bucket string, key string) ([]byte, error) {
	metadata := map[string]string{
		KeyValueKeyHeader:   key,
		BucketContextHeader: bucket,
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

func (c *BuiltinServicesClient) KVSet(ctx context.Context, bucket string, key string, value []byte) (*HostServicesKeyValueResponse, error) {
	metadata := map[string]string{
		KeyValueKeyHeader:   key,
		BucketContextHeader: bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameKeyValue, kvServiceMethodSet, value, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	var kvResponse HostServicesKeyValueResponse
	err = json.Unmarshal(resp.Data, &kvResponse)
	if err != nil {
		return nil, err
	}

	return &kvResponse, nil
}

func (c *BuiltinServicesClient) KVDelete(ctx context.Context, bucket string, key string) (*HostServicesKeyValueResponse, error) {
	metadata := map[string]string{
		KeyValueKeyHeader:   key,
		BucketContextHeader: bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameKeyValue, kvServiceMethodDelete, []byte{}, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}

	var kvResponse HostServicesKeyValueResponse
	err = json.Unmarshal(resp.Data, &kvResponse)
	if err != nil {
		return nil, err
	}

	return &kvResponse, nil
}

func (c *BuiltinServicesClient) KVKeys(ctx context.Context, bucket string) ([]string, error) {
	metadata := map[string]string{
		BucketContextHeader: bucket,
	}

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

func (c *BuiltinServicesClient) MessagingPublish(ctx context.Context, subject string, payload []byte) error {
	metadata := map[string]string{
		MessagingSubjectHeader: subject,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameMessaging, messagingServiceMethodPublish, payload, metadata)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return resp.Error()
	}

	var response HostServicesMessagingResponse
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
		MessagingSubjectHeader: subject,
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

func (c *BuiltinServicesClient) ObjectGet(ctx context.Context, bucket string, objectName string) ([]byte, error) {
	metadata := map[string]string{
		ObjectStoreObjectNameHeader: objectName,
		BucketContextHeader:         bucket,
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

func (c *BuiltinServicesClient) ObjectList(ctx context.Context, bucket string) ([]*nats.ObjectInfo, error) {
	metadata := map[string]string{
		BucketContextHeader: bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameObjectStore, objectStoreServiceMethodList, []byte{}, metadata)
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

func (c *BuiltinServicesClient) ObjectPut(ctx context.Context, bucket string, objectName string, payload []byte) (*nats.ObjectInfo, error) {
	metadata := map[string]string{
		ObjectStoreObjectNameHeader: objectName,
		BucketContextHeader:         bucket,
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

func (c *BuiltinServicesClient) ObjectDelete(ctx context.Context, bucket string, objectName string) error {
	metadata := map[string]string{
		ObjectStoreObjectNameHeader: objectName,
		BucketContextHeader:         bucket,
	}
	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameObjectStore, objectStoreServiceMethodDelete, []byte{}, metadata)
	if err != nil {
		return err
	}
	if resp.IsError() {
		return resp.Error()
	}

	var oResp HostServicesObjectStoreResponse
	err = json.Unmarshal(resp.Data, &oResp)
	if err != nil {
		return err
	}
	if !oResp.Success {
		return mergeErrors(oResp.Errors)
	}

	return nil
}

func (c *BuiltinServicesClient) SimpleHttpRequest(ctx context.Context, method string, url string, payload []byte) (*HostServicesHTTPResponse, error) {
	metadata := map[string]string{
		HttpURLHeader: url,
	}
	resp, err := c.hsClient.PerformRPC(ctx, builtinServiceNameHttpClient, method, payload, metadata)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, resp.Error()
	}
	var hResp HostServicesHTTPResponse
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
