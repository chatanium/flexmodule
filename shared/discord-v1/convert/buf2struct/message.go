package buf2struct

import (
	"github.com/bwmarrin/discordgo"
	proto "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
	"github.com/thirdscam/chatanium-flexmodule/util"
)

func Message(buf *proto.Message) *discordgo.Message {
	if buf == nil {
		return nil
	}

	attachments := make([]*discordgo.MessageAttachment, 0)
	for _, attachment := range buf.Attachments {
		attachments = append(attachments, MessageAttachment(attachment))
	}

	mentionChannels := make([]*discordgo.Channel, 0)
	for _, channel := range buf.MentionChannels {
		mentionChannels = append(mentionChannels, Channel(channel))
	}

	embeds := make([]*discordgo.MessageEmbed, 0)
	for _, embed := range buf.Embeds {
		embeds = append(embeds, MessageEmbed(embed))
	}

	var activity *discordgo.MessageActivity
	if buf.Activity != nil {
		activity = &discordgo.MessageActivity{
			Type:    discordgo.MessageActivityType(buf.Activity.Type),
			PartyID: buf.Activity.PartyId,
		}
	}

	var application *discordgo.MessageApplication
	if buf.Application != nil {
		application = &discordgo.MessageApplication{
			ID:          buf.Application.Id,
			CoverImage:  buf.Application.CoverImage,
			Description: buf.Application.Description,
			Icon:        buf.Application.Icon,
			Name:        buf.Application.Name,
		}
	}

	return &discordgo.Message{
		ID:              buf.Id,
		ChannelID:       buf.ChannelId,
		GuildID:         buf.GuildId,
		Content:         buf.Content,
		Timestamp:       buf.Timestamp.AsTime(),
		EditedTimestamp: util.PbTimestamp2AsTimePtr(buf.EditedTimestamp),
		MentionRoles:    buf.MentionRoles,
		TTS:             buf.Tts,
		MentionEveryone: buf.MentionEveryone,
		Author:          User(buf.Author),
		Attachments:     attachments,
		Components:      MessageComponents(buf.Components),
		Embeds:          embeds,
		Reactions:       nil,
		Pinned:          buf.Pinned,
		Type:            discordgo.MessageType(buf.Type),
		WebhookID:       buf.WebhookId,
		Member:          Member(buf.Member),
		MentionChannels: mentionChannels,
		Activity:        activity,
		Application:     application,
	}
}

func MessageAttachment(buf *proto.MessageAttachment) *discordgo.MessageAttachment {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageAttachment{
		ID:          buf.Id,
		URL:         buf.Url,
		ProxyURL:    buf.ProxyUrl,
		Filename:    buf.Filename,
		ContentType: buf.ContentType,
		Width:       int(buf.Width),
		Height:      int(buf.Height),
		Size:        int(buf.Size),
		Ephemeral:   buf.Ephemeral,
	}
}

func MessageEmbed(buf *proto.MessageEmbed) *discordgo.MessageEmbed {
	if buf == nil {
		return nil
	}

	messageEmbedField := make([]*discordgo.MessageEmbedField, 0)
	for _, field := range buf.Fields {
		messageEmbedField = append(messageEmbedField, MessageEmbedField(field))
	}

	return &discordgo.MessageEmbed{
		URL:         buf.Url,
		Type:        discordgo.EmbedType(buf.Type),
		Title:       buf.Title,
		Description: buf.Description,
		Timestamp:   buf.Timestamp,
		Color:       int(buf.Color),
		Footer:      MessageEmbedFooter(buf.Footer),
		Image:       MessageEmbedImage(buf.Image),
		Thumbnail:   MessageEmbedThumbnail(buf.Thumbnail),
		Video:       MessageEmbedVideo(buf.Video),
		Provider:    MessageEmbedProvider(buf.Provider),
		Author:      MessageEmbedAuthor(buf.Author),
		Fields:      messageEmbedField,
	}
}

func MessageEmbedField(buf *proto.MessageEmbedField) *discordgo.MessageEmbedField {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageEmbedField{
		Name:   buf.Name,
		Value:  buf.Value,
		Inline: buf.Inline,
	}
}

func MessageEmbedFooter(buf *proto.MessageEmbedFooter) *discordgo.MessageEmbedFooter {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageEmbedFooter{
		Text:         buf.Text,
		IconURL:      buf.IconUrl,
		ProxyIconURL: buf.ProxyIconUrl,
	}
}

func MessageEmbedImage(buf *proto.MessageEmbedImage) *discordgo.MessageEmbedImage {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageEmbedImage{
		URL:      buf.Url,
		ProxyURL: buf.ProxyUrl,
		Height:   int(buf.Height),
		Width:    int(buf.Width),
	}
}

func MessageEmbedThumbnail(buf *proto.MessageEmbedThumbnail) *discordgo.MessageEmbedThumbnail {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageEmbedThumbnail{
		URL:      buf.Url,
		ProxyURL: buf.ProxyUrl,
		Height:   int(buf.Height),
		Width:    int(buf.Width),
	}
}

func MessageEmbedVideo(buf *proto.MessageEmbedVideo) *discordgo.MessageEmbedVideo {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageEmbedVideo{
		URL:    buf.Url,
		Height: int(buf.Height),
		Width:  int(buf.Width),
	}
}

func MessageEmbedProvider(buf *proto.MessageEmbedProvider) *discordgo.MessageEmbedProvider {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageEmbedProvider{
		URL:  buf.Url,
		Name: buf.Name,
	}
}

func MessageEmbedAuthor(buf *proto.MessageEmbedAuthor) *discordgo.MessageEmbedAuthor {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageEmbedAuthor{
		URL:          buf.Url,
		Name:         buf.Name,
		IconURL:      buf.IconUrl,
		ProxyIconURL: buf.ProxyIconUrl,
	}
}

func MessageReactions(buf *proto.MessageReactions) *discordgo.MessageReactions {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageReactions{
		Count: int(buf.Count),
		Me:    buf.Me,
		Emoji: Emoji(buf.Emoji),
	}
}

func MessageSend(buf *proto.MessageSend) *discordgo.MessageSend {
	if buf == nil {
		return nil
	}

	embeds := make([]*discordgo.MessageEmbed, 0, len(buf.Embeds))
	for _, embed := range buf.Embeds {
		embeds = append(embeds, MessageEmbed(embed))
	}

	files := make([]*discordgo.File, 0, len(buf.Files))
	for _, file := range buf.Files {
		files = append(files, &discordgo.File{
			Name:        file.Name,
			ContentType: file.ContentType,
		})
	}

	stickerIDs := append([]string(nil), buf.StickerIds...)

	return &discordgo.MessageSend{
		Content:    buf.Content,
		Embeds:     embeds,
		TTS:        buf.Tts,
		Files:      files,
		StickerIDs: stickerIDs,
		// Note: AllowedMentions and Reference need conversion functions
		// Flags:      discordgo.MessageFlags(buf.Flags),
	}
}

func MessageEdit(buf *proto.MessageEdit) *discordgo.MessageEdit {
	if buf == nil {
		return nil
	}

	var embeds *[]*discordgo.MessageEmbed
	if len(buf.Embeds) > 0 {
		embedSlice := make([]*discordgo.MessageEmbed, 0, len(buf.Embeds))
		for _, embed := range buf.Embeds {
			embedSlice = append(embedSlice, MessageEmbed(embed))
		}
		embeds = &embedSlice
	}

	files := make([]*discordgo.File, 0, len(buf.Files))
	for _, file := range buf.Files {
		files = append(files, &discordgo.File{
			Name:        file.Name,
			ContentType: file.ContentType,
		})
	}

	var attachments *[]*discordgo.MessageAttachment
	if len(buf.Attachments) > 0 {
		attachmentSlice := make([]*discordgo.MessageAttachment, 0, len(buf.Attachments))
		for _, attachment := range buf.Attachments {
			attachmentSlice = append(attachmentSlice, MessageAttachment(attachment))
		}
		attachments = &attachmentSlice
	}

	var content *string
	if buf.Content != "" {
		content = &buf.Content
	}

	return &discordgo.MessageEdit{
		Content:     content,
		Embeds:      embeds,
		Files:       files,
		Attachments: attachments,
		ID:          buf.Id,
		Channel:     buf.Channel,
		// Note: AllowedMentions and Flags need proper handling
	}
}
