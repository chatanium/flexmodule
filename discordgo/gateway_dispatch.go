package discordgo

import "reflect"

// SetDispatchObserver installs the single host ingress observer. Passing nil
// detaches ingress/fatal callbacks and joins all callbacks already in flight.
// For each Discord opcode-0 dispatch it runs synchronously, in receive order,
// after internal state application and before ordinary SDK handlers. No Session,
// State or handler lock is held while calling it. It includes host-private voice
// events, not synthetic/raw duplicate events. RawData retains the exact d JSON;
// Struct has detached BeforeUpdate/BeforeDelete only when actually cached.
//
// The host must bound its callback and project/enqueue synchronously; it must not
// invoke module code, mutate the event/cache, reconfigure/detach callbacks, or
// open/close this session from a callback. Decode errors are reported rather than silently projected as zero
// values. Raw events and credentials must never cross the module boundary.
func (s *Session) SetDispatchObserver(observer func(*Session, *Event, error)) {
	s.dispatchMu.Lock()
	s.dispatchObserver = observer
	s.gatewayCallbacksDetached = observer == nil
	s.dispatchMu.Unlock()
}

// snapshotVisit preserves shared references and cycles without sharing mutable
// storage with the live cache. JSON cloning is incorrect here: SDK-only channel
// messages/members and Message.Components deliberately have json:"-" tags.
type snapshotVisit struct {
	typ    reflect.Type
	ptr    uintptr
	length int
}

func cloneDispatchValue(v reflect.Value, seen map[snapshotVisit]reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		key := snapshotVisit{typ: v.Type(), ptr: v.Pointer()}
		if old, ok := seen[key]; ok {
			return old
		}
		out := reflect.New(v.Type().Elem())
		seen[key] = out
		out.Elem().Set(cloneDispatchValue(v.Elem(), seen))
		return out
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(cloneDispatchValue(v.Elem(), seen))
		return out
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		key := snapshotVisit{typ: v.Type(), ptr: v.Pointer(), length: v.Len()}
		if old, ok := seen[key]; ok {
			return old
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		seen[key] = out
		for i := range v.Len() {
			out.Index(i).Set(cloneDispatchValue(v.Index(i), seen))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		key := snapshotVisit{typ: v.Type(), ptr: uintptr(v.UnsafePointer())}
		if old, ok := seen[key]; ok {
			return old
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		seen[key] = out
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneDispatchValue(iter.Value(), seen))
		}
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		// Unexported fields belong to immutable value types such as time.Time;
		// copy those values without unsafe access to their implementation.
		out.Set(v)
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				out.Field(i).Set(cloneDispatchValue(v.Field(i), seen))
			}
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := range v.Len() {
			out.Index(i).Set(cloneDispatchValue(v.Index(i), seen))
		}
		return out
	default:
		return v
	}
}

// cachedBefore selects only source wrappers that actually define before fields,
// and only when their corresponding cache tracking policy is enabled. It never
// fetches a missing object or manufactures a before value from the new event.
func (s *Session) cachedBefore(event interface{}) interface{} {
	state := s.State
	if state == nil || !s.StateEnabled {
		return nil
	}
	state.RLock()
	defer state.RUnlock()
	var old interface{}
	channel := func(id string, track bool) {
		if track && state.channelMap[id] != nil {
			old = state.channelMap[id]
		}
	}
	member := func(guildID, userID string) {
		if state.TrackMembers && state.memberMap[guildID][userID] != nil {
			old = state.memberMap[guildID][userID]
		}
	}
	role := func(guildID, roleID string) {
		if guild := state.guildMap[guildID]; state.TrackRoles && guild != nil {
			for _, value := range guild.Roles {
				if value != nil && value.ID == roleID {
					old = value
					break
				}
			}
		}
	}
	message := func(channelID, messageID string) {
		if ch := state.channelMap[channelID]; state.MaxMessageCount != 0 && ch != nil {
			for _, value := range ch.Messages {
				if value != nil && value.ID == messageID {
					old = value
					break
				}
			}
		}
	}
	switch e := event.(type) {
	case *GuildDelete:
		if e.Guild != nil && state.guildMap[e.ID] != nil {
			old = state.guildMap[e.ID]
		}
	case *ChannelUpdate:
		if e.Channel != nil {
			channel(e.ID, state.TrackChannels)
		}
	case *ChannelDelete:
		if e.Channel != nil {
			channel(e.ID, state.TrackChannels)
		}
	case *ThreadUpdate:
		if e.Channel != nil {
			channel(e.ID, state.TrackThreads)
		}
	case *ThreadDelete:
		if e.Channel != nil {
			channel(e.ID, state.TrackThreads)
		}
	case *GuildMemberUpdate:
		if e.Member != nil && e.User != nil {
			member(e.GuildID, e.User.ID)
		}
	case *GuildMemberRemove:
		if e.Member != nil && e.User != nil {
			member(e.GuildID, e.User.ID)
		}
	case *GuildRoleUpdate:
		if e.GuildRole != nil && e.Role != nil {
			role(e.GuildID, e.Role.ID)
		}
	case *GuildRoleDelete:
		role(e.GuildID, e.RoleID)
	case *MessageUpdate:
		if e.Message != nil {
			message(e.ChannelID, e.ID)
		}
	case *MessageDelete:
		if e.Message != nil {
			message(e.ChannelID, e.ID)
		}
	case *VoiceStateUpdate:
		if e.VoiceState != nil && state.TrackVoice {
			if guild := state.guildMap[e.GuildID]; guild != nil {
				for _, value := range guild.VoiceStates {
					if value != nil && value.UserID == e.UserID {
						old = value
						break
					}
				}
			}
		}
	}
	if old == nil {
		return nil
	}
	return cloneDispatchValue(reflect.ValueOf(old), make(map[snapshotVisit]reflect.Value)).Interface()
}

func setDispatchBefore(event, before interface{}) {
	switch e := event.(type) {
	case *GuildDelete:
		e.BeforeDelete, _ = before.(*Guild)
	case *ChannelUpdate:
		e.BeforeUpdate, _ = before.(*Channel)
	case *ChannelDelete:
		e.BeforeDelete, _ = before.(*Channel)
	case *ThreadUpdate:
		e.BeforeUpdate, _ = before.(*Channel)
	case *ThreadDelete:
		e.BeforeDelete, _ = before.(*Channel)
	case *GuildMemberUpdate:
		e.BeforeUpdate, _ = before.(*Member)
	case *GuildMemberRemove:
		e.BeforeDelete, _ = before.(*Member)
	case *GuildRoleUpdate:
		e.BeforeUpdate, _ = before.(*Role)
	case *GuildRoleDelete:
		e.BeforeDelete, _ = before.(*Role)
	case *MessageUpdate:
		e.BeforeUpdate, _ = before.(*Message)
	case *MessageDelete:
		e.BeforeDelete, _ = before.(*Message)
	case *VoiceStateUpdate:
		e.BeforeUpdate, _ = before.(*VoiceState)
	}
}
