package models

// Auctioneer allows user-defined auction validation logic. Implementations
// are called during the auction phase before a node places a bid.
//
// Returning a nil error allows the auction to proceed normally.
// Returning (false, err) silently drops this node from the auction — the
// error is logged but not sent to the caller.
// Returning (true, err) short-circuits the auction and sends the error
// back to the requesting client as a structured NexError.
type Auctioneer interface {
	Auction(namespace, agentType string, auctionTags map[string]string) (bool, error)
}
