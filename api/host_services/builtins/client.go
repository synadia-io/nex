package builtins

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/api/host_services"
)

type BuiltinServicesClient struct {
	hsClient     *hostservices.HostServicesClient
	workloadType string
}

const (
	builtinServiceNameKeyValue    = "kv"
	builtinServiceNameHttpClient  = "http"
	builtinServiceNameMessaging   = "messaging"
	builtinServiceNameObjectStore = "objectstore"
)

func NewBuiltinServicesClient(hsClient *hostservices.HostServicesClient, workloadType string) *BuiltinServicesClient {
	return &BuiltinServicesClient{
		hsClient:     hsClient,
		workloadType: workloadType,
	}
}

func (c *BuiltinServicesClient) KVGet(ctx context.Context, bucket string, key string) ([]byte, error) {
	metadata := map[string]string{
		hostservices.KeyValueKeyHeader:   key,
		hostservices.BucketContextHeader: bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameKeyValue, kvServiceMethodGet, []byte{}, metadata)
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
		hostservices.KeyValueKeyHeader:   key,
		hostservices.BucketContextHeader: bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameKeyValue, kvServiceMethodSet, value, metadata)
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
		hostservices.KeyValueKeyHeader:   key,
		hostservices.BucketContextHeader: bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameKeyValue, kvServiceMethodDelete, []byte{}, metadata)
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
		hostservices.BucketContextHeader: bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameKeyValue, kvServiceMethodKeys, []byte{}, metadata)
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
		hostservices.MessagingSubjectHeader: subject,
	}

	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameMessaging, messagingServiceMethodPublish, payload, metadata)
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
		hostservices.MessagingSubjectHeader: subject,
	}
	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameMessaging, messagingServiceMethodRequest, payload, metadata)
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
		hostservices.ObjectStoreObjectNameHeader: objectName,
		hostservices.BucketContextHeader:         bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameObjectStore, objectStoreServiceMethodGet, []byte{}, metadata)
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
		hostservices.BucketContextHeader: bucket,
	}

	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameObjectStore, objectStoreServiceMethodList, []byte{}, metadata)
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
		hostservices.ObjectStoreObjectNameHeader: objectName,
		hostservices.BucketContextHeader:         bucket,
	}
	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameObjectStore, objectStoreServiceMethodPut, payload, metadata)
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
		hostservices.ObjectStoreObjectNameHeader: objectName,
		hostservices.BucketContextHeader:         bucket,
	}
	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameObjectStore, objectStoreServiceMethodDelete, []byte{}, metadata)
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
		hostservices.HttpURLHeader: url,
	}
	resp, err := c.hsClient.PerformRPC(ctx, c.workloadType, builtinServiceNameHttpClient, method, payload, metadata)
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
