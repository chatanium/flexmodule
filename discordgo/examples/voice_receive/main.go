package main

import (
	"context"
	"encoding/json"
	"os"
	"flag"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// Variables used for command line parameters
var (
	Token     string
	ChannelID string
	GuildID   string
	VoiceConfigPath string
	VoiceConfig discordgo.VoiceConfig
)

func init() {
	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.StringVar(&GuildID, "g", "", "Guild in which voice channel exists")
	flag.StringVar(&ChannelID, "c", "", "Voice channel to connect to")
	flag.StringVar(&VoiceConfigPath, "voice-config", "", "Required JSON file containing explicit discordgo.VoiceConfig bounds (durations in nanoseconds)")
	flag.Parse()
}

func createPionRTPPacket(p *discordgo.Packet) *rtp.Packet {
	// DiscordGo already strips RTP extensions from Opus; do not copy their IDs
	// into the fresh header used only for Ogg recording.
	return &rtp.Packet{
		Header: rtp.Header{
			Version: 2,
			// Taken from Discord voice docs
			PayloadType:    0x78,
			SequenceNumber: p.Sequence,
			Timestamp:      p.Timestamp,
			SSRC:           p.SSRC,
		},
		Payload: p.Opus,
	}
}

func handleVoice(ctx context.Context, voice *discordgo.VoiceConnection) error {
	files := make(map[uint32]media.Writer)
	defer func() {
		for ssrc, file := range files {
			if err := file.Close(); err != nil {
				fmt.Printf("failed to close file %d.ogg: %v\n", ssrc, err)
			}
		}
	}()

	for {
		p, err := voice.ReceiveOpus(ctx)
		if err != nil { return err }
		file, ok := files[p.SSRC]
		if !ok {
			file, err = oggwriter.New(fmt.Sprintf("%d.ogg", p.SSRC), 48000, 2)
			if err != nil { return fmt.Errorf("create recording %d: %w", p.SSRC, err) }
			files[p.SSRC] = file
		}
		if err = file.WriteRTP(createPionRTPPacket(p)); err != nil {
			return fmt.Errorf("write recording %d: %w", p.SSRC, err)
		}
	}
}

func main() {
	if !discordgo.VoiceAvailable() { fmt.Println(discordgo.ErrVoiceUnavailable); return }
	configBytes, err := os.ReadFile(VoiceConfigPath)
	if err == nil { err = json.Unmarshal(configBytes, &VoiceConfig) }
	if err == nil { err = VoiceConfig.Validate() }
	if err != nil { fmt.Println("invalid required voice configuration:", err); return }
	s, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("error creating Discord session:", err)
		return
	}
	defer s.Close()

	// We only really care about receiving voice state updates.
	s.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildVoiceStates)

	err = s.Open()
	if err != nil {
		fmt.Println("error opening connection:", err)
		return
	}

	v, err := s.ChannelVoiceJoinContext(context.Background(), GuildID, ChannelID, true, false, VoiceConfig)
	if err != nil {
		fmt.Println("failed to join voice channel:", err)
		return
	}

	defer func() {
		if err := v.Disconnect(); err != nil {
			fmt.Println("failed to disconnect from voice channel:", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := handleVoice(ctx, v); err != nil && ctx.Err() == nil {
		fmt.Println("voice receive failed:", err)
	}
}
