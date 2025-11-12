package client

import (
	"time"
)

type ClientOption func(*nexClient) error

func WithDefaultTimeout(timeout time.Duration) ClientOption {
	return func(c *nexClient) error {
		c.defaultTimeout = timeout
		return nil
	}
}

func WithStartWorkloadTimeout(timeout time.Duration) ClientOption {
	return func(c *nexClient) error {
		c.startWorkloadTimeout = timeout
		return nil
	}
}

// WithAuctionStall modifies the stall duration for the request many call
// conducted during an auction
func WithAuctionStall(stall time.Duration) ClientOption {
	return func(c *nexClient) error {
		c.auctionRequestManyStall = stall
		return nil
	}
}

func WithRequestManyStall(stall time.Duration) ClientOption {
	return func(c *nexClient) error {
		c.requestManyStall = stall
		return nil
	}
}
