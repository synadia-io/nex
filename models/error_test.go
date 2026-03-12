package models

import (
	"errors"
	"net/http"
	"testing"

	"github.com/carlmjohnson/be"
)

func TestNewNexError(t *testing.T) {
	t.Parallel()

	raw := errors.New("disk full")
	ne := NewNexError("abc123", raw, "something went wrong", http.StatusInternalServerError)

	be.Equal(t, "abc123", ne.ID())
	be.Equal(t, "disk full", ne.Error())
	be.Equal(t, "something went wrong", string(ne.Body()))
	be.Equal(t, http.StatusInternalServerError, ne.Status())
}

func TestNexError_ImplementsError(t *testing.T) {
	t.Parallel()

	ne := NewNexError("", errors.New("boom"), "friendly", http.StatusBadRequest)

	var err error = ne
	be.Equal(t, "boom", err.Error())
}

func TestNexError_ImplementsNexErrorI(t *testing.T) {
	t.Parallel()

	ne := NewNexError("id1", errors.New("raw"), "safe msg", http.StatusNotFound)

	var i NexErrorI = ne
	be.Equal(t, "id1", i.ID())
	be.Equal(t, "raw", i.Error())
	be.Equal(t, "safe msg", string(i.Body()))
	be.Equal(t, http.StatusNotFound, i.Status())
}

func TestNexError_EmptyID(t *testing.T) {
	t.Parallel()

	ne := NewNexError("", errors.New("err"), "msg", http.StatusOK)
	be.Equal(t, "", ne.ID())
}

func TestNexError_BodyDistinctFromError(t *testing.T) {
	t.Parallel()

	ne := NewNexError("x", errors.New("secret: /var/lib/nex/keys/node.key not found"), "unable to process request", http.StatusInternalServerError)

	be.Equal(t, "secret: /var/lib/nex/keys/node.key not found", ne.Error())
	be.Equal(t, "unable to process request", string(ne.Body()))
}

func TestErrCodeConstants(t *testing.T) {
	t.Parallel()

	be.Equal(t, "400", ErrCodeBadRequest)
	be.Equal(t, "403", ErrCodeForbidden)
	be.Equal(t, "404", ErrCodeNotFound)
	be.Equal(t, "500", ErrCodeInternalServerError)
}
