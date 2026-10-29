//go:build !voice

package discordgo

func VoiceAvailable() bool                                 { return false }
func newVoiceEngine(*VoiceConnection) (voiceEngine, error) { return nil, ErrVoiceUnavailable }
