package lib

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	httpclient "github.com/kthomas/go-httpclient"
	"github.com/nats-io/nats.go"
	agentapi "github.com/synadia-io/nex/internal/agent-api"
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

	err := http.init()
	if err != nil {
		return nil, err
	}

	return http, nil
}

func (h *HTTPService) init() error {
	return nil
}

func (h *HTTPService) HandleRPC(msg *nats.Msg) {
	// agentint.{vmID}.rpc.{namespace}.{workload}.{service}.{method}
	tokens := strings.Split(msg.Subject, ".")
	service := tokens[5]
	method := tokens[6]

	switch method {
	case httpServiceMethodGet:
		h.handleGet(msg)
	case httpServiceMethodPost:
		h.handlePost(msg)
	case httpServiceMethodPut:
		h.handlePut(msg)
	case httpServiceMethodPatch:
		h.handlePatch(msg)
	case httpServiceMethodDelete:
		h.handleDelete(msg)
	case httpServiceMethodHead:
		h.handleHead(msg)
	default:
		h.log.Warn("Received invalid host services RPC request",
			slog.String("service", service),
			slog.String("method", method),
		)

		// msg.Respond()
	}
}

func (h *HTTPService) handleGet(msg *nats.Msg) {
	var req *agentapi.HostServicesHTTPRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	url, err := url.Parse(req.URL)
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to parse url for http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	client := httpclient.New(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	params := map[string]interface{}{}
	if req.Params != nil {
		err = json.Unmarshal(*req.Params, &params)
		if err != nil {
			h.log.Debug("failed to unmarshal query params for http RPC request", slog.String("error", err.Error()))

			resp, _ := json.Marshal(map[string]interface{}{
				"error": fmt.Sprintf("failed to unmarshal query params for http RPC request: %s", err.Error()),
			})

			err := msg.Respond(resp)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
			}
			return
		}
	}

	status, resphdrs, httpresp, err := client.Get(url.Path, params)
	if err != nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("http reqeust failed: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("http request failed: %s", err.Error()))
		}
		return
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	err = msg.Respond(resp)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to respond to http host service request: %s", err.Error()))
	}
}

func (h *HTTPService) handlePost(msg *nats.Msg) {
	var req *agentapi.HostServicesHTTPRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	url, err := url.Parse(req.URL)
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to parse url for http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	client := httpclient.New(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	params := map[string]interface{}{}
	if req.Params != nil {
		err = json.Unmarshal(*req.Params, &params)
		if err != nil {
			h.log.Debug("failed to unmarshal query params for http RPC request", slog.String("error", err.Error()))

			resp, _ := json.Marshal(map[string]interface{}{
				"error": fmt.Sprintf("failed to unmarshal query params for http RPC request: %s", err.Error()),
			})

			err := msg.Respond(resp)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
			}
			return
		}
	}

	status, resphdrs, httpresp, err := client.Post(url.Path, req.Body)
	if err != nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("http reqeust failed: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("http request failed: %s", err.Error()))
		}
		return
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	err = msg.Respond(resp)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to respond to http host service request: %s", err.Error()))
	}
}

func (h *HTTPService) handlePut(msg *nats.Msg) {
	var req *agentapi.HostServicesHTTPRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	url, err := url.Parse(req.URL)
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to parse url for http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	client := httpclient.New(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	params := map[string]interface{}{}
	if req.Params != nil {
		err = json.Unmarshal(*req.Params, &params)
		if err != nil {
			h.log.Debug("failed to unmarshal query params for http RPC request", slog.String("error", err.Error()))

			resp, _ := json.Marshal(map[string]interface{}{
				"error": fmt.Sprintf("failed to unmarshal query params for http RPC request: %s", err.Error()),
			})

			err := msg.Respond(resp)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
			}
			return
		}
	}

	status, resphdrs, httpresp, err := client.Put(url.Path, req.Body)
	if err != nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("http reqeust failed: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("http request failed: %s", err.Error()))
		}
		return
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	err = msg.Respond(resp)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to respond to http host service request: %s", err.Error()))
	}
}

func (h *HTTPService) handlePatch(msg *nats.Msg) {
	var req *agentapi.HostServicesHTTPRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	url, err := url.Parse(req.URL)
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to parse url for http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	client := httpclient.New(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	params := map[string]interface{}{}
	if req.Params != nil {
		err = json.Unmarshal(*req.Params, &params)
		if err != nil {
			h.log.Debug("failed to unmarshal query params for http RPC request", slog.String("error", err.Error()))

			resp, _ := json.Marshal(map[string]interface{}{
				"error": fmt.Sprintf("failed to unmarshal query params for http RPC request: %s", err.Error()),
			})

			err := msg.Respond(resp)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
			}
			return
		}
	}

	status, resphdrs, httpresp, err := client.Patch(url.Path, req.Body)
	if err != nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("http reqeust failed: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("http request failed: %s", err.Error()))
		}
		return
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	err = msg.Respond(resp)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to respond to http host service request: %s", err.Error()))
	}
}

func (h *HTTPService) handleDelete(msg *nats.Msg) {
	var req *agentapi.HostServicesHTTPRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	url, err := url.Parse(req.URL)
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to parse url for http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	client := httpclient.New(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	params := map[string]interface{}{}
	if req.Params != nil {
		err = json.Unmarshal(*req.Params, &params)
		if err != nil {
			h.log.Debug("failed to unmarshal query params for http RPC request", slog.String("error", err.Error()))

			resp, _ := json.Marshal(map[string]interface{}{
				"error": fmt.Sprintf("failed to unmarshal query params for http RPC request: %s", err.Error()),
			})

			err := msg.Respond(resp)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
			}
			return
		}
	}

	status, resphdrs, httpresp, err := client.Delete(url.Path)
	if err != nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("http reqeust failed: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("http request failed: %s", err.Error()))
		}
		return
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
		Body:    string(httpresp.([]byte)),
	})
	err = msg.Respond(resp)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to respond to http host service request: %s", err.Error()))
	}
}

func (h *HTTPService) handleHead(msg *nats.Msg) {
	var req *agentapi.HostServicesHTTPRequest
	err := json.Unmarshal(msg.Data, &req)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	url, err := url.Parse(req.URL)
	if err != nil {
		h.log.Debug("failed to parse url for http RPC request", slog.String("error", err.Error()))

		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("failed to parse url for http RPC request: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
		}
		return
	}

	client := httpclient.New(url.Scheme, url.Host, "").
		WithLogger(h.log).
		WithTimeoutMillis(defaultHTTPRequestTimeoutMillis)

	params := map[string]interface{}{}
	if req.Params != nil {
		err = json.Unmarshal(*req.Params, &params)
		if err != nil {
			h.log.Debug("failed to unmarshal query params for http RPC request", slog.String("error", err.Error()))

			resp, _ := json.Marshal(map[string]interface{}{
				"error": fmt.Sprintf("failed to unmarshal query params for http RPC request: %s", err.Error()),
			})

			err := msg.Respond(resp)
			if err != nil {
				h.log.Error(fmt.Sprintf("failed to respond to host services RPC request: %s", err.Error()))
			}
			return
		}
	}

	status, resphdrs, err := client.Head(url.Path, params)
	if err != nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": fmt.Sprintf("http reqeust failed: %s", err.Error()),
		})

		err := msg.Respond(resp)
		if err != nil {
			h.log.Error(fmt.Sprintf("http request failed: %s", err.Error()))
		}
		return
	}

	var respHeaders json.RawMessage
	respHeaders, _ = json.Marshal(resphdrs)

	resp, _ := json.Marshal(&agentapi.HostServicesHTTPResponse{
		Status:  status,
		Headers: &respHeaders,
	})
	err = msg.Respond(resp)
	if err != nil {
		h.log.Warn(fmt.Sprintf("failed to respond to http host service request: %s", err.Error()))
	}
}
