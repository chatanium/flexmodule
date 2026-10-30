//go:build !voice || !cgo

package main

import (
	"context"
	"encoding/json"
	"errors"

	discordbackend "github.com/thirdscam/chatanium-flexmodule/internal/backend/discord"
)

const voiceEnabled = false

func attachVoice(_ context.Context, _ *discordbackend.Backend, raw json.RawMessage, _ int) error {
	if len(raw) != 0 {
		return errors.New("voice configuration requires the voice-enabled CGO artifact")
	}
	return nil
}
