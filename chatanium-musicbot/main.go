package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc/metadata"

	broker "github.com/thirdscam/chatanium-flexmodule/shared"

	Core "github.com/thirdscam/chatanium-flexmodule/shared/core-v1"
	CorePlugin "github.com/thirdscam/chatanium-flexmodule/shared/core-v1/module"

	pb "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
	Discord "github.com/thirdscam/chatanium-flexmodule/shared/discord-v1"
	DiscordPlugin "github.com/thirdscam/chatanium-flexmodule/shared/discord-v1/module"

	Provider "github.com/thirdscam/chatanium-flexmodule/chatanium-musicbot/provider"
)

var PERMISSIONS = Core.Permissions{
	"DISCORD_V1_ON_CREATE_INTERACTION",
	"DISCORD_V1_REQ_VOICE_STATE",
	"DISCORD_V1_CREATE_VOICE_STREAM",
}

var MANIFEST = Core.Manifest{
	Name:        "MusicBot",
	Version:     "0.2.0",
	Author:      "ANTEGRAL",
	Repository:  "github:thirdscam/chatanium-flexmodule",
	Permissions: PERMISSIONS,
}

var log hclog.Logger

// providers holds the music providers (youtube, etc.)
var providers map[string]Provider.Interface = make(map[string]Provider.Interface)

// core implements the core-v1 plugin interface
type core struct {
	ready bool
}

func (c *core) GetManifest() (Core.Manifest, error) {
	return MANIFEST, nil
}

func (c *core) GetStatus() (Core.Status, error) {
	return Core.Status{
		IsReady: c.ready,
	}, nil
}

func (c *core) OnStage(stage string) {
	log.Debug("OnStage", "stage", stage)
	switch stage {
	case "MODULE_INIT":
		// Initialize providers
		providers = Provider.GetProviders()
		for k, v := range providers {
			log.Info("Starting provider", "name", k)
			if err := v.Start(); err != nil {
				log.Error("Failed to start provider", "name", k, "error", err)
				return
			}
		}
		// Create temp directory for audio files
		os.MkdirAll(MUSIC_PATH, 0o755)
		c.ready = true
	case "MODULE_SHUTDOWN":
		cancelMedia()
		// Cleanup temp directory
		os.RemoveAll(MUSIC_PATH)
	}
}

// musicBot implements the discord-v1 plugin interface
type musicBot struct {
	Discord.AbstractHooks
	helper      Discord.Helper
	voiceStream pb.VoiceStreamClient
}

func (m *musicBot) OnInit(h Discord.Helper) Discord.InitResponse {
	m.helper = h
	log.Info("MusicBot module initialized")

	return Discord.InitResponse{
		Interactions: []*discordgo.ApplicationCommand{
			{
				Name:        "play",
				Description: "Play music",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "provider",
						Description: "Enter a provider of music",
						Required:    true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{
								Name:  "youtube",
								Value: "youtube",
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "query",
						Description: "Enter a query to search",
						Required:    true,
					},
				},
			},
			{
				Name:        "remove",
				Description: "Remove a music from playlist",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionInteger,
						Name:        "index",
						Description: "Enter a index of music",
						Required:    true,
					},
				},
			},
			{
				Name:        "list",
				Description: "Show playlist",
			},
			{
				Name:        "pause",
				Description: "Pause/Resume music",
			},
			{
				Name:        "skip",
				Description: "Skip music",
			},
			{
				Name:        "loop",
				Description: "Loop music",
			},
		},
	}
}

// SetVoiceStream implements VoiceStreamAware interface
func (m *musicBot) SetVoiceStream(stream pb.VoiceStreamClient) {
	m.voiceStream = stream
	log.Debug("VoiceStream client set successfully")
}

func (m *musicBot) OnCreateInteraction(i *discordgo.Interaction) error {
	if i.Type != discordgo.InteractionApplicationCommand {
		return nil
	}

	data := i.ApplicationCommandData()

	switch data.Name {
	case "play":
		go m.handlePlay(i, data)
	case "remove":
		m.handleRemove(i, data)
	case "list":
		m.handleList(i)
	case "pause":
		m.handlePause(i)
	case "skip":
		m.handleSkip(i)
	case "loop":
		m.handleLoop(i)
	}

	return nil
}

func (m *musicBot) handlePlay(i *discordgo.Interaction, data discordgo.ApplicationCommandInteractionData) {
	log.Info("Play command called",
		"user", i.Member.User.Username,
		"channel", i.ChannelID,
		"query", data.Options[1].StringValue(),
	)

	// Send initial response
	m.ephemeralRespond(i, "**Adding song to queue...**\nIf you enter a playlist, it might take a while for the entire contents to import.\n(The first song will automatically play when it's ready.)")

	// Get the query options
	queryType := data.Options[0].StringValue()
	query := data.Options[1].StringValue()

	// Get the provider
	var provider Provider.Interface
	switch queryType {
	case "youtube":
		provider = &Provider.Youtube{}
	default:
		m.editResponse(i, "**Invalid provider name!**\nPlease input a valid provider name.")
		return
	}

	// Find user's voice channel
	channelID := m.getVoiceChannelByUser(i.GuildID, i.Member.User.ID)
	if channelID == "" {
		m.editResponse(i, "**Failed to find voice channel.**\nPlease rejoin the voice channel and try again. (or you're not in a voice channel)")
		return
	}

	// Get the music
	musics, err := provider.GetMusic(mediaContext, query)
	if err != nil {
		log.Error("Failed to query music", "error", err)
		m.editResponse(i, "**Failed to query music.**\nPlease try again or input another query.")
		return
	}

	// Create voice client
	voiceClient := DiscordPlugin.NewVoiceClient(m.voiceStream, "musicbot")

	// Join the voice channel
	ctx := getContextWithModuleID("musicbot")
	err = voiceClient.Join(ctx, i.GuildID, channelID, false, true)
	if err != nil {
		log.Error("Failed to join voice channel", "error", err)
		m.editResponse(i, "**Failed to join voice channel.**\nPlease try again.")
		return
	}
	log.Info("Joined voice channel", "channel", channelID)

	// Music download thread
	isReady := make(chan error, 1)

	go func() {
		var respMsg string

		for j, music := range musics {
			// Download file
			err := DownloadMusic(music.RawUrl, music.Id)
			if err != nil {
				m.editResponse(i, "**Failed to download music.**\nPlease try again.")
				if j == 0 {
					isReady <- err
				}
				return
			}

			log.Info("Downloaded music", "index", j+1, "total", len(musics), "title", music.Title)
			GetState(ChannelID(channelID)).Enqueue(music)

			// Update the response message
			if j == 0 {
				respMsg += fmt.Sprintf("**Added to queue:**\n-> **%s**", music.Title)
				isReady <- nil
			} else {
				respMsg += fmt.Sprintf("\n-> %s", music.Title)
			}

			m.editResponse(i, respMsg)
			time.Sleep(time.Second * 10) // prevent rate limit
		}
	}()
	if err := <-isReady; err != nil {
		return
	}

	// Play the music if not already playing
	state := GetState(ChannelID(channelID))
	if !state.IsPlaying() {
		m.playMusic(voiceClient, channelID, i.ChannelID)
	}
}

func (m *musicBot) handleRemove(i *discordgo.Interaction, data discordgo.ApplicationCommandInteractionData) {
	index := data.Options[0].IntValue()
	log.Info("Remove command", "index", index)

	channelID := m.getVoiceChannelByUser(i.GuildID, i.Member.User.ID)
	if channelID == "" {
		m.ephemeralRespond(i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	if index <= 0 {
		m.ephemeralRespond(i, "**Invalid index!**\nOnly positive integers are allowed.\n(The 0th song is the currently playing song, so you should use /skip)")
		return
	}

	music, err := GetState(ChannelID(channelID)).Remove(int(index - 1))

	if err == errEmptyQueue {
		m.ephemeralRespond(i, "**Cannot find queue!**\nPlease play a song first.")
		return
	}

	if err == errIndexOutOfRange {
		m.ephemeralRespond(i, "**Invalid index!**\nPlease input a valid index.")
		return
	}

	m.ephemeralRespond(i, fmt.Sprintf("Removing: **#%d** - %s", index, music.Title))
}

func (m *musicBot) handleList(i *discordgo.Interaction) {
	channelID := m.getVoiceChannelByUser(i.GuildID, i.Member.User.ID)
	if channelID == "" {
		m.ephemeralRespond(i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	queue := GetState(ChannelID(channelID)).GetQueue()
	if len(queue) == 0 {
		m.ephemeralRespond(i, "**Queue is empty!**\nPlease play a song first.")
		return
	}

	respMsg := fmt.Sprintf("**Now Playing: %s**\n\nQueue:\n", queue[0].Title)
	for idx, music := range queue {
		if idx == 0 {
			continue
		}
		respMsg += fmt.Sprintf("**#%d** - %s\n", idx, music.Title)
	}

	m.ephemeralRespond(i, respMsg)
}

func (m *musicBot) handlePause(i *discordgo.Interaction) {
	channelID := m.getVoiceChannelByUser(i.GuildID, i.Member.User.ID)
	if channelID == "" {
		m.ephemeralRespond(i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	err := GetState(ChannelID(channelID)).Pause()
	if err == errSignalTimeout {
		m.ephemeralRespond(i, "**Failed to pause/resume music.**\nPlease try again.")
		log.Warn("Failed to pause/resume music (channel timeout)")
		return
	}

	m.ephemeralRespond(i, "**Music paused/resumed.**")
}

func (m *musicBot) handleSkip(i *discordgo.Interaction) {
	channelID := m.getVoiceChannelByUser(i.GuildID, i.Member.User.ID)
	if channelID == "" {
		m.ephemeralRespond(i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	err := GetState(ChannelID(channelID)).Skip()
	if err == errSignalTimeout {
		m.ephemeralRespond(i, "**Failed to skip music.**\nPlease try again.")
		log.Warn("Failed to skip music (channel timeout)")
		return
	}

	m.ephemeralRespond(i, "**Music skipped.**")
}

func (m *musicBot) handleLoop(i *discordgo.Interaction) {
	channelID := m.getVoiceChannelByUser(i.GuildID, i.Member.User.ID)
	if channelID == "" {
		m.ephemeralRespond(i, "**Failed to find voice channel.** (or you're not in a voice channel)\nPlease rejoin the voice channel and try again.")
		return
	}

	isLoopMode := GetState(ChannelID(channelID)).ToggleLoop()

	message := "Loop mode is now off."
	if isLoopMode {
		message = "Loop mode is now on."
	}

	m.ephemeralRespond(i, fmt.Sprintf("**Loop mode switched!**\n%s", message))
}

func (m *musicBot) playMusic(voiceClient *DiscordPlugin.VoiceClient, voiceChannelID, textChannelID string) {
	defer voiceClient.Leave()

	// Wait for connection to be ready
	err := voiceClient.WaitForReady(5 * time.Second)
	if err != nil {
		log.Error("Voice connection not ready", "error", err)
		m.helper.ChannelMessageSend(textChannelID, "**Voice connection failed.**")
		return
	}

	for {
		state := GetState(ChannelID(voiceChannelID))
		state.SetIsPlaying(true)

		nowMusic := state.GetFront()

		if state.IsQueueEmpty() {
			state.SetIsPlaying(false)
			RemoveStatusEmbed(m.helper, textChannelID)
			return
		}

		// Update status embed
		SetStatusEmbed(m.helper, textChannelID, EmbedState{
			Title:        nowMusic.Title,
			ThumbnailUrl: nowMusic.ThumbnailUrl,
		})

		// Start playing
		log.Info("Playing music", "title", nowMusic.Title)

		err := voiceClient.Speaking(true)
		if err != nil {
			log.Error("Failed to set speaking state", "error", err)
		}

		PlayMusic(voiceClient, nowMusic.Id, state.pause, state.skip)

		voiceClient.Speaking(false)

		// Handle queue after playback
		state.Lock()
		front := state.Pop()

		if state.loop {
			state.queue = append(state.queue, front)
		}

		// Check for duplicates before removing file
		isDuplicated := false
		for _, music := range state.queue {
			if music.Id == front.Id {
				isDuplicated = true
				break
			}
		}

		if !isDuplicated {
			RemoveMusic(nowMusic.Id)
		}
		state.Unlock()

		time.Sleep(1 * time.Second)
	}
}

func (m *musicBot) getVoiceChannelByUser(guildID, userID string) string {
	guild, err := m.helper.Guild(guildID)
	if err != nil {
		log.Warn("Failed to get guild", "error", err)
		return ""
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			log.Debug("Found user in voice channel", "channel", vs.ChannelID, "user", vs.UserID)
			return vs.ChannelID
		}
	}

	return ""
}

func (m *musicBot) ephemeralRespond(i *discordgo.Interaction, content string) {
	log.Debug("ephemeralRespond", "content", content)
	m.helper.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	log.Debug("ephemeralRespond", "done")
}

func (m *musicBot) editResponse(i *discordgo.Interaction, content string) {
	m.helper.InteractionResponseEdit(i, &discordgo.WebhookEdit{
		Content: &content,
	})
}

func getContextWithModuleID(moduleID string) context.Context {
	ctx := context.Background()
	md := metadata.New(map[string]string{
		"module-id": moduleID,
	})
	return metadata.NewOutgoingContext(ctx, md)
}

func main() {
	log = hclog.New(&hclog.LoggerOptions{
		Name:       "MusicBot",
		Level:      hclog.LevelFromString("DEBUG"),
		JSONFormat: true,
		Output:     os.Stderr,
	})

	// Set logger for discord module package to enable error logging before termination
	DiscordPlugin.SetLogger(log)

	broker.ServeToRuntime(map[string]plugin.Plugin{
		"core-v1":    &CorePlugin.Plugin{Impl: &core{}},
		"discord-v1": &DiscordPlugin.Plugin{Impl: &musicBot{}},
	})
}
