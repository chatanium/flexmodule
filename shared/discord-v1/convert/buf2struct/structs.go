package buf2struct

import (
	"github.com/bwmarrin/discordgo"
	proto "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
	"github.com/thirdscam/chatanium-flexmodule/util"
)

func User(buf *proto.User) *discordgo.User {
	if buf == nil {
		return nil
	}

	return &discordgo.User{
		ID:            buf.Id,
		Username:      buf.Username,
		Discriminator: buf.Discriminator,
		Avatar:        buf.Avatar,
		Bot:           buf.Bot,
		PublicFlags:   discordgo.UserFlags(buf.PublicFlagsValue),
		Locale:        buf.Locale,
		Verified:      buf.Verified,
		Email:         buf.Email,
		Flags:         int(buf.Flags),
	}
}

func Member(buf *proto.Member) *discordgo.Member {
	if buf == nil {
		return nil
	}

	var user *discordgo.User
	if buf.User != nil {
		user = User(buf.User)
	}

	return &discordgo.Member{
		GuildID:                    buf.GuildId,
		JoinedAt:                   buf.JoinedAt.AsTime(),
		Nick:                       buf.Nick,
		Deaf:                       buf.Deaf,
		Mute:                       buf.Mute,
		Avatar:                     buf.Avatar,
		Roles:                      buf.Roles,
		PremiumSince:               util.PbTimestamp2AsTimePtr(buf.PremiumSince),
		Flags:                      discordgo.MemberFlags(buf.Flags),
		Pending:                    buf.Pending,
		Permissions:                buf.Permissions,
		Banner:                     buf.Banner,
		User:                       user,
		CommunicationDisabledUntil: util.PbTimestamp2AsTimePtr(buf.CommunicationDisabledUntil),
	}
}

func Channel(buf *proto.Channel) *discordgo.Channel {
	if buf == nil {
		return nil
	}

	recipients := make([]*discordgo.User, 0)
	for _, recipient := range buf.Recipients {
		recipients = append(recipients, User(recipient))
	}

	availableTags := make([]discordgo.ForumTag, 0)
	for _, tag := range buf.AvailableTags {
		availableTags = append(availableTags, *ForumTag(tag))
	}

	permissionOverwrites := make([]*discordgo.PermissionOverwrite, 0)
	for _, overwrite := range buf.PermissionOverwrites {
		permissionOverwrites = append(permissionOverwrites, PermissionOverwrite(overwrite))
	}

	sortOrder := discordgo.ForumSortOrderType(buf.DefaultSortOrder)
	defaultSortOrder := &sortOrder
	return &discordgo.Channel{
		ID:                            buf.Id,
		GuildID:                       buf.GuildId,
		Name:                          buf.Name,
		Topic:                         buf.Topic,
		Type:                          discordgo.ChannelType(buf.Type),
		LastMessageID:                 buf.LastMessageId,
		LastPinTimestamp:              util.PbTimestamp2AsTimePtr(buf.LastPinTimestamp),
		MessageCount:                  int(buf.MessageCount),
		MemberCount:                   int(buf.MemberCount),
		NSFW:                          buf.Nsfw,
		Icon:                          buf.Icon,
		Position:                      int(buf.Position),
		Bitrate:                       int(buf.Bitrate),
		Recipients:                    recipients,
		Messages:                      nil, // TODO: Mapping state-enabled fields (#1)
		PermissionOverwrites:          permissionOverwrites,
		UserLimit:                     int(buf.UserLimit),
		ParentID:                      buf.ParentId,
		RateLimitPerUser:              int(buf.RateLimitPerUser),
		OwnerID:                       buf.OwnerId,
		ApplicationID:                 buf.ApplicationId,
		ThreadMetadata:                ThreadMetadata(buf.ThreadMetadata),
		Member:                        ThreadMember(buf.Member),
		Members:                       nil, // TODO: Mapping state-enabled fields (#1)
		Flags:                         discordgo.ChannelFlags(buf.Flags),
		AvailableTags:                 availableTags,
		AppliedTags:                   buf.AppliedTags,
		DefaultReactionEmoji:          *ForumDefaultReaction(buf.DefaultReactionEmoji),
		DefaultThreadRateLimitPerUser: int(buf.DefaultThreadRateLimitPerUser),
		DefaultSortOrder:              defaultSortOrder,
		DefaultForumLayout:            discordgo.ForumLayout(buf.DefaultForumLayout),
	}
}

func ForumDefaultReaction(buf *proto.ForumDefaultReaction) *discordgo.ForumDefaultReaction {
	if buf == nil {
		return nil
	}

	return &discordgo.ForumDefaultReaction{
		EmojiID:   buf.EmojiId,
		EmojiName: buf.EmojiName,
	}
}

func PermissionOverwrite(buf *proto.PermissionOverwrite) *discordgo.PermissionOverwrite {
	if buf == nil {
		return nil
	}

	return &discordgo.PermissionOverwrite{
		ID:    buf.Id,
		Type:  discordgo.PermissionOverwriteType(buf.Type),
		Deny:  buf.Deny,
		Allow: buf.Allow,
	}
}

func ThreadMetadata(buf *proto.ThreadMetadata) *discordgo.ThreadMetadata {
	if buf == nil {
		return nil
	}

	return &discordgo.ThreadMetadata{
		Archived:            buf.Archived,
		AutoArchiveDuration: int(buf.AutoArchiveDuration),
		ArchiveTimestamp:    buf.ArchiveTimestamp.AsTime(),
		Locked:              buf.Locked,
		Invitable:           buf.Invitable,
	}
}

func ThreadMember(buf *proto.ThreadMember) *discordgo.ThreadMember {
	if buf == nil {
		return nil
	}

	return &discordgo.ThreadMember{
		ID:            buf.Id,
		UserID:        buf.UserId,
		JoinTimestamp: buf.JoinTimestamp.AsTime(),
		Flags:         int(buf.Flags),
		Member:        Member(buf.Member),
	}
}

func ForumTag(buf *proto.ForumTag) *discordgo.ForumTag {
	if buf == nil {
		return nil
	}

	return &discordgo.ForumTag{
		ID:        buf.Id,
		Name:      buf.Name,
		EmojiID:   buf.EmojiId,
		EmojiName: buf.EmojiName,
		Moderated: buf.Moderated,
	}
}

func Emoji(buf *proto.Emoji) *discordgo.Emoji {
	if buf == nil {
		return nil
	}

	return &discordgo.Emoji{
		ID:            buf.Id,
		Name:          buf.Name,
		Roles:         buf.Roles,
		User:          User(buf.User),
		RequireColons: buf.RequireColons,
		Managed:       buf.Managed,
		Animated:      buf.Animated,
		Available:     buf.Animated,
	}
}

func Role(buf *proto.Role) *discordgo.Role {
	if buf == nil {
		return nil
	}

	return &discordgo.Role{
		ID:           buf.Id,
		Name:         buf.Name,
		Managed:      buf.Managed,
		Mentionable:  buf.Mentionable,
		Hoist:        buf.Hoist,
		Color:        int(buf.Color),
		Position:     int(buf.Position),
		Permissions:  buf.Permissions,
		Icon:         buf.Icon,
		UnicodeEmoji: buf.UnicodeEmoji,
		Flags:        discordgo.RoleFlags(buf.Flags),
	}
}

// ChannelEdit converts proto ChannelEdit to discordgo ChannelEdit
func ChannelEdit(buf *proto.ChannelEdit) *discordgo.ChannelEdit {
	if buf == nil {
		return nil
	}

	edit := &discordgo.ChannelEdit{}

	if buf.Name != nil {
		edit.Name = *buf.Name
	}
	if buf.Topic != nil {
		edit.Topic = *buf.Topic
	}
	if buf.Nsfw != nil {
		edit.NSFW = buf.Nsfw
	}
	if buf.Position != nil {
		pos := int(*buf.Position)
		edit.Position = &pos
	}
	if buf.Bitrate != nil {
		edit.Bitrate = int(*buf.Bitrate)
	}
	if buf.UserLimit != nil {
		edit.UserLimit = int(*buf.UserLimit)
	}
	if buf.ParentId != nil {
		edit.ParentID = *buf.ParentId
	}
	if buf.RateLimitPerUser != nil {
		rate := int(*buf.RateLimitPerUser)
		edit.RateLimitPerUser = &rate
	}

	return edit
}

func Presence(buf *proto.Presence) *discordgo.Presence {
	if buf == nil {
		return nil
	}

	activities := make([]*discordgo.Activity, 0, len(buf.Activities))
	for _, activity := range buf.Activities {
		activities = append(activities, &discordgo.Activity{
			Name:          activity.Name,
			Type:          discordgo.ActivityType(activity.Type),
			URL:           activity.Url,
			CreatedAt:     activity.CreatedAt.AsTime(),
			ApplicationID: activity.ApplicationId,
			State:         activity.State,
			Details:       activity.Details,
			Timestamps: discordgo.TimeStamps{
				StartTimestamp: activity.Timestamps.StartTimestamp,
				EndTimestamp:   activity.Timestamps.EndTimestamp,
			},
			Emoji: *Emoji(activity.Emoji),
		})
	}

	var sincePtr *int
	if buf.Since != 0 {
		since := int(buf.Since)
		sincePtr = &since
	}

	return &discordgo.Presence{
		User:         User(buf.User),
		Status:       discordgo.Status(buf.Status),
		Activities:   activities,
		Since:        sincePtr,
		ClientStatus: *ClientStatus(buf.ClientStatus),
	}
}

func ClientStatus(buf *proto.ClientStatus) *discordgo.ClientStatus {
	if buf == nil {
		return nil
	}

	return &discordgo.ClientStatus{
		Desktop: discordgo.Status(buf.Desktop),
		Mobile:  discordgo.Status(buf.Mobile),
		Web:     discordgo.Status(buf.Web),
	}
}

func VoiceState(buf *proto.VoiceState) *discordgo.VoiceState {
	if buf == nil {
		return nil
	}

	return &discordgo.VoiceState{
		GuildID:                 buf.GuildId,
		ChannelID:               buf.ChannelId,
		UserID:                  buf.UserId,
		Member:                  Member(buf.Member),
		SessionID:               buf.SessionId,
		Deaf:                    buf.Deaf,
		Mute:                    buf.Mute,
		SelfDeaf:                buf.SelfDeaf,
		SelfMute:                buf.SelfMute,
		SelfStream:              buf.SelfStream,
		SelfVideo:               buf.SelfVideo,
		Suppress:                buf.Suppress,
		RequestToSpeakTimestamp: util.PbTimestamp2AsTimePtr(buf.RequestToSpeakTimestamp),
	}
}

func StageInstance(buf *proto.StageInstance) *discordgo.StageInstance {
	if buf == nil {
		return nil
	}

	return &discordgo.StageInstance{
		ID:                    buf.Id,
		GuildID:               buf.GuildId,
		ChannelID:             buf.ChannelId,
		Topic:                 buf.Topic,
		PrivacyLevel:          discordgo.StageInstancePrivacyLevel(buf.PrivacyLevel),
		DiscoverableDisabled:  buf.DiscoverableDisabled,
		GuildScheduledEventID: buf.GuildScheduledEventId,
	}
}

// Guild converts proto Guild to discordgo Guild
func Guild(buf *proto.Guild) *discordgo.Guild {
	if buf == nil {
		return nil
	}

	features := make([]discordgo.GuildFeature, 0, len(buf.Features))
	for _, feature := range buf.Features {
		features = append(features, discordgo.GuildFeature(feature))
	}

	roles := make([]*discordgo.Role, 0, len(buf.Roles))
	for _, role := range buf.Roles {
		roles = append(roles, Role(role))
	}

	emojis := make([]*discordgo.Emoji, 0, len(buf.Emojis))
	for _, emoji := range buf.Emojis {
		emojis = append(emojis, Emoji(emoji))
	}

	stickers := make([]*discordgo.Sticker, 0, len(buf.Stickers))
	for _, sticker := range buf.Stickers {
		stickers = append(stickers, &discordgo.Sticker{
			ID:          sticker.Id,
			Name:        sticker.Name,
			Description: sticker.Description,
			Tags:        sticker.Tags,
			Type:        discordgo.StickerType(sticker.Type),
			FormatType:  discordgo.StickerFormat(sticker.FormatType),
			Available:   sticker.Available,
			GuildID:     sticker.GuildId,
			User:        User(sticker.User),
			SortValue:   int(sticker.SortValue),
			PackID:      sticker.PackId,
		})
	}

	members := make([]*discordgo.Member, 0, len(buf.Members))
	for _, member := range buf.Members {
		members = append(members, Member(member))
	}

	presences := make([]*discordgo.Presence, 0, len(buf.Presences))
	for _, presence := range buf.Presences {
		presences = append(presences, Presence(presence))
	}

	channels := make([]*discordgo.Channel, 0, len(buf.Channels))
	for _, channel := range buf.Channels {
		channels = append(channels, Channel(channel))
	}

	threads := make([]*discordgo.Channel, 0, len(buf.Threads))
	for _, thread := range buf.Threads {
		threads = append(threads, Channel(thread))
	}

	voiceStates := make([]*discordgo.VoiceState, 0, len(buf.VoiceStates))
	for _, state := range buf.VoiceStates {
		voiceStates = append(voiceStates, VoiceState(state))
	}

	stageInstances := make([]*discordgo.StageInstance, 0, len(buf.StageInstances))
	for _, instance := range buf.StageInstances {
		stageInstances = append(stageInstances, StageInstance(instance))
	}

	// Basic implementation - more fields can be added as needed
	return &discordgo.Guild{
		// Basic guild information
		ID:                          buf.Id,
		Name:                        buf.Name,
		Icon:                        buf.Icon,
		Region:                      buf.Region,
		AfkChannelID:                buf.AfkChannelId,
		AfkTimeout:                  int(buf.AfkTimeout),
		OwnerID:                     buf.OwnerId,
		VerificationLevel:           discordgo.VerificationLevel(buf.VerificationLevel),
		DefaultMessageNotifications: discordgo.MessageNotifications(buf.DefaultMessageNotifications),
		ExplicitContentFilter:       discordgo.ExplicitContentFilterLevel(buf.ExplicitContentFilter),
		Features:                    features,
		Description:                 buf.Description,
		Splash:                      buf.Splash,
		Banner:                      buf.Banner,
		Owner:                       buf.Owner,
		JoinedAt:                    buf.JoinedAt.AsTime(),
		DiscoverySplash:             buf.DiscoverySplash,
		MemberCount:                 int(buf.MemberCount),
		Large:                       buf.Large,
		Roles:                       roles,
		Emojis:                      emojis,
		Stickers:                    stickers,
		Members:                     members,
		Presences:                   presences,
		MaxPresences:                int(buf.MaxPresences),
		MaxMembers:                  int(buf.MaxMembers),
		Channels:                    channels,
		Threads:                     threads,
		VoiceStates:                 voiceStates,
		Unavailable:                 buf.Unavailable,
		NSFWLevel:                   discordgo.GuildNSFWLevel(buf.NsfwLevel),
		MfaLevel:                    discordgo.MfaLevel(buf.MfaLevel),
		ApplicationID:               buf.ApplicationId,
		SystemChannelID:             buf.SystemChannelId,
		SystemChannelFlags:          discordgo.SystemChannelFlag(buf.SystemChannelFlags),
		RulesChannelID:              buf.RulesChannelId,
		PublicUpdatesChannelID:      buf.PublicUpdatesChannelId,
		PreferredLocale:             buf.PreferredLocale,
		WidgetEnabled:               buf.WidgetEnabled,
		WidgetChannelID:             buf.WidgetChannelId,
		VanityURLCode:               buf.VanityUrlCode,
		PremiumTier:                 discordgo.PremiumTier(buf.PremiumTier),
		PremiumSubscriptionCount:    int(buf.PremiumSubscriptionCount),
		MaxVideoChannelUsers:        int(buf.MaxVideoChannelUsers),
		ApproximateMemberCount:      int(buf.ApproximateMemberCount),
		ApproximatePresenceCount:    int(buf.ApproximatePresenceCount),
		Permissions:                 buf.Permissions,
		StageInstances:              stageInstances,
	}
}

// VoiceRegion converts protobuf VoiceRegion to discordgo VoiceRegion
// Webhook converts protobuf Webhook to discordgo Webhook
func Webhook(buf *proto.Webhook) *discordgo.Webhook {
	if buf == nil {
		return nil
	}
	webhook := &discordgo.Webhook{
		ID:        buf.Id,
		Type:      discordgo.WebhookType(buf.Type),
		GuildID:   buf.GuildId,
		ChannelID: buf.ChannelId,
		Name:      buf.Name,
		Avatar:    buf.Avatar,
		Token:     buf.Token,
	}
	if buf.User != nil {
		webhook.User = User(buf.User)
	}
	if buf.ApplicationId != nil {
		webhook.ApplicationID = *buf.ApplicationId
	}
	return webhook
}
