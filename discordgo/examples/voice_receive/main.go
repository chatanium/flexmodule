package main

import (
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
)

func init() {
	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.StringVar(&GuildID, "g", "", "Guild in which voice channel exists")
	flag.StringVar(&ChannelID, "c", "", "Voice channel to connect to")
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

func handleVoice(c <-chan *discordgo.Packet, done <-chan time.Time) {
	files := make(map[uint32]media.Writer)
	defer func() {
		for ssrc, file := range files {
			if err := file.Close(); err != nil {
				fmt.Printf("failed to close file %d.ogg: %v\n", ssrc, err)
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case p, ok := <-c:
			if !ok {
				return
			}
			file, ok := files[p.SSRC]
			if !ok {
				var err error
				file, err = oggwriter.New(fmt.Sprintf("%d.ogg", p.SSRC), 48000, 2)
				if err != nil {
					fmt.Printf("failed to create file %d.ogg, giving up on recording: %v\n", p.SSRC, err)
					return
				}
				files[p.SSRC] = file
			}
			// Construct pion RTP packet from DiscordGo's type.
			rtp := createPionRTPPacket(p)
			err := file.WriteRTP(rtp)
			if err != nil {
				fmt.Printf("failed to write to file %d.ogg: %v\n", p.SSRC, err)
			}
		}
	}
}

func main() {
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

	v, err := s.ChannelVoiceJoin(GuildID, ChannelID, true, false)
	if err != nil {
		fmt.Println("failed to join voice channel:", err)
		return
	}

	defer func() {
		if err := v.Disconnect(); err != nil {
			fmt.Println("failed to disconnect from voice channel:", err)
		}
	}()

	// Stop receiving without closing the channel owned by DiscordGo's sender.
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	handleVoice(v.OpusRecv, timer.C)
}
