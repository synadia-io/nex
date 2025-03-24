package auctioneer

import (
	"log/slog"
	"os"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-labs/nex/internal"
	"github.com/synadia-labs/nex/models"
)

func TestTagAuctioneer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	regs := models.NewRegistrationList(logger)
	be.Nonzero(t, regs)
	be.NilErr(t, regs.New("1234", models.RegTypeEmbeddedAgent, "AAPIUZ3KE6ENDNAF7THAX4ENKLWJMVTQAV24TWKY2N6NKKAUQVA5Y36Q"))
	be.NilErr(t, regs.Update("1234", &models.Reg{
		Id: "1234",
		OriginalRequest: &models.RegisterAgentRequest{
			RegisterType: "1234",
		},
		Type: models.RegTypeEmbeddedAgent,
	}))

	ta := TagAuctioneer{
		Regs:       regs,
		AuctionMap: internal.NewTTLMap(10),
	}

	resp, err := ta.Auction("1234", "1234", map[string]string{"key": "value"}, map[string]string{"key": "value"}, logger)
	be.NilErr(t, err)

	be.True(t, ta.AuctionMap.Exists(resp.BidderId))
}
