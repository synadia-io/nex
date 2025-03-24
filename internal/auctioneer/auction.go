package auctioneer

import (
	"fmt"
	"log/slog"

	"github.com/nats-io/nuid"
	"github.com/synadia-labs/nex/internal"
	"github.com/synadia-labs/nex/models"
)

type TagAuctioneer struct {
	Regs       *models.Regs
	AuctionMap *internal.TTLMap
}

func (ta *TagAuctioneer) Auction(auctionId, agentType string, auctionTags, nodeTags map[string]string, logger *slog.Logger) (*models.AuctionResponse, error) {
	// If node doesnt have agent type, request is thrown away
	_, reg, ok := ta.Regs.Find(agentType)
	if !ok {
		return nil, fmt.Errorf("agent type not found during auction: %s", agentType)
	}

	// If all auction tags aren't satisfied, request is thrown away
	for k, v := range auctionTags {
		if tV, ok := nodeTags[k]; !ok || tV != v {
			return nil, fmt.Errorf("tag not satisfied during auction: %s=%s", k, v)
		}
	}

	bidderId := nuid.New().Next()
	ta.AuctionMap.Put(bidderId, "", nil)

	logger.Debug("responding to auction", slog.Any("auctionId", auctionId))
	return &models.AuctionResponse{
		BidderId:            bidderId,
		Xkey:                reg.OriginalRequest.PublicXkey,
		StartRequestSchema:  reg.OriginalRequest.StartRequestSchema,
		SupportedLifecycles: reg.OriginalRequest.SupportedLifecycles,
	}, nil
}
