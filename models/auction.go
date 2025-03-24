package models

import "log/slog"

type Auctioneer interface {
	Auction(auctionId, agentType string, auctionTags, nodeTags map[string]string, logger *slog.Logger) (*AuctionResponse, error)
}
