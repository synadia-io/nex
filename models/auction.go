package models

type Auctioneer interface {
	Auction(namespace, agentType string, auctionTags map[string]string) error
}
