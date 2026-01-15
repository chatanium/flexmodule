package struct2buf

import (
	"github.com/bwmarrin/discordgo"
	proto "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
	"github.com/thirdscam/chatanium-flexmodule/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func User(s *discordgo.User) *proto.User {
	if s == nil {
		return nil
	}

	return &proto.User{
		Id:               s.ID,
		Username:         s.Username,
		Discriminator:    s.Discriminator,
		Avatar:           s.Avatar,
		Bot:              s.Bot,
		PublicFlagsValue: int32(s.PublicFlags),
		Locale:           s.Locale,
		Verified:         s.Verified,
		Email:            s.Email,
		Flags:            int32(s.Flags),
	}
}

func Member(s *discordgo.Member) *proto.Member {
	if s == nil {
		return nil
	}

	var user *proto.User
	if s.User != nil {
		user = User(s.User)
	}

	return &proto.Member{
		GuildId:                    s.GuildID,
		JoinedAt:                   timestamppb.New(s.JoinedAt),
		Nick:                       s.Nick,
		Deaf:                       s.Deaf,
		Mute:                       s.Mute,
		Avatar:                     s.Avatar,
		Roles:                      s.Roles,
		PremiumSince:               util.AsTimePtrToPbTimestamp(s.PremiumSince),
		Flags:                      int32(s.Flags),
		Pending:                    s.Pending,
		Permissions:                s.Permissions,
		Banner:                     s.Banner,
		User:                       user,
		CommunicationDisabledUntil: util.AsTimePtrToPbTimestamp(s.CommunicationDisabledUntil),
	}
}

func Channel(s *discordgo.Channel) *proto.Channel {
	if s == nil {
		return nil
	}

	recipients := make([]*proto.User, 0, len(s.Recipients))
	for _, recipient := range s.Recipients {
		recipients = append(recipients, User(recipient))
	}

	availableTags := make([]*proto.ForumTag, 0, len(s.AvailableTags))
	for _, tag := range s.AvailableTags {
		availableTags = append(availableTags, ForumTag(&tag))
	}

	permissionOverwrites := make([]*proto.PermissionOverwrite, 0, len(s.PermissionOverwrites))
	for _, overwrite := range s.PermissionOverwrites {
		permissionOverwrites = append(permissionOverwrites, PermissionOverwrite(overwrite))
	}

	var defaultSortOrder int32
	if s.DefaultSortOrder != nil {
		defaultSortOrder = int32(*s.DefaultSortOrder)
	}

	return &proto.Channel{
		Id:                            s.ID,
		GuildId:                       s.GuildID,
		Name:                          s.Name,
		Topic:                         s.Topic,
		Type:                          int32(s.Type),
		LastMessageId:                 s.LastMessageID,
		LastPinTimestamp:              util.AsTimePtrToPbTimestamp(s.LastPinTimestamp),
		MessageCount:                  int32(s.MessageCount),
		MemberCount:                   int32(s.MemberCount),
		Nsfw:                          s.NSFW,
		Icon:                          s.Icon,
		Position:                      int32(s.Position),
		Bitrate:                       int32(s.Bitrate),
		Recipients:                    recipients,
		PermissionOverwrites:          permissionOverwrites,
		UserLimit:                     int32(s.UserLimit),
		ParentId:                      s.ParentID,
		RateLimitPerUser:              int32(s.RateLimitPerUser),
		OwnerId:                       s.OwnerID,
		ApplicationId:                 s.ApplicationID,
		ThreadMetadata:                ThreadMetadata(s.ThreadMetadata),
		Member:                        ThreadMember(s.Member),
		Flags:                         int32(s.Flags),
		AvailableTags:                 availableTags,
		AppliedTags:                   s.AppliedTags,
		DefaultReactionEmoji:          ForumDefaultReaction(&s.DefaultReactionEmoji),
		DefaultThreadRateLimitPerUser: int32(s.DefaultThreadRateLimitPerUser),
		DefaultSortOrder:              defaultSortOrder,
		DefaultForumLayout:            int32(s.DefaultForumLayout),
	}
}

func ForumDefaultReaction(s *discordgo.ForumDefaultReaction) *proto.ForumDefaultReaction {
	if s == nil {
		return nil
	}

	return &proto.ForumDefaultReaction{
		EmojiId:   s.EmojiID,
		EmojiName: s.EmojiName,
	}
}

func PermissionOverwrite(s *discordgo.PermissionOverwrite) *proto.PermissionOverwrite {
	if s == nil {
		return nil
	}

	return &proto.PermissionOverwrite{
		Id:    s.ID,
		Type:  int32(s.Type),
		Deny:  s.Deny,
		Allow: s.Allow,
	}
}

func ThreadMetadata(s *discordgo.ThreadMetadata) *proto.ThreadMetadata {
	if s == nil {
		return nil
	}

	return &proto.ThreadMetadata{
		Archived:            s.Archived,
		AutoArchiveDuration: int32(s.AutoArchiveDuration),
		ArchiveTimestamp:    timestamppb.New(s.ArchiveTimestamp),
		Locked:              s.Locked,
		Invitable:           s.Invitable,
	}
}

func ThreadMember(s *discordgo.ThreadMember) *proto.ThreadMember {
	if s == nil {
		return nil
	}

	return &proto.ThreadMember{
		Id:            s.ID,
		UserId:        s.UserID,
		JoinTimestamp: timestamppb.New(s.JoinTimestamp),
		Flags:         int32(s.Flags),
		Member:        Member(s.Member),
	}
}

func ForumTag(s *discordgo.ForumTag) *proto.ForumTag {
	if s == nil {
		return nil
	}

	return &proto.ForumTag{
		Id:        s.ID,
		Name:      s.Name,
		EmojiId:   s.EmojiID,
		EmojiName: s.EmojiName,
		Moderated: s.Moderated,
	}
}

func Emoji(s *discordgo.Emoji) *proto.Emoji {
	if s == nil {
		return nil
	}

	return &proto.Emoji{
		Id:            s.ID,
		Name:          s.Name,
		Roles:         s.Roles,
		User:          User(s.User),
		RequireColons: s.RequireColons,
		Managed:       s.Managed,
		Animated:      s.Animated,
		Available:     s.Available,
	}
}

// Guild converts discordgo Guild to proto Guild
func Guild(s *discordgo.Guild) *proto.Guild {
	if s == nil {
		return nil
	}

	features := make([]string, 0, len(s.Features))
	for _, feature := range s.Features {
		features = append(features, string(feature))
	}

	roles := make([]*proto.Role, 0, len(s.Roles))
	for _, role := range s.Roles {
		roles = append(roles, Role(role))
	}

	emojis := make([]*proto.Emoji, 0, len(s.Emojis))
	for _, emoji := range s.Emojis {
		emojis = append(emojis, Emoji(emoji))
	}

	stickers := make([]*proto.Sticker, 0, len(s.Stickers))
	for _, sticker := range s.Stickers {
		stickers = append(stickers, &proto.Sticker{
			Id:          sticker.ID,
			Name:        sticker.Name,
			Description: sticker.Description,
			Tags:        sticker.Tags,
			Type:        int32(sticker.Type),
			FormatType:  int32(sticker.FormatType),
			Available:   sticker.Available,
			GuildId:     sticker.GuildID,
			User:        User(sticker.User),
			SortValue:   int32(sticker.SortValue),
			PackId:      sticker.PackID,
		})
	}

	members := make([]*proto.Member, 0, len(s.Members))
	for _, member := range s.Members {
		members = append(members, Member(member))
	}

	presences := make([]*proto.Presence, 0, len(s.Presences))
	for _, presence := range s.Presences {
		presences = append(presences, Presence(presence))
	}

	channels := make([]*proto.Channel, 0, len(s.Channels))
	for _, channel := range s.Channels {
		channels = append(channels, Channel(channel))
	}

	threads := make([]*proto.Channel, 0, len(s.Threads))
	for _, thread := range s.Threads {
		threads = append(threads, Channel(thread))
	}

	voiceStates := make([]*proto.VoiceState, 0, len(s.VoiceStates))
	for _, state := range s.VoiceStates {
		voiceStates = append(voiceStates, VoiceState(state))
	}

	stageInstances := make([]*proto.StageInstance, 0, len(s.StageInstances))
	for _, instance := range s.StageInstances {
		stageInstances = append(stageInstances, StageInstance(instance))
	}

	return &proto.Guild{
		Id:                          s.ID,
		Name:                        s.Name,
		Icon:                        s.Icon,
		Region:                      s.Region,
		AfkChannelId:                s.AfkChannelID,
		AfkTimeout:                  int32(s.AfkTimeout),
		OwnerId:                     s.OwnerID,
		VerificationLevel:           int32(s.VerificationLevel),
		DefaultMessageNotifications: int32(s.DefaultMessageNotifications),
		ExplicitContentFilter:       int32(s.ExplicitContentFilter),
		Features:                    features,
		Description:                 s.Description,
		Splash:                      s.Splash,
		Banner:                      s.Banner,
		Owner:                       s.Owner,
		JoinedAt:                    timestamppb.New(s.JoinedAt),
		DiscoverySplash:             s.DiscoverySplash,
		MemberCount:                 int32(s.MemberCount),
		Large:                       s.Large,
		Roles:                       roles,
		Emojis:                      emojis,
		Stickers:                    stickers,
		Members:                     members,
		Presences:                   presences,
		MaxPresences:                int32(s.MaxPresences),
		MaxMembers:                  int32(s.MaxMembers),
		Channels:                    channels,
		Threads:                     threads,
		VoiceStates:                 voiceStates,
		Unavailable:                 s.Unavailable,
		NsfwLevel:                   int32(s.NSFWLevel),
		MfaLevel:                    int32(s.MfaLevel),
		ApplicationId:               s.ApplicationID,
		SystemChannelId:             s.SystemChannelID,
		SystemChannelFlags:          int32(s.SystemChannelFlags),
		RulesChannelId:              s.RulesChannelID,
		PublicUpdatesChannelId:      s.PublicUpdatesChannelID,
		PreferredLocale:             s.PreferredLocale,
		WidgetEnabled:               s.WidgetEnabled,
		WidgetChannelId:             s.WidgetChannelID,
		VanityUrlCode:               s.VanityURLCode,
		PremiumTier:                 int32(s.PremiumTier),
		PremiumSubscriptionCount:    int32(s.PremiumSubscriptionCount),
		MaxVideoChannelUsers:        int32(s.MaxVideoChannelUsers),
		ApproximateMemberCount:      int32(s.ApproximateMemberCount),
		ApproximatePresenceCount:    int32(s.ApproximatePresenceCount),
		Permissions:                 s.Permissions,
		StageInstances:              stageInstances,
	}
}

// Role converts discordgo Role to proto Role
func Role(s *discordgo.Role) *proto.Role {
	if s == nil {
		return nil
	}

	return &proto.Role{
		Id:           s.ID,
		Name:         s.Name,
		Managed:      s.Managed,
		Mentionable:  s.Mentionable,
		Hoist:        s.Hoist,
		Color:        int32(s.Color),
		Position:     int32(s.Position),
		Permissions:  s.Permissions,
		Icon:         s.Icon,
		UnicodeEmoji: s.UnicodeEmoji,
		Flags:        int32(s.Flags),
	}
}

// ChannelEdit converts discordgo ChannelEdit to proto ChannelEdit
func ChannelEdit(s *discordgo.ChannelEdit) *proto.ChannelEdit {
	if s == nil {
		return nil
	}

	edit := &proto.ChannelEdit{}

	if s.Name != "" {
		edit.Name = &s.Name
	}
	if s.Topic != "" {
		edit.Topic = &s.Topic
	}
	if s.NSFW != nil {
		edit.Nsfw = s.NSFW
	}
	if s.Position != nil {
		pos := int32(*s.Position)
		edit.Position = &pos
	}
	if s.Bitrate != 0 {
		bitrate := int32(s.Bitrate)
		edit.Bitrate = &bitrate
	}
	if s.UserLimit != 0 {
		userLimit := int32(s.UserLimit)
		edit.UserLimit = &userLimit
	}
	if s.ParentID != "" {
		edit.ParentId = &s.ParentID
	}
	if s.RateLimitPerUser != nil {
		rate := int32(*s.RateLimitPerUser)
		edit.RateLimitPerUser = &rate
	}

	return edit
}

// Presence converts discordgo Presence to proto Presence
func Presence(s *discordgo.Presence) *proto.Presence {
	if s == nil {
		return nil
	}

	activities := make([]*proto.Activity, 0, len(s.Activities))
	for _, activity := range s.Activities {
		activities = append(activities, &proto.Activity{
			Name:          activity.Name,
			Type:          int32(activity.Type),
			Url:           activity.URL,
			CreatedAt:     timestamppb.New(activity.CreatedAt),
			ApplicationId: activity.ApplicationID,
			State:         activity.State,
			Details:       activity.Details,
			Timestamps: &proto.TimeStamps{
				StartTimestamp: activity.Timestamps.StartTimestamp,
				EndTimestamp:   activity.Timestamps.EndTimestamp,
			},
			Emoji: Emoji(&activity.Emoji),
		})
	}

	var since int32
	if s.Since != nil {
		since = int32(*s.Since)
	}

	return &proto.Presence{
		User:         User(s.User),
		Status:       string(s.Status),
		Activities:   activities,
		Since:        since,
		ClientStatus: ClientStatus(&s.ClientStatus),
	}
}

// ClientStatus converts discordgo ClientStatus to proto ClientStatus
func ClientStatus(s *discordgo.ClientStatus) *proto.ClientStatus {
	if s == nil {
		return nil
	}

	return &proto.ClientStatus{
		Desktop: string(s.Desktop),
		Mobile:  string(s.Mobile),
		Web:     string(s.Web),
	}
}

// VoiceState converts discordgo VoiceState to proto VoiceState
func VoiceState(s *discordgo.VoiceState) *proto.VoiceState {
	if s == nil {
		return nil
	}

	return &proto.VoiceState{
		GuildId:                 s.GuildID,
		ChannelId:               s.ChannelID,
		UserId:                  s.UserID,
		Member:                  Member(s.Member),
		SessionId:               s.SessionID,
		Deaf:                    s.Deaf,
		Mute:                    s.Mute,
		SelfDeaf:                s.SelfDeaf,
		SelfMute:                s.SelfMute,
		SelfStream:              s.SelfStream,
		SelfVideo:               s.SelfVideo,
		Suppress:                s.Suppress,
		RequestToSpeakTimestamp: util.AsTimePtrToPbTimestamp(s.RequestToSpeakTimestamp),
	}
}

// StageInstance converts discordgo StageInstance to proto StageInstance
func StageInstance(s *discordgo.StageInstance) *proto.StageInstance {
	if s == nil {
		return nil
	}

	return &proto.StageInstance{
		Id:                    s.ID,
		GuildId:               s.GuildID,
		ChannelId:             s.ChannelID,
		Topic:                 s.Topic,
		PrivacyLevel:          int32(s.PrivacyLevel),
		DiscoverableDisabled:  s.DiscoverableDisabled,
		GuildScheduledEventId: s.GuildScheduledEventID,
	}
}

// Webhook converts discordgo Webhook to protobuf Webhook
func Webhook(s *discordgo.Webhook) *proto.Webhook {
	if s == nil {
		return nil
	}
	webhook := &proto.Webhook{
		Id:        s.ID,
		Type:      proto.WebhookType(s.Type),
		GuildId:   s.GuildID,
		ChannelId: s.ChannelID,
		Name:      s.Name,
		Avatar:    s.Avatar,
		Token:     s.Token,
	}
	if s.User != nil {
		webhook.User = User(s.User)
	}
	if s.ApplicationID != "" {
		webhook.ApplicationId = &s.ApplicationID
	}
	return webhook
}
