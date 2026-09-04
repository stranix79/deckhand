package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/stranix79/deckhand/internal/local"
)

// startRelay connects the local presentation to a hub (milestone 4).
func startRelay(_ context.Context, _ *cobra.Command, _ *local.Presentation, _ string, _ local.Options) error {
	return errors.New("hub relay is not available yet (milestone 4)")
}
