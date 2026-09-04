package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/stranix79/deckhand/internal/local"
	"github.com/stranix79/deckhand/internal/session"
)

// startRelay pushes the deck to the hub, starts a relayed presentation and
// forwards every local state change. Errors here never stop the local
// presentation: the caller prints them in yellow and carries on.
func startRelay(ctx context.Context, cmd *cobra.Command, p *local.Presentation, path string, o local.Options) error {
	hc, err := hubClient(o.Hub)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	pctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	res, err := hc.Push(pctx, path, o.HubSlug)
	if err != nil {
		return err
	}
	start, err := hc.Start(pctx, res.ID)
	if err != nil {
		return err
	}
	// Remote viewers use the hub link: the stage QR and the remote show it.
	p.Session.ViewerURL = start.ViewerURL
	fmt.Fprintf(out, "  %s   %s\n", ui.dim("hub    "), ui.ok(start.ViewerURL))
	fmt.Fprintf(out, "  %s   %s\n\n", ui.dim("stats  "), ui.dim(start.StatsURL))

	states := make(chan session.State, 64)
	p.Session.OnState = func(st session.State) {
		select {
		case states <- st:
		default:
			// The relay is slower than the presenter: drop, the next state is complete.
		}
	}
	go local.Relay(ctx, start.RelayURL, states,
		func(n int) { fmt.Fprintf(out, "  %s\n", ui.dim(fmt.Sprintf("hub: %d remote viewer(s)", n))) },
		func(msg string) { fmt.Fprintf(out, "  %s\n", ui.warn(msg)) },
	)
	return nil
}
