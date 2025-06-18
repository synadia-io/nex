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

func WithAuctionTimeout(timeout time.Duration) ClientOption {
	return func(c *nexClient) error {
		c.auctionTimeout = timeout
		return nil
	}
}

func WithStartWorkloadTimeout(timeout time.Duration) ClientOption {
	return func(c *nexClient) error {
		c.startWorkloadTimeout = timeout
		return nil
	}
}

func WithRequestManyStall(stall time.Duration) ClientOption {
	return func(c *nexClient) error {
		c.requestManyStall = stall
		return nil
	}
}

