package builtins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/nats-io/nats.go"
	hostservices "github.com/synadia-io/nex/host-services"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
	"github.com/synadia-io/nex/internal/node/services/util"
)

const httpServiceMethodGet = "get"
const httpServiceMethodPost = "post"
const httpServiceMethodPut = "put"
const httpServiceMethodPatch = "patch"
const httpServiceMethodDelete = "delete"
const httpServiceMethodHead = "head"

const defaultHTTPRequestTimeoutMillis = 2500

type HTTPService struct {
	log *slog.Logger
	nc  *nats.Conn
}

func NewHTTPService(nc *nats.Conn, log *slog.Logger) (*HTTPService, error) {
	http := &HTTPService{
		log: log,
		nc:  nc,
	}

	return http, nil
}

func (h *HTTPService) Initialize(_ map[string]string) error {
	return nil
}

func (h *HTTPService) HandleRequest(namespace string,
	workloadId string,
	method string,
	workloadName string,
	metadata map[string]string,
	request []byte) (hostservices.ServiceResult, error) {

	switch method {
	case httpServiceMethodGet:
		return h.handleGet(workloadId, workloadName, request, metadata)
	case httpServiceMethodPost:
		return h.handlePost(workloadId, workloadName, request, metadata)
	case httpServiceMethodPut:
		return h.handlePut(workloadId, workloadName, request, metadata)
	case httpServiceMethodPatch:
		return h.handlePatch(workloadId, workloadName, request, metadata)
	case httpServiceMethodDelete:
		return h.handleDelete(workloadId, workloadName, request, metadata)
	case httpServiceMethodHead:
		return h.handleHead(workloadId, workloadName, request, metadata)
	default:
		h.log.Warn("Received invalid host services RPC request",
			slog.String("service", "http"),
			slog.String("method", method),
		)
		return hostservices.ServiceResultFail(400, "Received invalid host services RPC request"), nil
	}
}

func (h *HTTPService) handleGet(_, _ string, _ []byte, metadata map[string]string) (hostservices.ServiceResult, error) {
	url, err := url.Parse(metadata[agentapi.HttpURLHeader])
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))
		return hostservices.ServiceResultFail(400, err.Error()), nil
	}

	client := util.NewHTTPClient(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	status, resphdrs, httpresp, err := client.Get(url.Path, url.Query())
	if err != nil {
		return hostservices.ServiceResultFail(500, err.Error()), nil
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (h *HTTPService) handlePost(_, _ string, data []byte, metadata map[string]string) (hostservices.ServiceResult, error) {
	url, err := url.Parse(metadata[agentapi.HttpURLHeader])
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))

		return hostservices.ServiceResultFail(400, "failed to parse url for http RPC request"), nil
	}

	client := util.NewHTTPClient(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	status, resphdrs, httpresp, err := client.Post(url.Path, data)
	if err != nil {
		return hostservices.ServiceResultFail(500, fmt.Sprintf("http reqeust failed: %s", err.Error())), nil
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (h *HTTPService) handlePut(_, _ string, data []byte, metadata map[string]string) (hostservices.ServiceResult, error) {
	url, err := url.Parse(metadata[agentapi.HttpURLHeader])
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))
		return hostservices.ServiceResultFail(400, "failed to parse url for http RPC request"), nil
	}

	client := util.NewHTTPClient(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	status, resphdrs, httpresp, err := client.Put(url.Path, data)
	if err != nil {
		return hostservices.ServiceResultFail(500, "http request failed"), nil
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (h *HTTPService) handlePatch(_, _ string, data []byte, metadata map[string]string) (hostservices.ServiceResult, error) {
	url, err := url.Parse(metadata[agentapi.HttpURLHeader])
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))
		return hostservices.ServiceResultFail(400, "failed to parse url for http RPC request"), nil
	}

	client := util.NewHTTPClient(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	status, resphdrs, httpresp, err := client.Patch(url.Path, data)
	if err != nil {
		return hostservices.ServiceResultFail(500, "http request failed"), nil
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (h *HTTPService) handleDelete(_, _ string, _ []byte, metadata map[string]string) (hostservices.ServiceResult, error) {
	url, err := url.Parse(metadata[agentapi.HttpURLHeader])
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))
		return hostservices.ServiceResultFail(400, "failed to parse url for http RPC request"), nil
	}

	client := util.NewHTTPClient(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	status, resphdrs, httpresp, err := client.Delete(url.Path)
	if err != nil {
		return hostservices.ServiceResultFail(500, "http request failed"), nil
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}

func (h *HTTPService) handleHead(_, _ string, _ []byte, metadata map[string]string) (hostservices.ServiceResult, error) {
	url, err := url.Parse(metadata[agentapi.HttpURLHeader])
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))
		return hostservices.ServiceResultFail(200, "failed to parse url for http RPC request"), nil
	}

	client := util.NewHTTPClient(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	status, resphdrs, err := client.Head(url.Path, url.Query())
	if err != nil {
		return hostservices.ServiceResultFail(500, "http request failed"), nil
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
	})
	return hostservices.ServiceResultPass(200, "", resp), nil
}
