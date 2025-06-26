package models

import (
	"fmt"
	"testing"

	"github.com/carlmjohnson/be"
)

func TestStringerForEvents(t *testing.T) {
	t.Run("TestNexNodeStartedEvent", func(t *testing.T) {
		e := NexNodeStartedEvent{}
		be.Equal(t, "NODESTARTED", fmt.Sprintf("%s", e)) //nolint
	})
}
