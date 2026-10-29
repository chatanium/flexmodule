package discordgo

import (
	"bytes"
	"errors"
)

var errGatewayPayload = errors.New("invalid Gateway event payload")

// Valid JSON can still decode to a nil embedded object or a missing required
// pointer that the SDK cache dereferences. Reject those known payloads through
// the selective observer error path, before touching cache/voice state.
func validGatewayStatePayload(event interface{}) bool {
	member := func(m *Member) bool { return m != nil && m.User != nil }
	presence := func(p *Presence) bool { return p != nil && p.User != nil }
	guild := func(g *Guild) bool {
		if g == nil {
			return false
		}
		for _, c := range g.Channels {
			if c == nil {
				return false
			}
		}
		for _, c := range g.Threads {
			if c == nil {
				return false
			}
		}
		for _, m := range g.Members {
			if !member(m) {
				return false
			}
		}
		for _, v := range g.VoiceStates {
			if v == nil {
				return false
			}
		}
		return true
	}
	switch e := event.(type) {
	case *Ready:
		if e.User == nil {
			return false
		}
		for _, g := range e.Guilds {
			if !guild(g) {
				return false
			}
		}
		for _, c := range e.PrivateChannels {
			if c == nil {
				return false
			}
		}
	case *GuildCreate:
		return guild(e.Guild)
	case *GuildUpdate:
		return guild(e.Guild)
	case *GuildDelete:
		return e.Guild != nil
	case *GuildMemberAdd:
		return member(e.Member)
	case *GuildMemberUpdate:
		return member(e.Member)
	case *GuildMemberRemove:
		return member(e.Member)
	case *GuildMembersChunk:
		for _, m := range e.Members {
			if !member(m) {
				return false
			}
		}
		for _, p := range e.Presences {
			if !presence(p) {
				return false
			}
		}
	case *GuildRoleCreate:
		return e.GuildRole != nil && e.Role != nil
	case *GuildRoleUpdate:
		return e.GuildRole != nil && e.Role != nil
	case *ChannelCreate:
		return e.Channel != nil
	case *ChannelUpdate:
		return e.Channel != nil
	case *ChannelDelete:
		return e.Channel != nil
	case *ThreadCreate:
		return e.Channel != nil
	case *ThreadUpdate:
		return e.Channel != nil
	case *ThreadDelete:
		return e.Channel != nil
	case *ThreadMemberUpdate:
		return e.ThreadMember != nil
	case *ThreadMembersUpdate:
		for _, m := range e.AddedMembers {
			if m.ThreadMember == nil || (m.Presence != nil && !presence(m.Presence)) {
				return false
			}
		}
	case *ThreadListSync:
		for _, c := range e.Threads {
			if c == nil {
				return false
			}
		}
		for _, m := range e.Members {
			if m == nil {
				return false
			}
		}
	case *MessageCreate:
		return e.Message != nil
	case *MessageUpdate:
		return e.Message != nil
	case *MessageDelete:
		return e.Message != nil
	case *VoiceStateUpdate:
		return e.VoiceState != nil
	case *PresenceUpdate:
		return e.User != nil
	}
	return true
}

func validGatewayPayloadShape(eventType string, raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return false
	}
	if eventType == presencesReplaceEventType {
		return raw[0] == '['
	}
	return raw[0] == '{'
}
