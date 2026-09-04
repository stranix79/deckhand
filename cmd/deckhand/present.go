package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/stranix79/deckhand/internal/deck"
	"github.com/stranix79/deckhand/internal/local"
)

func presentCmd() *cobra.Command {
	var o local.Options
	c := &cobra.Command{
		Use:   "present <deck>",
		Short: "Present a deck on this network: stage, remote and audience screens",
		Long: `present validates the deck, starts a server on your LAN and prints the
three links with QR codes. Scan the REMOTE code with your phone, open the
stage link on the projector, share the AUDIENCE code with the room.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := deck.Load(args[0])
			if err != nil {
				printReport(cmd.OutOrStdout(), args[0], nil, deck.AsReport(err), false)
				os.Exit(1)
			}
			defer func() { _ = d.Close() }()
			for _, wmsg := range d.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s %s\n", ui.warn("!"), wmsg)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			p, err := local.Start(ctx, d, o)
			if err != nil {
				return err
			}
			p.Banner(cmd.OutOrStdout(), d.Title, len(d.Slides), ui.on)
			if o.Hub != "" {
				if err := startRelay(ctx, cmd, p, args[0], o); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s hub: %v (local presentation continues)\n", ui.warn("!"), err)
				}
			}
			if o.Open {
				if err := local.OpenBrowser(p.URLs.Stage); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s cannot open the browser: %v\n", ui.warn("!"), err)
				}
			}
			return p.Serve(ctx)
		},
	}
	c.Flags().IntVar(&o.Port, "port", 7777, "port to listen on (next free one if taken)")
	c.Flags().StringVar(&o.IP, "ip", "", "LAN IP to show in the links (auto-detected)")
	c.Flags().BoolVar(&o.Open, "open", false, "open the stage in the default browser")
	c.Flags().BoolVar(&o.NoLAN, "no-lan", false, "listen on 127.0.0.1 only")
	c.Flags().StringVar(&o.Hub, "hub", "", "relay the presentation to a Deckhand hub (https://…)")
	c.Flags().StringVar(&o.HubSlug, "slug", "", "deck slug on the hub (default: from the title)")
	return c
}
