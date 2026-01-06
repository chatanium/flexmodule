package buf2struct

import (
	"github.com/bwmarrin/discordgo"
	proto "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
)

// MessageComponent converts a proto.MessageComponent to discordgo.MessageComponent interface
func MessageComponent(buf *proto.MessageComponent) discordgo.MessageComponent {
	if buf == nil {
		return nil
	}

	switch c := buf.Component.(type) {
	case *proto.MessageComponent_ActionsRow:
		return ActionsRow(c.ActionsRow)
	case *proto.MessageComponent_Button:
		return Button(c.Button)
	case *proto.MessageComponent_SelectMenu:
		return SelectMenu(c.SelectMenu)
	case *proto.MessageComponent_TextInput:
		return TextInput(c.TextInput)
	case *proto.MessageComponent_Section:
		return Section(c.Section)
	case *proto.MessageComponent_TextDisplay:
		return TextDisplay(c.TextDisplay)
	case *proto.MessageComponent_Thumbnail:
		return Thumbnail(c.Thumbnail)
	case *proto.MessageComponent_MediaGallery:
		return MediaGallery(c.MediaGallery)
	case *proto.MessageComponent_FileComponent:
		return FileComponent(c.FileComponent)
	case *proto.MessageComponent_Separator:
		return Separator(c.Separator)
	case *proto.MessageComponent_Container:
		return Container(c.Container)
	default:
		return nil
	}
}

// MessageComponents converts a slice of proto.MessageComponent to discordgo.MessageComponent slice
func MessageComponents(buf []*proto.MessageComponent) []discordgo.MessageComponent {
	if buf == nil {
		return nil
	}

	result := make([]discordgo.MessageComponent, 0, len(buf))
	for _, c := range buf {
		if converted := MessageComponent(c); converted != nil {
			result = append(result, converted)
		}
	}
	return result
}

// ActionsRow converts proto.ActionsRow to discordgo.ActionsRow
func ActionsRow(buf *proto.ActionsRow) *discordgo.ActionsRow {
	if buf == nil {
		return nil
	}

	return &discordgo.ActionsRow{
		Components: MessageComponents(buf.Components),
		ID:         int(buf.Id),
	}
}

// Button converts proto.Button to discordgo.Button
func Button(buf *proto.Button) *discordgo.Button {
	if buf == nil {
		return nil
	}

	return &discordgo.Button{
		Label:    buf.Label,
		Style:    discordgo.ButtonStyle(buf.Style),
		Disabled: buf.Disabled,
		Emoji:    ComponentEmoji(buf.Emoji),
		URL:      buf.Url,
		CustomID: buf.CustomId,
		SKUID:    buf.SkuId,
		ID:       int(buf.Id),
	}
}

// ComponentEmoji converts proto.ComponentEmoji to discordgo.ComponentEmoji
func ComponentEmoji(buf *proto.ComponentEmoji) *discordgo.ComponentEmoji {
	if buf == nil {
		return nil
	}

	return &discordgo.ComponentEmoji{
		Name:     buf.Name,
		ID:       buf.Id,
		Animated: buf.Animated,
	}
}

// SelectMenu converts proto.SelectMenu to discordgo.SelectMenu
func SelectMenu(buf *proto.SelectMenu) *discordgo.SelectMenu {
	if buf == nil {
		return nil
	}

	options := make([]discordgo.SelectMenuOption, 0, len(buf.Options))
	for _, opt := range buf.Options {
		if converted := SelectMenuOption(opt); converted != nil {
			options = append(options, *converted)
		}
	}

	defaultValues := make([]discordgo.SelectMenuDefaultValue, 0, len(buf.DefaultValues))
	for _, dv := range buf.DefaultValues {
		if converted := SelectMenuDefaultValue(dv); converted != nil {
			defaultValues = append(defaultValues, *converted)
		}
	}

	channelTypes := make([]discordgo.ChannelType, 0, len(buf.ChannelTypes))
	for _, ct := range buf.ChannelTypes {
		channelTypes = append(channelTypes, discordgo.ChannelType(ct))
	}

	result := &discordgo.SelectMenu{
		MenuType:      discordgo.SelectMenuType(buf.MenuType),
		CustomID:      buf.CustomId,
		Placeholder:   buf.Placeholder,
		MaxValues:     int(buf.MaxValues),
		DefaultValues: defaultValues,
		Options:       options,
		Disabled:      buf.Disabled,
		ChannelTypes:  channelTypes,
		ID:            int(buf.Id),
	}

	if buf.MinValues != nil {
		minVal := int(*buf.MinValues)
		result.MinValues = &minVal
	}

	return result
}

// SelectMenuOption converts proto.SelectMenuOption to discordgo.SelectMenuOption
func SelectMenuOption(buf *proto.SelectMenuOption) *discordgo.SelectMenuOption {
	if buf == nil {
		return nil
	}

	return &discordgo.SelectMenuOption{
		Label:       buf.Label,
		Value:       buf.Value,
		Description: buf.Description,
		Emoji:       ComponentEmoji(buf.Emoji),
		Default:     buf.IsDefault,
	}
}

// SelectMenuDefaultValue converts proto.SelectMenuDefaultValue to discordgo.SelectMenuDefaultValue
func SelectMenuDefaultValue(buf *proto.SelectMenuDefaultValue) *discordgo.SelectMenuDefaultValue {
	if buf == nil {
		return nil
	}

	var valueType discordgo.SelectMenuDefaultValueType
	switch buf.Type {
	case proto.SelectMenuDefaultValueType_SELECT_MENU_DEFAULT_VALUE_TYPE_USER:
		valueType = discordgo.SelectMenuDefaultValueUser
	case proto.SelectMenuDefaultValueType_SELECT_MENU_DEFAULT_VALUE_TYPE_ROLE:
		valueType = discordgo.SelectMenuDefaultValueRole
	case proto.SelectMenuDefaultValueType_SELECT_MENU_DEFAULT_VALUE_TYPE_CHANNEL:
		valueType = discordgo.SelectMenuDefaultValueChannel
	}

	return &discordgo.SelectMenuDefaultValue{
		ID:   buf.Id,
		Type: valueType,
	}
}

// TextInput converts proto.TextInput to discordgo.TextInput
func TextInput(buf *proto.TextInput) *discordgo.TextInput {
	if buf == nil {
		return nil
	}

	return &discordgo.TextInput{
		CustomID:    buf.CustomId,
		Label:       buf.Label,
		Style:       discordgo.TextInputStyle(buf.Style),
		Placeholder: buf.Placeholder,
		Value:       buf.Value,
		Required:    buf.Required,
		MinLength:   int(buf.MinLength),
		MaxLength:   int(buf.MaxLength),
		ID:          int(buf.Id),
	}
}

// Section converts proto.Section to discordgo.Section
func Section(buf *proto.Section) *discordgo.Section {
	if buf == nil {
		return nil
	}

	return &discordgo.Section{
		ID:         int(buf.Id),
		Components: MessageComponents(buf.Components),
		Accessory:  MessageComponent(buf.Accessory),
	}
}

// TextDisplay converts proto.TextDisplay to discordgo.TextDisplay
func TextDisplay(buf *proto.TextDisplay) *discordgo.TextDisplay {
	if buf == nil {
		return nil
	}

	return &discordgo.TextDisplay{
		Content: buf.Content,
	}
}

// UnfurledMediaItem converts proto.UnfurledMediaItem to discordgo.UnfurledMediaItem
func UnfurledMediaItem(buf *proto.UnfurledMediaItem) *discordgo.UnfurledMediaItem {
	if buf == nil {
		return nil
	}

	return &discordgo.UnfurledMediaItem{
		URL: buf.Url,
	}
}

// Thumbnail converts proto.Thumbnail to discordgo.Thumbnail
func Thumbnail(buf *proto.Thumbnail) *discordgo.Thumbnail {
	if buf == nil {
		return nil
	}

	result := &discordgo.Thumbnail{
		ID:      int(buf.Id),
		Spoiler: buf.Spoiler,
	}

	if buf.Media != nil {
		result.Media = *UnfurledMediaItem(buf.Media)
	}

	if buf.Description != nil {
		result.Description = buf.Description
	}

	return result
}

// MediaGalleryItem converts proto.MediaGalleryItem to discordgo.MediaGalleryItem
func MediaGalleryItem(buf *proto.MediaGalleryItem) *discordgo.MediaGalleryItem {
	if buf == nil {
		return nil
	}

	result := &discordgo.MediaGalleryItem{
		Spoiler: buf.Spoiler,
	}

	if buf.Media != nil {
		result.Media = *UnfurledMediaItem(buf.Media)
	}

	if buf.Description != nil {
		result.Description = buf.Description
	}

	return result
}

// MediaGallery converts proto.MediaGallery to discordgo.MediaGallery
func MediaGallery(buf *proto.MediaGallery) *discordgo.MediaGallery {
	if buf == nil {
		return nil
	}

	items := make([]discordgo.MediaGalleryItem, 0, len(buf.Items))
	for _, item := range buf.Items {
		if converted := MediaGalleryItem(item); converted != nil {
			items = append(items, *converted)
		}
	}

	return &discordgo.MediaGallery{
		ID:    int(buf.Id),
		Items: items,
	}
}

// FileComponent converts proto.FileComponent to discordgo.FileComponent
func FileComponent(buf *proto.FileComponent) *discordgo.FileComponent {
	if buf == nil {
		return nil
	}

	result := &discordgo.FileComponent{
		ID:      int(buf.Id),
		Spoiler: buf.Spoiler,
	}

	if buf.File != nil {
		result.File = *UnfurledMediaItem(buf.File)
	}

	return result
}

// Separator converts proto.Separator to discordgo.Separator
func Separator(buf *proto.Separator) *discordgo.Separator {
	if buf == nil {
		return nil
	}

	result := &discordgo.Separator{
		ID: int(buf.Id),
	}

	if buf.Divider != nil {
		result.Divider = buf.Divider
	}

	if buf.Spacing != nil {
		spacing := discordgo.SeparatorSpacingSize(*buf.Spacing)
		result.Spacing = &spacing
	}

	return result
}

// Container converts proto.Container to discordgo.Container
func Container(buf *proto.Container) *discordgo.Container {
	if buf == nil {
		return nil
	}

	result := &discordgo.Container{
		ID:         int(buf.Id),
		Spoiler:    buf.Spoiler,
		Components: MessageComponents(buf.Components),
	}

	if buf.AccentColor != nil {
		accentColor := int(*buf.AccentColor)
		result.AccentColor = &accentColor
	}

	return result
}
