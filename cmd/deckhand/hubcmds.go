package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/stranix79/deckhand/internal/deck"
	"github.com/stranix79/deckhand/internal/hub"
	"github.com/stranix79/deckhand/internal/local"
)

func serveCmd() *cobra.Command {
	var addr, pg, deckOrigin, baseURL string
	c := &cobra.Command{
		Use:   "serve",
		Short: "Run a Deckhand hub (multi-user, PostgreSQL, see docs/HUB.md)",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg := hub.FromEnv()
			if addr != "" {
				cfg.Addr = addr
			}
			if pg != "" {
				cfg.PG = pg
			}
			if deckOrigin != "" {
				cfg.DeckOrigin = strings.TrimRight(deckOrigin, "/")
			}
			if baseURL != "" {
				cfg.BaseURL = strings.TrimRight(baseURL, "/")
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return hub.Serve(ctx, cfg)
		},
	}
	c.Flags().StringVar(&addr, "addr", "", "listen address (DECKHAND_ADDR, default :8080)")
	c.Flags().StringVar(&pg, "pg", "", "PostgreSQL URL (DECKHAND_PG)")
	c.Flags().StringVar(&deckOrigin, "deck-origin", "", "origin serving deck files (DECKHAND_DECK_ORIGIN)")
	c.Flags().StringVar(&baseURL, "base-url", "", "public URL of the hub (DECKHAND_BASE_URL)")
	return c
}

func loginCmd() *cobra.Command {
	var hubURL, token string
	c := &cobra.Command{
		Use:   "login --hub https://… --token …",
		Short: "Save a hub API token in ~/.config/deckhand/config.toml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if hubURL == "" || token == "" {
				return errors.New("both --hub and --token are required (get a token on the hub: Decks → Get an API token)")
			}
			hc := &local.HubClient{URL: hubURL, Token: token}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			email, handle, plan, err := hc.Me(ctx)
			if err != nil {
				return err
			}
			cfg, _ := local.LoadConfig()
			cfg.Hub.URL = strings.TrimRight(hubURL, "/")
			cfg.Hub.Token = token
			if err := local.SaveConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s signed in as %s (%s, %s plan) — saved to %s\n", ui.ok("✓"), email, handle, plan, local.ConfigPath())
			return nil
		},
	}
	c.Flags().StringVar(&hubURL, "hub", "", "hub URL")
	c.Flags().StringVar(&token, "token", "", "API token")
	return c
}

func pushCmd() *cobra.Command {
	var hubURL, slug string
	c := &cobra.Command{
		Use:   "push <deck>",
		Short: "Publish a deck on a hub and get its permanent link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := deck.Load(args[0])
			if err != nil {
				printReport(cmd.OutOrStdout(), args[0], nil, deck.AsReport(err), false)
				os.Exit(1)
			}
			_ = d.Close()
			hc, err := hubClient(hubURL)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			res, err := hc.Push(ctx, args[0], slug)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s v%d (%d slides) pushed\n  %s\n", ui.ok("✓"), ui.bold(res.Title), res.Version, res.Slides, ui.ok(res.URL))
			if res.ExpiresAt != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", ui.dim("free plan: this link expires on "+res.ExpiresAt.Format("2006-01-02")))
			}
			return nil
		},
	}
	c.Flags().StringVar(&hubURL, "hub", "", "hub URL (default: from config)")
	c.Flags().StringVar(&slug, "slug", "", "deck slug (default: from the title)")
	return c
}

// hubClient builds a client from --hub and the saved config.
func hubClient(hubURL string) (*local.HubClient, error) {
	cfg, err := local.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", local.ConfigPath(), err)
	}
	if hubURL == "" {
		hubURL = cfg.Hub.URL
	}
	if hubURL == "" {
		return nil, errors.New("no hub configured: run `deckhand login --hub https://… --token …`")
	}
	if cfg.Hub.Token == "" {
		return nil, errors.New("no API token: run `deckhand login --hub " + hubURL + " --token …` (token from the hub: Decks → Get an API token)")
	}
	return &local.HubClient{URL: strings.TrimRight(hubURL, "/"), Token: cfg.Hub.Token}, nil
}
