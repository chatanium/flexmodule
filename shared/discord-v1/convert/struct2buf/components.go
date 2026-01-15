package struct2buf

import (
	"github.com/bwmarrin/discordgo"
	proto "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
)

// MessageComponent converts a discordgo.MessageComponent interface to proto.MessageComponent
func MessageComponent(s discordgo.MessageComponent) *proto.MessageComponent {
	if s == nil {
		return nil
	}

	switch c := s.(type) {
	case *discordgo.ActionsRow:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_ActionsRow{
				ActionsRow: ActionsRow(c),
			},
		}
	case discordgo.ActionsRow:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_ActionsRow{
				ActionsRow: ActionsRow(&c),
			},
		}
	case *discordgo.Button:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Button{
				Button: Button(c),
			},
		}
	case discordgo.Button:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Button{
				Button: Button(&c),
			},
		}
	case *discordgo.SelectMenu:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_SelectMenu{
				SelectMenu: SelectMenu(c),
			},
		}
	case discordgo.SelectMenu:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_SelectMenu{
				SelectMenu: SelectMenu(&c),
			},
		}
	case *discordgo.TextInput:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_TextInput{
				TextInput: TextInput(c),
			},
		}
	case discordgo.TextInput:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_TextInput{
				TextInput: TextInput(&c),
			},
		}
	case *discordgo.Section:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Section{
				Section: Section(c),
			},
		}
	case discordgo.Section:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Section{
				Section: Section(&c),
			},
		}
	case *discordgo.TextDisplay:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_TextDisplay{
				TextDisplay: TextDisplay(c),
			},
		}
	case discordgo.TextDisplay:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_TextDisplay{
				TextDisplay: TextDisplay(&c),
			},
		}
	case *discordgo.Thumbnail:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Thumbnail{
				Thumbnail: Thumbnail(c),
			},
		}
	case discordgo.Thumbnail:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Thumbnail{
				Thumbnail: Thumbnail(&c),
			},
		}
	case *discordgo.MediaGallery:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_MediaGallery{
				MediaGallery: MediaGallery(c),
			},
		}
	case discordgo.MediaGallery:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_MediaGallery{
				MediaGallery: MediaGallery(&c),
			},
		}
	case *discordgo.FileComponent:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_FileComponent{
				FileComponent: FileComponent(c),
			},
		}
	case discordgo.FileComponent:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_FileComponent{
				FileComponent: FileComponent(&c),
			},
		}
	case *discordgo.Separator:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Separator{
				Separator: Separator(c),
			},
		}
	case discordgo.Separator:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Separator{
				Separator: Separator(&c),
			},
		}
	case *discordgo.Container:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Container{
				Container: Container(c),
			},
		}
	case discordgo.Container:
		return &proto.MessageComponent{
			Component: &proto.MessageComponent_Container{
				Container: Container(&c),
			},
		}
	default:
		return nil
	}
}

// ActionsRow converts discordgo.ActionsRow to proto.ActionsRow
func ActionsRow(s *discordgo.ActionsRow) *proto.ActionsRow {
	if s == nil {
		return nil
	}

	components := make([]*proto.MessageComponent, 0, len(s.Components))
	for _, c := range s.Components {
		if converted := MessageComponent(c); converted != nil {
			components = append(components, converted)
		}
	}

	return &proto.ActionsRow{
		Components: components,
		Id:         int32(s.ID),
	}
}

// Button converts discordgo.Button to proto.Button
func Button(s *discordgo.Button) *proto.Button {
	if s == nil {
		return nil
	}

	return &proto.Button{
		Label:    s.Label,
		Style:    proto.ButtonStyle(s.Style),
		Disabled: s.Disabled,
		Emoji:    ComponentEmoji(s.Emoji),
		Url:      s.URL,
		CustomId: s.CustomID,
		SkuId:    s.SKUID,
		Id:       int32(s.ID),
	}
}

// ComponentEmoji converts discordgo.ComponentEmoji to proto.ComponentEmoji
func ComponentEmoji(s *discordgo.ComponentEmoji) *proto.ComponentEmoji {
	if s == nil {
		return nil
	}

	return &proto.ComponentEmoji{
		Name:     s.Name,
		Id:       s.ID,
		Animated: s.Animated,
	}
}

// SelectMenu converts discordgo.SelectMenu to proto.SelectMenu
func SelectMenu(s *discordgo.SelectMenu) *proto.SelectMenu {
	if s == nil {
		return nil
	}

	options := make([]*proto.SelectMenuOption, 0, len(s.Options))
	for _, opt := range s.Options {
		options = append(options, SelectMenuOption(&opt))
	}

	defaultValues := make([]*proto.SelectMenuDefaultValue, 0, len(s.DefaultValues))
	for _, dv := range s.DefaultValues {
		defaultValues = append(defaultValues, SelectMenuDefaultValue(&dv))
	}

	channelTypes := make([]int32, 0, len(s.ChannelTypes))
	for _, ct := range s.ChannelTypes {
		channelTypes = append(channelTypes, int32(ct))
	}

	result := &proto.SelectMenu{
		MenuType:      proto.ComponentType(s.MenuType),
		CustomId:      s.CustomID,
		Placeholder:   s.Placeholder,
		MaxValues:     int32(s.MaxValues),
		DefaultValues: defaultValues,
		Options:       options,
		Disabled:      s.Disabled,
		ChannelTypes:  channelTypes,
		Id:            int32(s.ID),
	}

	if s.MinValues != nil {
		minVal := int32(*s.MinValues)
		result.MinValues = &minVal
	}

	return result
}

// SelectMenuOption converts discordgo.SelectMenuOption to proto.SelectMenuOption
func SelectMenuOption(s *discordgo.SelectMenuOption) *proto.SelectMenuOption {
	if s == nil {
		return nil
	}

	return &proto.SelectMenuOption{
		Label:       s.Label,
		Value:       s.Value,
		Description: s.Description,
		Emoji:       ComponentEmoji(s.Emoji),
		IsDefault:   s.Default,
	}
}

// SelectMenuDefaultValue converts discordgo.SelectMenuDefaultValue to proto.SelectMenuDefaultValue
func SelectMenuDefaultValue(s *discordgo.SelectMenuDefaultValue) *proto.SelectMenuDefaultValue {
	if s == nil {
		return nil
	}

	var valueType proto.SelectMenuDefaultValueType
	switch s.Type {
	case discordgo.SelectMenuDefaultValueUser:
		valueType = proto.SelectMenuDefaultValueType_SELECT_MENU_DEFAULT_VALUE_TYPE_USER
	case discordgo.SelectMenuDefaultValueRole:
		valueType = proto.SelectMenuDefaultValueType_SELECT_MENU_DEFAULT_VALUE_TYPE_ROLE
	case discordgo.SelectMenuDefaultValueChannel:
		valueType = proto.SelectMenuDefaultValueType_SELECT_MENU_DEFAULT_VALUE_TYPE_CHANNEL
	}

	return &proto.SelectMenuDefaultValue{
		Id:   s.ID,
		Type: valueType,
	}
}

// TextInput converts discordgo.TextInput to proto.TextInput
func TextInput(s *discordgo.TextInput) *proto.TextInput {
	if s == nil {
		return nil
	}

	return &proto.TextInput{
		CustomId:    s.CustomID,
		Label:       s.Label,
		Style:       proto.TextInputStyle(s.Style),
		Placeholder: s.Placeholder,
		Value:       s.Value,
		Required:    s.Required,
		MinLength:   int32(s.MinLength),
		MaxLength:   int32(s.MaxLength),
		Id:          int32(s.ID),
	}
}

// Section converts discordgo.Section to proto.Section
func Section(s *discordgo.Section) *proto.Section {
	if s == nil {
		return nil
	}

	components := make([]*proto.MessageComponent, 0, len(s.Components))
	for _, c := range s.Components {
		if converted := MessageComponent(c); converted != nil {
			components = append(components, converted)
		}
	}

	return &proto.Section{
		Id:         int32(s.ID),
		Components: components,
		Accessory:  MessageComponent(s.Accessory),
	}
}

// TextDisplay converts discordgo.TextDisplay to proto.TextDisplay
func TextDisplay(s *discordgo.TextDisplay) *proto.TextDisplay {
	if s == nil {
		return nil
	}

	return &proto.TextDisplay{
		Content: s.Content,
	}
}

// UnfurledMediaItem converts discordgo.UnfurledMediaItem to proto.UnfurledMediaItem
func UnfurledMediaItem(s *discordgo.UnfurledMediaItem) *proto.UnfurledMediaItem {
	if s == nil {
		return nil
	}

	return &proto.UnfurledMediaItem{
		Url: s.URL,
	}
}

// Thumbnail converts discordgo.Thumbnail to proto.Thumbnail
func Thumbnail(s *discordgo.Thumbnail) *proto.Thumbnail {
	if s == nil {
		return nil
	}

	result := &proto.Thumbnail{
		Id:      int32(s.ID),
		Media:   UnfurledMediaItem(&s.Media),
		Spoiler: s.Spoiler,
	}

	if s.Description != nil {
		result.Description = s.Description
	}

	return result
}

// MediaGalleryItem converts discordgo.MediaGalleryItem to proto.MediaGalleryItem
func MediaGalleryItem(s *discordgo.MediaGalleryItem) *proto.MediaGalleryItem {
	if s == nil {
		return nil
	}

	result := &proto.MediaGalleryItem{
		Media:   UnfurledMediaItem(&s.Media),
		Spoiler: s.Spoiler,
	}

	if s.Description != nil {
		result.Description = s.Description
	}

	return result
}

// MediaGallery converts discordgo.MediaGallery to proto.MediaGallery
func MediaGallery(s *discordgo.MediaGallery) *proto.MediaGallery {
	if s == nil {
		return nil
	}

	items := make([]*proto.MediaGalleryItem, 0, len(s.Items))
	for _, item := range s.Items {
		items = append(items, MediaGalleryItem(&item))
	}

	return &proto.MediaGallery{
		Id:    int32(s.ID),
		Items: items,
	}
}

// FileComponent converts discordgo.FileComponent to proto.FileComponent
func FileComponent(s *discordgo.FileComponent) *proto.FileComponent {
	if s == nil {
		return nil
	}

	return &proto.FileComponent{
		Id:      int32(s.ID),
		File:    UnfurledMediaItem(&s.File),
		Spoiler: s.Spoiler,
	}
}

// Separator converts discordgo.Separator to proto.Separator
func Separator(s *discordgo.Separator) *proto.Separator {
	if s == nil {
		return nil
	}

	result := &proto.Separator{
		Id: int32(s.ID),
	}

	if s.Divider != nil {
		result.Divider = s.Divider
	}

	if s.Spacing != nil {
		spacing := proto.SeparatorSpacingSize(*s.Spacing)
		result.Spacing = &spacing
	}

	return result
}

// Container converts discordgo.Container to proto.Container
func Container(s *discordgo.Container) *proto.Container {
	if s == nil {
		return nil
	}

	components := make([]*proto.MessageComponent, 0, len(s.Components))
	for _, c := range s.Components {
		if converted := MessageComponent(c); converted != nil {
			components = append(components, converted)
		}
	}

	result := &proto.Container{
		Id:         int32(s.ID),
		Spoiler:    s.Spoiler,
		Components: components,
	}

	if s.AccentColor != nil {
		accentColor := int32(*s.AccentColor)
		result.AccentColor = &accentColor
	}

	return result
}
