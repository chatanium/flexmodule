package main

import (
	"github.com/bwmarrin/discordgo"
	Discord "github.com/thirdscam/chatanium-flexmodule/shared/discord-v1"
)

type EmbedState struct {
	messageID    string
	Title        string
	ThumbnailUrl string
}

var metadatas = map[string]EmbedState{}

func SendStatusEmbed(h Discord.Helper, channelID string, form EmbedState) string {
	embed := &discordgo.MessageEmbed{
		Title:       "Now Playing",
		Description: form.Title,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: form.ThumbnailUrl,
		},
		Color: 0x9f7fed,
	}

	m, err := h.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return ""
	}

	metadatas[m.ID] = EmbedState{
		messageID:    m.ID,
		Title:        form.Title,
		ThumbnailUrl: form.ThumbnailUrl,
	}

	return m.ID
}

func SetStatusEmbed(h Discord.Helper, channelID string, form EmbedState) string {
	embed := &discordgo.MessageEmbed{
		Title:       "Now Playing",
		Description: form.Title,
		Color:       0x9f7fed,
	}

	existingState, exists := metadatas[channelID]
	if !exists || existingState.messageID == "" {
		newMessageID := SendStatusEmbed(h, channelID, form)
		metadatas[channelID] = EmbedState{
			messageID:    newMessageID,
			Title:        form.Title,
			ThumbnailUrl: form.ThumbnailUrl,
		}
		return newMessageID
	}

	m, err := h.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel: channelID,
		ID:      existingState.messageID,
		Embeds:  &[]*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		newMessageID := SendStatusEmbed(h, channelID, form)
		metadatas[channelID] = EmbedState{
			messageID:    newMessageID,
			Title:        form.Title,
			ThumbnailUrl: form.ThumbnailUrl,
		}
		return newMessageID
	}

	metadatas[m.ID] = EmbedState{
		messageID:    m.ID,
		Title:        form.Title,
		ThumbnailUrl: form.ThumbnailUrl,
	}

	return m.ID
}

func RemoveStatusEmbed(h Discord.Helper, channelID string) {
	existingState, exists := metadatas[channelID]
	if !exists || existingState.messageID == "" {
		return
	}

	err := h.ChannelMessageDelete(channelID, existingState.messageID)
	if err != nil {
		log.Warn("Failed to remove embed", "error", err)
	}

	delete(metadatas, channelID)
}
