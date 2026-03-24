package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/synadia-io/nex/_test"
	"github.com/synadia-io/nex/models"
)

func TestNexClientErrorHelpers_GenerateUniqueIDs(t *testing.T) {
	t.Parallel()

	server := _test.StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	c, err := NewClient(context.Background(), nc, "test")
	be.NilErr(t, err)
	client := c.(*nexClient)

	e1 := client.nexInternalError(errors.New("a"), "a")
	e2 := client.nexInternalError(errors.New("b"), "b")
	e3 := client.nexBadRequestError(errors.New("c"), "c")
	e4 := client.nexNotFoundError(errors.New("d"), "d")

	ids := map[string]bool{
		e1.ID(): true,
		e2.ID(): true,
		e3.ID(): true,
		e4.ID(): true,
	}
	be.Equal(t, 4, len(ids))
}

func TestNexClientErrorHelpers_NonEmptyIDs(t *testing.T) {
	t.Parallel()

	server := _test.StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	c, err := NewClient(context.Background(), nc, "test")
	be.NilErr(t, err)
	client := c.(*nexClient)

	be.Nonzero(t, client.nexInternalError(errors.New("x"), "x").ID())
	be.Nonzero(t, client.nexBadRequestError(errors.New("x"), "x").ID())
	be.Nonzero(t, client.nexNotFoundError(errors.New("x"), "x").ID())
}

func TestNexClientErrorHelpers_StatusCodes(t *testing.T) {
	t.Parallel()

	server := _test.StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	c, err := NewClient(context.Background(), nc, "test")
	be.NilErr(t, err)
	client := c.(*nexClient)

	be.Equal(t, http.StatusInternalServerError, client.nexInternalError(errors.New("x"), "x").Status())
	be.Equal(t, http.StatusBadRequest, client.nexBadRequestError(errors.New("x"), "x").Status())
	be.Equal(t, http.StatusNotFound, client.nexNotFoundError(errors.New("x"), "x").Status())
}

func TestNexClientErrorHelpers_PreservesMessages(t *testing.T) {
	t.Parallel()

	server := _test.StartNatsServer(t, t.TempDir())
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	be.NilErr(t, err)
	defer nc.Close()

	c, err := NewClient(context.Background(), nc, "test")
	be.NilErr(t, err)
	client := c.(*nexClient)

	ne := client.nexInternalError(errors.New("raw detail"), "friendly message")
	be.Equal(t, "raw detail", ne.Error())
	be.Equal(t, "friendly message", string(ne.Body()))
}

func TestNexErrorFromMsg_WithErrorHeaders(t *testing.T) {
	t.Parallel()

	body := struct {
		ErrorID string `json:"error_id"`
		Error   string `json:"error"`
	}{
		ErrorID: "corr-123",
		Error:   "something broke",
	}
	bodyB, err := json.Marshal(body)
	be.NilErr(t, err)

	msg := &nats.Msg{
		Header: nats.Header{},
		Data:   bodyB,
	}
	msg.Header.Set(micro.ErrorCodeHeader, "500")
	msg.Header.Set(micro.ErrorHeader, "internal server error")

	ne := nexErrorFromMsg(msg)
	be.Nonzero(t, ne)
	be.Equal(t, "corr-123", ne.ID())
	be.Equal(t, "something broke", ne.Error())
	be.Equal(t, "internal server error", string(ne.Body()))
	be.Equal(t, 500, ne.Status())
}

func TestNexErrorFromMsg_NoErrorHeaders(t *testing.T) {
	t.Parallel()

	msg := &nats.Msg{
		Header: nats.Header{},
		Data:   []byte(`{}`),
	}

	ne := nexErrorFromMsg(msg)
	be.Zero(t, ne)
}

func TestNexErrorFromMsg_MalformedBody(t *testing.T) {
	t.Parallel()

	msg := &nats.Msg{
		Header: nats.Header{},
		Data:   []byte(`not json`),
	}
	msg.Header.Set(micro.ErrorCodeHeader, "400")
	msg.Header.Set(micro.ErrorHeader, "bad request")

	ne := nexErrorFromMsg(msg)
	be.Nonzero(t, ne)
	// Falls back to friendlyMsg when body unmarshal fails
	be.Equal(t, "bad request", ne.Error())
	be.Equal(t, "bad request", string(ne.Body()))
	be.Equal(t, 400, ne.Status())
	be.Equal(t, "", ne.ID())
}

func TestNexErrorFromMsg_EmptyBodyError(t *testing.T) {
	t.Parallel()

	body := struct {
		ErrorID string `json:"error_id"`
		Error   string `json:"error"`
	}{
		ErrorID: "id-456",
		Error:   "",
	}
	bodyB, err := json.Marshal(body)
	be.NilErr(t, err)

	msg := &nats.Msg{
		Header: nats.Header{},
		Data:   bodyB,
	}
	msg.Header.Set(micro.ErrorCodeHeader, "404")
	msg.Header.Set(micro.ErrorHeader, "not found")

	ne := nexErrorFromMsg(msg)
	be.Nonzero(t, ne)
	be.Equal(t, "id-456", ne.ID())
	// Falls back to friendlyMsg when body.Error is empty
	be.Equal(t, "not found", ne.Error())
	be.Equal(t, 404, ne.Status())
}

func TestNexErrorFromMsg_SatisfiesNexErrorI(t *testing.T) {
	t.Parallel()

	body := struct {
		ErrorID string `json:"error_id"`
		Error   string `json:"error"`
	}{
		ErrorID: "abc",
		Error:   "detail",
	}
	bodyB, err := json.Marshal(body)
	be.NilErr(t, err)

	msg := &nats.Msg{
		Header: nats.Header{},
		Data:   bodyB,
	}
	msg.Header.Set(micro.ErrorCodeHeader, "500")
	msg.Header.Set(micro.ErrorHeader, "server error")

	ne := nexErrorFromMsg(msg)

	var i models.NexErrorI = ne
	be.Equal(t, "abc", i.ID())
	be.Equal(t, "detail", i.Error())
	be.Equal(t, "server error", string(i.Body()))
	be.Equal(t, 500, i.Status())
}
