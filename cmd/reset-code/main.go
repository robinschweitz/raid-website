// Command reset-code issues one password-reset code without an HTTP surface.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/config"
	"raid-website/internal/db"
	"raid-website/internal/player"
	"raid-website/internal/registration"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) (err error) {
	flags := flag.NewFlagSet("reset-code", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.Int64("player-id", 0, "Player ID")
	name := flags.String("player", "", "Exact player name or login")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || ((*id > 0) == (*name != "")) {
		return errors.New("supply exactly one of --player-id ID or --player NAME")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	service := registration.NewService(database.DB, player.NewRepo(database.DB), character.NewRepo(database.DB), audit.NewRepo(database.DB), nil, nil)
	result, err := service.CreateOperatorResetCode(ctx, *id, *name)
	if err != nil {
		return err
	}
	slog.New(slog.NewJSONHandler(os.Stderr, nil)).Warn("security_event", "event", "reset_code_created", "actor", "server operator", "player_id", result.Record.PlayerID)
	_, err = fmt.Fprintln(out, result.Code)
	return err
}
