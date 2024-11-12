package builtins

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	// TODO: look into removing this dependency
	"github.com/vincent-petithory/dataurl"
)

const defaultRequestTimeoutMillis = 5000

// Generic HTTP client utility for interacting with web services; when a token is configured on a
// client instance it will be provided as a bearer authorization header; when a username and
// password are configured on a client instance, they will be used for HTTP basic authorization.
// If a token is configured on a client instance, the username and password supplied for basic auth are
// currently discarded.
type HTTPClient struct {
	Host   string
	Path   string
	Scheme string

	Cookie  *string
	Headers map[string][]string

	Token    *string
	Username *string
	Password *string

	log                *slog.Logger
	responseMarshaling bool
	timeout            time.Duration
	userAgent          *string
}

func NewHTTPClient(scheme, host, path string) *HTTPClient {
	return NewWithTimeout(scheme, host, path, defaultRequestTimeoutMillis)
}

func NewWithTimeout(scheme, host, path string, timeoutMillis int64) *HTTPClient {
	return &HTTPClient{
		Scheme:  scheme,
		Host:    host,
		Path:    path,
		timeout: time.Millisecond * time.Duration(timeoutMillis),
	}
}

// Get constructs and synchronously sends an API GET request
func (c *HTTPClient) Get(uri string, params interface{}) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequest("GET", url, nil, params)
	if err != nil {
		return
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API HEAD request; returns the headers
func (c *HTTPClient) Head(uri string, params interface{}) (status int, headers map[string][]string, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequest("HEAD", url, nil, params)
	if err != nil {
		if resp != nil {
			return resp.StatusCode, nil, err
		}
		return 0, nil, err
	}
	return resp.StatusCode, resp.Header, nil
}

// Constructs and synchronously sends an API HEAD request using the given TLS client config; returns the headers
func (c *HTTPClient) HeadWithTLSClientConfig(uri string, params interface{}, tlsClientConfig *tls.Config) (status int, headers map[string][]string, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequestWithTLSClientConfig("HEAD", url, nil, params, tlsClientConfig)
	if err != nil {
		if resp != nil {
			return resp.StatusCode, nil, err
		}
		return 0, nil, err
	}
	return resp.StatusCode, resp.Header, nil
}

// Constructs and synchronously sends an API GET request using the given TLS client config
func (c *HTTPClient) GetWithTLSClientConfig(uri string, params interface{}, tlsClientConfig *tls.Config) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequestWithTLSClientConfig("GET", url, nil, params, tlsClientConfig)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API PATCH request
func (c *HTTPClient) Patch(uri string, params interface{}) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequest("PATCH", url, nil, params)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API PATCH request using the given TLS client config
func (c *HTTPClient) PatchWithTLSClientConfig(uri string, params interface{}, tlsClientConfig *tls.Config) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequestWithTLSClientConfig("PATCH", url, nil, params, tlsClientConfig)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API POST request
func (c *HTTPClient) Post(uri string, params interface{}) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequest("POST", url, nil, params)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API POST request using the given TLS client config
func (c *HTTPClient) PostWithTLSClientConfig(uri string, params interface{}, tlsClientConfig *tls.Config) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequestWithTLSClientConfig("POST", url, nil, params, tlsClientConfig)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API POST request using application/x-www-form-urlencoded as the content-type
func (c *HTTPClient) PostWWWFormURLEncoded(uri string, params interface{}) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	contentType := "application/x-www-form-urlencoded"
	resp, err := c.sendRequest("POST", url, &contentType, params)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API POST request using application/x-www-form-urlencoded as the content-type and the given TLS client config
func (c *HTTPClient) PostWWWFormURLEncodedWithTLSClientConfig(uri string, params interface{}, tlsClientConfig *tls.Config) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	contentType := "application/x-www-form-urlencoded"
	resp, err := c.sendRequestWithTLSClientConfig("POST", url, &contentType, params, tlsClientConfig)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API POST request using multipart/form-data as the content-type
func (c *HTTPClient) PostMultipartFormData(uri string, params interface{}) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	contentType := "multipart/form-data"
	resp, err := c.sendRequest("POST", url, &contentType, params)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API POST request using multipart/form-data as the content-type and the given TLS client config
func (c *HTTPClient) PostMultipartFormDataWithTLSClientConfig(uri string, params interface{}, tlsClientConfig *tls.Config) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	contentType := "multipart/form-data"
	resp, err := c.sendRequestWithTLSClientConfig("POST", url, &contentType, params, tlsClientConfig)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API PUT request
func (c *HTTPClient) Put(uri string, params interface{}) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequest("PUT", url, nil, params)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API PUT request using the given TLS client config
func (c *HTTPClient) PutWithTLSClientConfig(uri string, params interface{}, tlsClientConfig *tls.Config) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequestWithTLSClientConfig("PUT", url, nil, params, tlsClientConfig)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API DELETE request
func (c *HTTPClient) Delete(uri string) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequest("DELETE", url, nil, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

// Constructs and synchronously sends an API DELETE request using the given TLS client config
func (c *HTTPClient) DeleteWithTLSClientConfig(uri string, tlsClientConfig *tls.Config) (status int, headers map[string][]string, response interface{}, err error) {
	url := c.buildURL(uri)
	resp, err := c.sendRequestWithTLSClientConfig("DELETE", url, nil, nil, tlsClientConfig)
	if err != nil {
		return 0, nil, nil, err
	}
	return c.handleResponse(resp)
}

func (c *HTTPClient) WithAuthorization(authorization string) *HTTPClient {
	c.Headers["Authorization"] = []string{authorization}
	return c
}

func (c *HTTPClient) WithCookie(cookie string) *HTTPClient {
	c.Headers["Cookie"] = []string{cookie}
	return c
}

func (c *HTTPClient) WithLogger(log *slog.Logger) *HTTPClient {
	c.log = log
	return c
}

func (c *HTTPClient) WithResponseMarshaling(responseMarshaling bool) *HTTPClient {
	c.responseMarshaling = responseMarshaling
	return c
}

func (c *HTTPClient) WithTimeoutMillis(timeoutMillis int64) *HTTPClient {
	c.timeout = time.Millisecond * time.Duration(timeoutMillis)
	return c
}

func (c *HTTPClient) WithUserAgent(userAgent string) *HTTPClient {
	c.userAgent = &userAgent
	return c
}

func (c *HTTPClient) handleResponse(resp *http.Response) (status int, headers map[string][]string, response interface{}, err error) {
	if resp == nil {
		return 0, nil, nil, errors.New("nil response")
	}

	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		c.log.Warn("failed to invoke HTTP request", slog.String("method", resp.Request.Method), slog.String("url", resp.Request.URL.String()), slog.String("error", err.Error()))
		return 0, nil, nil, err
	}

	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
	default:
		reader = resp.Body
	}

	buf := new(bytes.Buffer)
	if reader != nil {
		defer reader.Close()

		for {
			buffer := make([]byte, 256)
			n, err := reader.Read(buffer)
			if n > 0 {
				// c.log.Debug("read bytes from HTTP response stream", slog.Int("length", n))

				_, err := buf.Write(buffer[0:n])
				if err != nil {
					c.log.Warn("failed to write HTTP response to internal client buffer", slog.String("error", err.Error()))
					return resp.StatusCode, nil, nil, err
				} // TODO-- enable Trace slog.Logger else { //
				// c.log.Debug("wrote bytes from HTTP response to internal client buffer", slog.Int("length", i))
				//}
			} else if err != nil {
				if err != io.EOF {
					c.log.Warn("failed to read HTTP response stream", slog.String("error", err.Error()))
					return resp.StatusCode, nil, nil, err
				}
				break
			}
		}
	}

	if !c.responseMarshaling {
		return resp.StatusCode, resp.Header, buf.Bytes(), nil
	}

	if buf.Len() > 0 {
		contentTypeParts := strings.Split(resp.Header.Get("Content-Type"), ";")
		switch strings.ToLower(contentTypeParts[0]) {
		case "application/json":
			err = json.Unmarshal(buf.Bytes(), &response)
			if err != nil {
				err = fmt.Errorf("failed to unmarshal %v-byte HTTP %s response from %s; %s", len(buf.Bytes()), resp.Request.Method, resp.Request.URL.String(), err.Error())
				return resp.StatusCode, resp.Header, nil, err
			}
		default:
			// FIXME-- this should log a warning if response marshaling is enabled but the content type is not supported
			// no-op
		}
	}

	return resp.StatusCode, resp.Header, response, nil
}

func (c *HTTPClient) sendRequest(
	method,
	urlString string,
	contentType *string,
	params interface{},
) (resp *http.Response, err error) {
	tlsClientConfig, err := c.tlsClientConfigForURL(urlString)
	if err != nil {
		return nil, err
	}

	return c.sendRequestWithTLSClientConfig(method, urlString, contentType, params, tlsClientConfig)
}

func (c *HTTPClient) sendRequestWithTLSClientConfig(
	method,
	urlString string,
	contentType *string,
	params interface{},
	tlsClientConfig *tls.Config,
) (resp *http.Response, err error) {
	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig:   tlsClientConfig,
		},
		Timeout: c.timeout,
	}

	mthd := strings.ToUpper(method)
	reqURL, err := url.Parse(urlString)
	if err != nil {
		c.log.Warn("failed to parse URL for HTTP request", slog.String("method", method), slog.String("url", urlString), slog.String("error", err.Error()))
		return nil, err
	}

	if mthd == "GET" && params != nil {
		if _params, ok := params.(map[string]interface{}); ok {
			q := reqURL.Query()
			for name := range _params {
				if val, valOk := _params[name].(string); valOk {
					q.Set(name, val)
				}
			}
			reqURL.RawQuery = q.Encode()
		} else if raw, ok := params.([]byte); ok {
			reqURL.RawQuery = string(raw)
		}
	}

	headers := map[string][]string{}

	if _, ok := c.Headers["Accept"]; !ok {
		headers["Accept"] = []string{"*"}
	}

	if _, ok := c.Headers["Accept-Encoding"]; !ok {
		headers["Accept-Encoding"] = []string{"gzip, deflate"}
	}

	if _, ok := c.Headers["Accept-Language"]; !ok {
		headers["Accept-Language"] = []string{"en-us"}
	}

	if c.Token != nil {
		headers["Authorization"] = []string{fmt.Sprintf("bearer %s", *c.Token)}
	} else if c.Username != nil && c.Password != nil {
		headers["Authorization"] = []string{buildBasicAuthorizationHeader(*c.Username, *c.Password)}
	}

	if c.Headers != nil {
		for name, val := range c.Headers {
			headers[name] = val
		}
	}

	if _, ok := c.Headers["User-Agent"]; c.userAgent != nil && ok {
		headers["User-Agent"] = []string{*c.userAgent}
	}

	if c.Cookie != nil {
		// should probably warn if this is overriding a headers["Cookie"]
		headers["Cookie"] = []string{*c.Cookie}
	}

	var req *http.Request

	if mthd == "POST" || mthd == "PUT" || mthd == "PATCH" {
		var payload []byte

		if contentType == nil {
			_contentType := "text/plain"
			contentType = &_contentType
		}

		if raw, ok := params.([]byte); ok {
			payload = raw
		} else if _params, ok := params.(map[string]interface{}); ok {
			switch *contentType {
			case "application/json":
				payload, err = json.Marshal(_params)
				if err != nil {
					c.log.Warn("failed to marshal JSON payload for HTTP request", slog.String("method", method), slog.String("url", urlString), slog.String("error", err.Error()))
					return nil, err
				}

			case "application/x-www-form-urlencoded":
				urlEncodedForm := url.Values{}

				for key, val := range _params {
					if valStr, valOk := val.(string); valOk {
						urlEncodedForm.Add(key, valStr)
					} else {
						c.log.Warn("failed to marshal application/x-www-form-urlencoded parameter: %s; value was non-string", slog.String("key", key))
					}
				}
				payload = []byte(urlEncodedForm.Encode())

			case "multipart/form-data":
				body := new(bytes.Buffer)
				writer := multipart.NewWriter(body)

				for key, val := range _params {
					if valStr, valStrOk := val.(string); valStrOk {
						dURL, err := dataurl.DecodeString(valStr)
						if err == nil {
							c.log.Debug("parsed data url parameter", slog.String("key", key))

							part, err := writer.CreateFormFile(key, key)
							if err != nil {
								return nil, err
							}
							_, _ = part.Write(dURL.Data)
						} else {
							_ = writer.WriteField(key, valStr)
						}
					} else {
						c.log.Warn("skipping non-string value when constructing multipart/form-data request", slog.String("key", key))
					}
				}

				err = writer.Close()
				if err != nil {
					return nil, err
				}

				payload = []byte(body.Bytes())
			default:
				c.log.Warn("attempted HTTP %s request with unsupported content type: %s; unable to marshal request body", mthd, contentType)
			}
		}

		req, _ = http.NewRequest(method, urlString, bytes.NewReader(payload))
		headers["Content-Type"] = []string{*contentType}
	} else {
		req = &http.Request{
			URL:    reqURL,
			Method: mthd,
		}
	}

	req.Header = headers
	return client.Do(req)
}

// FIXME? cache URL => tls config in a map so lookup happens once
func (c *HTTPClient) tlsClientConfigForURL(urlString string) (*tls.Config, error) {
	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: false,
	}

	_url, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}

	hostIP := net.ParseIP(_url.Hostname())
	if hostIP != nil && hostIP.IsPrivate() {
		tlsClientConfig.InsecureSkipVerify = true
	}

	return tlsClientConfig, nil
}

func (c *HTTPClient) buildURL(uri string) string {
	path := c.Path
	if len(path) == 1 && path == "/" {
		path = ""
	} else if len(path) > 1 && strings.Index(path, "/") != 0 {
		path = fmt.Sprintf("/%s", path)
	}
	return fmt.Sprintf("%s://%s%s/%s", c.Scheme, c.Host, path, uri)
}

func buildBasicAuthorizationHeader(username, password string) string {
	auth := fmt.Sprintf("%s:%s", username, password)
	return fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(auth)))
}
