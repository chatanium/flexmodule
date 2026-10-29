package struct2buf

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	proto "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
)

func ApplicationCommand(s *discordgo.ApplicationCommand) *proto.ApplicationCommand {
	if s == nil {
		return nil
	}

	var cmdType proto.ApplicationCommandType
	switch s.Type {
	case discordgo.ChatApplicationCommand:
		cmdType = 1
	case discordgo.UserApplicationCommand:
		cmdType = 2
	case discordgo.MessageApplicationCommand:
		cmdType = 3
	default:
		cmdType = 0
	}

	var defaultMemberPermissions int64
	if s.DefaultMemberPermissions != nil {
		defaultMemberPermissions = *s.DefaultMemberPermissions
	}

	nameLocalizations := make(map[string]string)
	if s.NameLocalizations != nil {
		for k, v := range *s.NameLocalizations {
			nameLocalizations[string(k)] = v
		}
	}

	descriptionLocalizations := make(map[string]string)
	if s.DescriptionLocalizations != nil {
		for k, v := range *s.DescriptionLocalizations {
			descriptionLocalizations[string(k)] = v
		}
	}

	options := make([]*proto.ApplicationCommandOption, 0)
	if s.Options != nil {
		for _, v := range s.Options {
			options = append(options, ApplicationCommandOption(v))
		}
	}

	return &proto.ApplicationCommand{
		Id:                s.ID,
		ApplicationId:     s.ApplicationID,
		GuildId:           s.GuildID,
		Version:           s.Version,
		Type:              cmdType,
		Name:              s.Name,
		NameLocalizations: nameLocalizations,
		// DefaultPermission:        s.DefaultPermission, // Deprecated
		DefaultMemberPermissions: defaultMemberPermissions,
		// DmPermission:             s.DMPermission, // Deprecated
		Nsfw:                     s.NSFW,
		Description:              s.Description,
		DescriptionLocalizations: descriptionLocalizations,
		Options:                  options,
	}
}

func ApplicationCommandOption(s *discordgo.ApplicationCommandOption) *proto.ApplicationCommandOption {
	if s == nil {
		return nil
	}

	options := make([]*proto.ApplicationCommandOption, 0)
	for _, v := range s.Options {
		options = append(options, ApplicationCommandOption(v))
	}

	nameLocalizations := make(map[string]string)
	for k, v := range s.NameLocalizations {
		nameLocalizations[string(k)] = v
	}

	descriptionLocalizations := make(map[string]string)
	for k, v := range s.DescriptionLocalizations {
		descriptionLocalizations[string(k)] = v
	}

	channelType := make([]proto.ChannelType, 0)
	for _, v := range s.ChannelTypes {
		channelType = append(channelType, proto.ChannelType(v))
	}

	choices := make([]*proto.ApplicationCommandOptionChoice, 0)
	for _, v := range s.Choices {
		choices = append(choices, ApplicationCommandOptionChoice(v))
	}

	result := &proto.ApplicationCommandOption{
		Type:                     proto.ApplicationCommandOptionType(s.Type),
		Name:                     s.Name,
		NameLocalizations:        nameLocalizations,
		Description:              s.Description,
		DescriptionLocalizations: descriptionLocalizations,
		ChannelTypes:             channelType,
		Autocomplete:             s.Autocomplete,
		Choices:                  choices,
		Required:                 s.Required,
		Options:                  options,
		MinValue:                 s.MinValue,
		MaxValue:                 &s.MaxValue,
		MinLength:                IntPtrToInt32Ptr(s.MinLength),
		MaxLength:                IntPtrToInt32Ptr(&s.MaxLength),
	}

	return result
}

func ApplicationCommandOptionChoice(s *discordgo.ApplicationCommandOptionChoice) *proto.ApplicationCommandOptionChoice {
	if s == nil {
		return nil
	}

	result := &proto.ApplicationCommandOptionChoice{
		Name: s.Name,
	}

	switch v := s.Value.(type) {
	case string:
		result.Value = &proto.ApplicationCommandOptionChoice_StringValue{
			StringValue: v,
		}
	case int:
		result.Value = &proto.ApplicationCommandOptionChoice_IntegerValue{
			IntegerValue: int64(v),
		}
	case float64:
		result.Value = &proto.ApplicationCommandOptionChoice_NumberValue{
			NumberValue: v,
		}
	default:
		fmt.Printf("ApplicationCommandOptionChoice > unknown choice value type: %T\n", v)
	}

	return result
}

func Interaction(s *discordgo.Interaction) *proto.Interaction {
	if s == nil {
		return nil
	}

	var guildLocale *string
	if s.GuildLocale != nil {
		gl := string(*s.GuildLocale)
		guildLocale = &gl
	}

	result := &proto.Interaction{
		Id:                           s.ID,
		AppId:                        s.AppID,
		Type:                         proto.InteractionType(s.Type),
		GuildId:                      s.GuildID,
		ChannelId:                    s.ChannelID,
		Message:                      Message(s.Message),
		AppPermissions:               s.AppPermissions,
		Member:                       Member(s.Member),
		User:                         User(s.User),
		Locale:                       string(s.Locale),
		GuildLocale:                  guildLocale,
		Context:                      0,   // TODO(discord/bufstruct): Preparing for discordgo updates (^v0.28.1)
		AuthorizingIntegrationOwners: nil, // TODO(discord/bufstruct): Preparing for discordgo updates (^v0.28.1)
		Token:                        s.Token,
		Version:                      int32(s.Version),
		Entitlements:                 nil, // TODO(discord/bufstruct): Preparing for discordgo updates (^v0.28.1)
	}

	if s.Data != nil {
		switch s.Data.Type() {
		case discordgo.InteractionApplicationCommand, discordgo.InteractionApplicationCommandAutocomplete:
			data := s.Data.(discordgo.ApplicationCommandInteractionData)
			result.Data = &proto.Interaction_ApplicationCommandData{
				ApplicationCommandData: ApplicationCommandInteractionData(&data),
			}
		case discordgo.InteractionMessageComponent:
			data := s.Data.(discordgo.MessageComponentInteractionData)
			result.Data = &proto.Interaction_MessageComponentData{
				MessageComponentData: MessageComponentInteractionData(&data),
			}
		case discordgo.InteractionModalSubmit:
			data := s.Data.(discordgo.ModalSubmitInteractionData)
			result.Data = &proto.Interaction_ModalSubmitData{
				ModalSubmitData: ModalSubmitInteractionData(&data),
			}
		default:
			fmt.Printf("Interaction > unknown interaction data type: %d\n", s.Data.Type())
		}
	}

	return result
}

func ApplicationCommandInteractionData(s *discordgo.ApplicationCommandInteractionData) *proto.ApplicationCommandInteractionData {
	if s == nil {
		return nil
	}

	options := make([]*proto.ApplicationCommandInteractionDataOption, 0)
	for _, v := range s.Options {
		options = append(options, ApplicationCommandInteractionDataOption(v))
	}

	return &proto.ApplicationCommandInteractionData{
		Id:          s.ID,
		Name:        s.Name,
		CommandType: proto.ApplicationCommandType(s.CommandType),
		Resolved:    ApplicationCommandInteractionDataResolved(s.Resolved),
		Options:     options,
		TargetId:    s.TargetID,
	}
}

func ApplicationCommandInteractionDataOption(s *discordgo.ApplicationCommandInteractionDataOption) *proto.ApplicationCommandInteractionDataOption {
	if s == nil {
		return nil
	}

	options := make([]*proto.ApplicationCommandInteractionDataOption, 0)
	for _, v := range s.Options {
		options = append(options, ApplicationCommandInteractionDataOption(v))
	}

	result := &proto.ApplicationCommandInteractionDataOption{
		Name:    s.Name,
		Type:    proto.ApplicationCommandOptionType(s.Type),
		Options: options, // TODO(struct2buf): Consider same-type recursion issues
		Focused: s.Focused,
	}

	if s.Value != nil {
		switch s.Type {
		case discordgo.ApplicationCommandOptionInteger:
			result.Value = &proto.ApplicationCommandInteractionDataOption_IntegerValue{
				IntegerValue: s.IntValue(),
			}
		case discordgo.ApplicationCommandOptionNumber:
			result.Value = &proto.ApplicationCommandInteractionDataOption_NumberValue{
				NumberValue: s.FloatValue(),
			}
		case discordgo.ApplicationCommandOptionString:
			result.Value = &proto.ApplicationCommandInteractionDataOption_StringValue{
				StringValue: s.StringValue(),
			}
		case discordgo.ApplicationCommandOptionBoolean:
			result.Value = &proto.ApplicationCommandInteractionDataOption_BooleanValue{
				BooleanValue: s.BoolValue(),
			}
		case discordgo.ApplicationCommandOptionChannel:
			if s.Value == nil {
				fmt.Println("ApplicationCommandInteractionDataOption > discordgo.Channel is nil")
				result.Value = &proto.ApplicationCommandInteractionDataOption_ChannelValueId{
					ChannelValueId: "",
				}
			} else {
				result.Value = &proto.ApplicationCommandInteractionDataOption_ChannelValueId{
					ChannelValueId: s.Value.(string),
				}
			}
		case discordgo.ApplicationCommandOptionUser:
			if s.Value == nil {
				fmt.Println("ApplicationCommandInteractionDataOption > discordgo.User is nil")
				result.Value = &proto.ApplicationCommandInteractionDataOption_UserValueId{
					UserValueId: "",
				}
			} else {
				result.Value = &proto.ApplicationCommandInteractionDataOption_UserValueId{
					UserValueId: s.Value.(string),
				}
			}
		default:
			fmt.Printf("ApplicationCommandInteractionDataOption > unknown option value type for option '%s' of type %d\n", s.Name, s.Type)
		}
	}

	return result
}

func ApplicationCommandInteractionDataResolved(s *discordgo.ApplicationCommandInteractionDataResolved) *proto.ApplicationCommandInteractionDataResolved {
	if s == nil {
		return nil
	}

	users := make(map[string]*proto.User)
	for k, v := range s.Users {
		users[k] = User(v)
	}

	roles := make(map[string]*proto.Role)
	for k, v := range s.Roles {
		roles[k] = Role(v)
	}

	members := make(map[string]*proto.Member)
	for k, v := range s.Members {
		members[k] = Member(v)
	}

	channels := make(map[string]*proto.Channel)
	for k, v := range s.Channels {
		channels[k] = Channel(v)
	}

	messages := make(map[string]*proto.Message)
	for k, v := range s.Messages {
		messages[k] = Message(v)
	}

	attachments := make(map[string]*proto.MessageAttachment)
	for k, v := range s.Attachments {
		attachments[k] = MessageAttachment(v)
	}

	return &proto.ApplicationCommandInteractionDataResolved{
		Users:       users,
		Members:     members,
		Roles:       roles,
		Channels:    channels,
		Messages:    messages,
		Attachments: attachments,
	}
}

func MessageComponentInteractionData(s *discordgo.MessageComponentInteractionData) *proto.MessageComponentInteractionData {
	if s == nil {
		return nil
	}

	return &proto.MessageComponentInteractionData{
		CustomId:      s.CustomID,
		ComponentType: proto.ComponentType(s.ComponentType),
		Resolved:      MessageComponentInteractionDataResolved(&s.Resolved),
		Values:        s.Values,
	}
}

func MessageComponentInteractionDataResolved(s *discordgo.MessageComponentInteractionDataResolved) *proto.MessageComponentInteractionDataResolved {
	if s == nil {
		return nil
	}

	users := make(map[string]*proto.User)
	for k, v := range s.Users {
		users[k] = User(v)
	}

	members := make(map[string]*proto.Member)
	for k, v := range s.Members {
		members[k] = Member(v)
	}

	roles := make(map[string]*proto.Role)
	for k, v := range s.Roles {
		roles[k] = Role(v)
	}

	channels := make(map[string]*proto.Channel)
	for k, v := range s.Channels {
		channels[k] = Channel(v)
	}

	return &proto.MessageComponentInteractionDataResolved{
		Users:    users,
		Members:  members,
		Roles:    roles,
		Channels: channels,
	}
}

func ModalSubmitInteractionData(s *discordgo.ModalSubmitInteractionData) *proto.ModalSubmitInteractionData {
	if s == nil {
		return nil
	}

	var components []*proto.MessageComponent
	for _, c := range s.Components {
		if converted := MessageComponent(c); converted != nil {
			components = append(components, converted)
		}
	}

	return &proto.ModalSubmitInteractionData{
		CustomId:   s.CustomID,
		Components: components,
	}
}

// InteractionResponse converts discordgo InteractionResponse to proto InteractionResponse
func InteractionResponse(s *discordgo.InteractionResponse) *proto.InteractionResponse {
	if s == nil {
		return nil
	}

	return &proto.InteractionResponse{
		Type: proto.InteractionResponseType(s.Type),
		Data: InteractionResponseData(s.Data), // TODO: implement InteractionResponseData conversion
	}
}

func InteractionResponseData(s *discordgo.InteractionResponseData) *proto.InteractionResponseData {
	if s == nil {
		return nil
	}

	return &proto.InteractionResponseData{
		Tts:             s.TTS,
		Content:         s.Content,
		Components:      MessageComponents(s.Components),
		Embeds:          MessageEmbeds(s.Embeds),
		AllowedMentions: MessageAllowedMentions(s.AllowedMentions),
		Flags:           int64(s.Flags),
	}
}

// WebhookEdit converts discordgo WebhookEdit to proto WebhookEdit
func WebhookEdit(s *discordgo.WebhookEdit) *proto.WebhookEdit {
	if s == nil {
		return nil
	}

	edit := &proto.WebhookEdit{}
	if s.Content != nil {
		edit.Content = s.Content
	}
	// Add more fields as needed for embeds, components, etc.

	return edit
}

// WebhookParams converts discordgo WebhookParams to protobuf WebhookParams
func WebhookParams(s *discordgo.WebhookParams) *proto.WebhookParams {
	if s == nil {
		return nil
	}

	params := &proto.WebhookParams{}

	if s.Content != "" {
		params.Content = &s.Content
	}
	if s.Username != "" {
		params.Username = &s.Username
	}
	if s.AvatarURL != "" {
		params.AvatarUrl = &s.AvatarURL
	}
	if s.TTS {
		params.Tts = &s.TTS
	}
	if s.ThreadName != "" {
		params.ThreadName = &s.ThreadName
	}

	// Convert embeds
	for _, embed := range s.Embeds {
		if embed != nil {
			params.Embeds = append(params.Embeds, MessageEmbed(embed))
		}
	}

	// Convert attachments
	for _, attachment := range s.Attachments {
		if attachment != nil {
			params.Attachments = append(params.Attachments, MessageAttachment(attachment))
		}
	}

	// Convert allowed mentions
	if s.AllowedMentions != nil {
		params.AllowedMentions = MessageAllowedMentions(s.AllowedMentions)
	}

	// Note: Files and Components are complex and would need special handling
	// For now, we'll leave them empty as they're not commonly used in simple webhook calls

	return params
}
