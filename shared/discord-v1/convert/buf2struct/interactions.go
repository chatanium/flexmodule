package buf2struct

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	proto "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
	"github.com/thirdscam/chatanium-flexmodule/util"
)

func ApplicationCommand(buf *proto.ApplicationCommand) *discordgo.ApplicationCommand {
	if buf == nil {
		return nil
	}

	nameLocalizations := make(map[discordgo.Locale]string)
	if buf.NameLocalizations != nil {
		for k, v := range buf.NameLocalizations {
			nameLocalizations[util.StringToLocale(k)] = v
		}
	}

	descriptionLocalizations := make(map[discordgo.Locale]string)
	if buf.DescriptionLocalizations != nil {
		for k, v := range buf.DescriptionLocalizations {
			descriptionLocalizations[util.StringToLocale(k)] = v
		}
	}

	options := make([]*discordgo.ApplicationCommandOption, 0)
	for _, v := range buf.Options {
		options = append(options, ApplicationCommandOption(v))
	}

	return &discordgo.ApplicationCommand{
		ID:                buf.Id,
		ApplicationID:     buf.ApplicationId,
		GuildID:           buf.GuildId,
		Version:           buf.Version,
		Type:              discordgo.ApplicationCommandType(buf.Type),
		Name:              buf.Name,
		NameLocalizations: &nameLocalizations,
		// DefaultPermission:        buf.DefaultPermission, // Deprecated
		DefaultMemberPermissions: &buf.DefaultMemberPermissions,
		// DMPermission:             buf.DmPermission, // Deprecated
		NSFW:                     buf.Nsfw,
		Description:              buf.Description,
		DescriptionLocalizations: &descriptionLocalizations,
		Options:                  options,
	}
}

func ApplicationCommandOption(buf *proto.ApplicationCommandOption) *discordgo.ApplicationCommandOption {
	if buf == nil {
		return nil
	}

	options := make([]*discordgo.ApplicationCommandOption, 0)
	for _, v := range buf.Options {
		options = append(options, ApplicationCommandOption(v))
	}

	nameLocalizations := make(map[discordgo.Locale]string)
	if buf.NameLocalizations != nil {
		for k, v := range buf.NameLocalizations {
			nameLocalizations[util.StringToLocale(k)] = v
		}
	}

	descriptionLocalizations := make(map[discordgo.Locale]string)
	if buf.DescriptionLocalizations != nil {
		for k, v := range buf.DescriptionLocalizations {
			descriptionLocalizations[util.StringToLocale(k)] = v
		}
	}

	channelType := make([]discordgo.ChannelType, 0)
	for _, v := range buf.ChannelTypes {
		channelType = append(channelType, discordgo.ChannelType(v))
	}

	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0)
	for _, v := range buf.Choices {
		choices = append(choices, ApplicationCommandOptionChoice(v))
	}

	var maxValue float64
	if buf.MaxValue != nil {
		maxValue = *buf.MaxValue
	}

	var maxLength int
	if buf.MaxLength != nil {
		maxLength = *Int32PtrToIntPtr(buf.MaxLength)
	}

	return &discordgo.ApplicationCommandOption{
		Type:                     discordgo.ApplicationCommandOptionType(buf.Type),
		Name:                     buf.Name,
		NameLocalizations:        nameLocalizations,
		Description:              buf.Description,
		DescriptionLocalizations: descriptionLocalizations,
		Required:                 buf.Required,
		Choices:                  choices,
		Options:                  options,
		ChannelTypes:             channelType,
		MinValue:                 buf.MinValue,
		MaxValue:                 maxValue,
		MinLength:                Int32PtrToIntPtr(buf.MinLength),
		MaxLength:                maxLength,
	}
}

func ApplicationCommandOptionChoice(buf *proto.ApplicationCommandOptionChoice) *discordgo.ApplicationCommandOptionChoice {
	if buf == nil {
		return nil
	}

	var value any
	switch v := buf.Value.(type) {
	case *proto.ApplicationCommandOptionChoice_StringValue:
		value = v.StringValue
	case *proto.ApplicationCommandOptionChoice_IntegerValue:
		value = v.IntegerValue
	case *proto.ApplicationCommandOptionChoice_NumberValue:
		value = v.NumberValue
	default:
		fmt.Printf("ApplicationCommandOptionChoice > Unknown value type: %+v\n", v)
		value = nil
	}

	return &discordgo.ApplicationCommandOptionChoice{
		Name:  buf.Name,
		Value: value,
	}
}

func Interaction(buf *proto.Interaction) *discordgo.Interaction {
	if buf == nil {
		return nil
	}

	var guildLocale *discordgo.Locale
	if buf.GuildLocale != nil {
		gl := util.StringToLocale(*buf.GuildLocale)
		guildLocale = &gl
	}

	interaction := &discordgo.Interaction{
		ID:             buf.Id,
		AppID:          buf.AppId,
		Type:           discordgo.InteractionType(buf.Type),
		GuildID:        buf.GuildId,
		ChannelID:      buf.ChannelId,
		Message:        Message(buf.Message),
		AppPermissions: buf.AppPermissions,
		Member:         Member(buf.Member),
		User:           User(buf.User),
		Locale:         util.StringToLocale(buf.Locale),
		GuildLocale:    guildLocale,
		// Context:                      0,   // TODO(discord/bufstruct): Preparing for discordgo updates (^v0.28.1)
		// AuthorizingIntegrationOwners: nil, // TODO(discord/bufstruct): Preparing for discordgo updates (^v0.28.1)
		Token:   buf.Token,
		Version: int(buf.Version),
		// Entitlements:                 nil, // TODO(discord/bufstruct): Preparing for discordgo updates (^v0.28.1)
	}

	switch buf.Data.(type) {
	case *proto.Interaction_ApplicationCommandData:
		appCmdData := buf.Data.(*proto.Interaction_ApplicationCommandData).ApplicationCommandData
		interaction.Data = *ApplicationCommandInteractionData(appCmdData)
	case *proto.Interaction_MessageComponentData:
		msgCompData := buf.Data.(*proto.Interaction_MessageComponentData).MessageComponentData
		interaction.Data = *MessageComponentInteractionData(msgCompData)
	case *proto.Interaction_ModalSubmitData:
		modalSubmitData := buf.Data.(*proto.Interaction_ModalSubmitData).ModalSubmitData
		interaction.Data = *ModalSubmitInteractionData(modalSubmitData)
	default:
		fmt.Println("Interaction > Unknown interaction data type")
	}

	return interaction
}

func ApplicationCommandInteractionData(buf *proto.ApplicationCommandInteractionData) *discordgo.ApplicationCommandInteractionData {
	if buf == nil {
		return nil
	}

	options := make([]*discordgo.ApplicationCommandInteractionDataOption, 0)
	for _, v := range buf.Options {
		options = append(options, ApplicationCommandInteractionDataOption(v))
	}

	return &discordgo.ApplicationCommandInteractionData{
		ID:          buf.Id,
		Name:        buf.Name,
		CommandType: discordgo.ApplicationCommandType(buf.CommandType),
		Resolved:    ApplicationCommandInteractionDataResolved(buf.Resolved),
		Options:     options,
		TargetID:    buf.TargetId,
	}
}

func ApplicationCommandInteractionDataOption(buf *proto.ApplicationCommandInteractionDataOption) *discordgo.ApplicationCommandInteractionDataOption {
	if buf == nil {
		return nil
	}

	options := make([]*discordgo.ApplicationCommandInteractionDataOption, 0)
	for _, v := range buf.Options {
		options = append(options, ApplicationCommandInteractionDataOption(v))
	}

	fmt.Println("ApplicationCommandInteractionDataOption")

	var value any
	switch buf.Value.(type) {
	case *proto.ApplicationCommandInteractionDataOption_StringValue:
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_StringValue).StringValue
	case *proto.ApplicationCommandInteractionDataOption_IntegerValue:
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_IntegerValue).IntegerValue
	case *proto.ApplicationCommandInteractionDataOption_BooleanValue:
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_BooleanValue).BooleanValue
	case *proto.ApplicationCommandInteractionDataOption_UserValueId: // TODO: check this (ID or User struct?)
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_UserValueId).UserValueId
	case *proto.ApplicationCommandInteractionDataOption_ChannelValueId: // TODO: check this (ID or Channel struct?)
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_ChannelValueId).ChannelValueId
	case *proto.ApplicationCommandInteractionDataOption_RoleValueId: // TODO: check this (ID or Role struct?)
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_RoleValueId).RoleValueId
	case *proto.ApplicationCommandInteractionDataOption_MentionableValueId: // TODO: check this
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_MentionableValueId).MentionableValueId
	case *proto.ApplicationCommandInteractionDataOption_NumberValue:
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_NumberValue).NumberValue
	case *proto.ApplicationCommandInteractionDataOption_AttachmentValueId:
		value = buf.Value.(*proto.ApplicationCommandInteractionDataOption_AttachmentValueId).AttachmentValueId

	// Add other types as needed
	default:
		fmt.Printf("ApplicationCommandInteractionDataOption > Unknown value type: %+v\n", buf.Value)
		value = nil
	}

	return &discordgo.ApplicationCommandInteractionDataOption{
		Name:    buf.Name,
		Type:    discordgo.ApplicationCommandOptionType(buf.Type),
		Value:   value,
		Options: options,
		Focused: buf.Focused,
	}
}

func ApplicationCommandInteractionDataResolved(buf *proto.ApplicationCommandInteractionDataResolved) *discordgo.ApplicationCommandInteractionDataResolved {
	if buf == nil {
		return nil
	}

	users := make(map[string]*discordgo.User)
	for _, v := range buf.Users {
		users[v.Id] = User(v)
	}

	members := make(map[string]*discordgo.Member)
	for _, v := range buf.Members {
		members[v.User.Id] = Member(v)
	}

	roles := make(map[string]*discordgo.Role)
	for _, v := range buf.Roles {
		roles[v.Id] = Role(v)
	}

	channels := make(map[string]*discordgo.Channel)
	for _, v := range buf.Channels {
		channels[v.Id] = Channel(v)
	}

	messages := make(map[string]*discordgo.Message)
	for _, v := range buf.Messages {
		messages[v.Id] = Message(v)
	}

	attachments := make(map[string]*discordgo.MessageAttachment)
	for _, v := range buf.Attachments {
		attachments[v.Id] = MessageAttachment(v)
	}

	return &discordgo.ApplicationCommandInteractionDataResolved{
		Users:       users,
		Members:     members,
		Roles:       roles,
		Channels:    channels,
		Messages:    messages,
		Attachments: attachments,
	}
}

func MessageComponentInteractionData(buf *proto.MessageComponentInteractionData) *discordgo.MessageComponentInteractionData {
	if buf == nil {
		return nil
	}

	return &discordgo.MessageComponentInteractionData{
		CustomID:      buf.CustomId,
		ComponentType: discordgo.ComponentType(buf.ComponentType),
		Resolved:      *MessageComponentInteractionDataResolved(buf.Resolved),
		Values:        buf.Values,
	}
}

func MessageComponentInteractionDataResolved(buf *proto.MessageComponentInteractionDataResolved) *discordgo.MessageComponentInteractionDataResolved {
	if buf == nil {
		return nil
	}

	users := make(map[string]*discordgo.User)
	for k, v := range buf.Users {
		users[k] = User(v)
	}

	members := make(map[string]*discordgo.Member)
	for k, v := range buf.Members {
		members[k] = Member(v)
	}

	roles := make(map[string]*discordgo.Role)
	for k, v := range buf.Roles {
		roles[k] = Role(v)
	}

	channels := make(map[string]*discordgo.Channel)
	for k, v := range buf.Channels {
		channels[k] = Channel(v)
	}

	return &discordgo.MessageComponentInteractionDataResolved{
		Users:    users,
		Members:  members,
		Roles:    roles,
		Channels: channels,
	}
}

func ModalSubmitInteractionData(buf *proto.ModalSubmitInteractionData) *discordgo.ModalSubmitInteractionData {
	if buf == nil {
		return nil
	}

	var components []discordgo.MessageComponent
	for _, c := range buf.Components {
		if converted := MessageComponent(c); converted != nil {
			components = append(components, converted)
		}
	}

	return &discordgo.ModalSubmitInteractionData{
		CustomID:   buf.CustomId,
		Components: components,
	}
}

// InteractionResponse converts proto InteractionResponse to discordgo InteractionResponse
func InteractionResponse(buf *proto.InteractionResponse) *discordgo.InteractionResponse {
	if buf == nil {
		return nil
	}

	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseType(buf.Type),
		Data: nil, // TODO: implement InteractionResponseData conversion
	}
}

// WebhookEdit converts proto WebhookEdit to discordgo WebhookEdit
func WebhookEdit(buf *proto.WebhookEdit) *discordgo.WebhookEdit {
	if buf == nil {
		return nil
	}

	// Basic implementation - more fields can be added as needed
	edit := &discordgo.WebhookEdit{}
	if buf.Content != nil {
		edit.Content = buf.Content
	}
	return edit
}
