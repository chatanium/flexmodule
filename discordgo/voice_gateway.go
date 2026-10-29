//go:build voice && cgo

package discordgo

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo/internal/dave"
	"github.com/gorilla/websocket"
)

func (n *nativeVoice) roster() []string {
	ids := make([]string, 0, len(n.participants))
	for id := range n.participants {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
func (n *nativeVoice) control(m voiceMessage) error {
	if m.kind == websocket.BinaryMessage {
		seq, op, b, err := parseVoiceBinary(m.data, n.v.config.MaxWebSocketBytes)
		if err != nil {
			return err
		}
		n.seqAck = int(seq)
		switch op {
		case 25:
			if len(b) == 0 || len(b) > n.v.config.MaxProposalBytes {
				return errors.New("invalid external sender size")
			}
			n.external = append(n.external[:0], b...)
			if err := n.dave.SetExternalSender(n.ctx, b); err != nil {
				return err
			}
			// A delayed external sender may create the pending group after26.
			// Re-grant only the still-unconsumed matching epoch-one reset.
			return n.authorizePendingReset()
		case 27:
			if len(b) < 2 || len(b)-1 > n.v.config.MaxProposalBytes || b[0] > 1 {
				return errors.New("invalid DAVE proposal framing")
			}
			if n.proposalDeadline.IsZero() {
				n.proposalDeadline = time.Now().Add(n.v.config.RekeyTimeout)
			}
			out, err := n.dave.ProcessProposals(n.ctx, dave.ProposalOperation(b[0]), b[1:], n.roster())
			if err != nil {
				return err
			}
			if len(out.Wire) == 0 {
				// The patched native parser has retired its final queued proposal
				// and outbound candidate. Only this negotiation is complete.
				n.proposalDeadline = time.Time{}
				return nil
			}
			return n.writeBinary(n.ctx, 28, out.Wire)
		case 29, 30:
			if len(b) < 3 || len(b)-2 > n.v.config.MaxTransitionBytes {
				return errors.New("invalid DAVE transition framing")
			}
			id := binary.BigEndian.Uint16(b[:2])
			_, alreadyPrepared := n.transitions[id]
			if err := n.addTransition(id); err != nil {
				return err
			}
			if op == 29 {
				result, err := n.dave.ProcessCommit(n.ctx, id, b[2:])
				if err != nil || result == dave.CommitFailed {
					return n.recoverInvalid(id)
				}
				if result == dave.CommitIgnored {
					if !alreadyPrepared {
						delete(n.transitions, id)
					}
					return nil
				}
			} else if err := n.dave.ProcessWelcome(n.ctx, id, b[2:], n.roster()); err != nil {
				return n.recoverInvalid(id)
			}
			if err := n.dave.PrepareReceivers(n.ctx, id); err != nil {
				return n.recoverInvalid(id)
			}
			// Successful MLS establishment supersedes reset/proposal negotiation;
			// the prepared ID retains its own deadline until execution.
			n.resetDeadline = time.Time{}
			n.proposalDeadline = time.Time{}
			n.clearPendingResetAuthorization()
			if id == 0 {
				n.finishTransition(id)
				return nil
			} // Native prepares and executes immediate zero.
			return n.writeJSON(n.ctx, 23, voiceTransition{id})
		default:
			return errors.New("unexpected server binary voice opcode")
		}
	}
	var e voiceEnvelope
	if err := json.Unmarshal(m.data, &e); err != nil {
		return errors.New("invalid voice text framing")
	}
	if e.Seq != nil {
		n.seqAck = int(*e.Seq)
	}
	if fields := voiceRequiredFields[e.Op]; fields != nil {
		if err := requireVoiceFields(e.D, fields...); err != nil {
			return err
		}
	}
	decode := func(dst any) error {
		if len(e.D) == 0 || string(e.D) == "null" {
			return errors.New("missing voice opcode data")
		}
		if err := json.Unmarshal(e.D, dst); err != nil {
			return errors.New("invalid voice opcode data")
		}
		return nil
	}
	switch e.Op {
	case 8:
		var d struct {
			Interval float64 `json:"heartbeat_interval"`
		}
		if err := decode(&d); err != nil {
			return err
		}
		if math.IsNaN(d.Interval) || math.IsInf(d.Interval, 0) || d.Interval <= 0 || d.Interval > float64(math.MaxInt64)/float64(time.Millisecond) {
			return errors.New("invalid voice heartbeat interval")
		}
		interval := time.Duration(d.Interval * float64(time.Millisecond))
		if interval <= 0 {
			return errors.New("invalid voice heartbeat interval")
		}
		n.heartbeatMu.Lock()
		if n.heartbeatInterval != 0 {
			n.heartbeatMu.Unlock()
			return errors.New("duplicate voice Hello")
		}
		n.heartbeatInterval = interval
		n.heartbeatMu.Unlock()
		select {
		case n.heartbeatWake <- struct{}{}:
		default:
		}
		return nil
	case 6:
		var d struct {
			T int64 `json:"t"`
		}
		if err := decode(&d); err != nil {
			return err
		}
		n.heartbeatMu.Lock()
		defer n.heartbeatMu.Unlock()
		if !n.heartbeatPending || d.T != n.heartbeatNonce {
			return errors.New("unexpected voice heartbeat ACK")
		}
		n.heartbeatPending = false
		select {
		case n.heartbeatWake <- struct{}{}:
		default:
		}
		return nil
	case 2:
		var d struct {
			SSRC  uint32   `json:"ssrc"`
			IP    string   `json:"ip"`
			Port  int      `json:"port"`
			Modes []string `json:"modes"`
		}
		if err := decode(&d); err != nil {
			return err
		}
		if n.mode != "" {
			return errors.New("duplicate voice Ready")
		}
		mode, err := selectVoiceMode(d.Modes)
		if err != nil {
			return err
		}
		n.mode = mode
		n.ssrc = d.SSRC
		return n.discoverUDP(d.IP, d.Port)
	case 4:
		var d struct {
			Mode    string `json:"mode"`
			Key     []int  `json:"secret_key"`
			Version uint16 `json:"dave_protocol_version"`
		}
		if err := decode(&d); err != nil {
			return err
		}
		if n.aead != nil || d.Mode != n.mode || len(d.Key) != 32 {
			return errors.New("invalid voice session description")
		}
		var key [32]byte
		for i, b := range d.Key {
			if b < 0 || b > 255 {
				return errors.New("invalid transport key")
			}
			key[i] = byte(b)
		}
		aead, err := voiceAEAD(d.Mode, key[:])
		clear(key[:])
		clear(d.Key)
		if err != nil {
			return err
		}
		n.aead = aead
		n.mu.Lock()
		n.transportReady = true
		n.mu.Unlock()
		if err = n.initializeDAVE(d.Version, false); err != nil {
			return err
		}
		return nil
	case 5:
		var d struct {
			UserID   string `json:"user_id"`
			SSRC     uint32 `json:"ssrc"`
			Speaking int    `json:"speaking"`
		}
		if err := decode(&d); err != nil {
			return err
		}
		if _, ok := n.participants[d.UserID]; !ok {
			return nil
		}
		if old, ok := n.senders[d.SSRC]; ok && old != d.UserID {
			return errors.New("voice SSRC identity conflict")
		}
		if len(n.senders) >= n.v.config.MaxDecryptors {
			if _, ok := n.senders[d.SSRC]; !ok {
				return errors.New("voice sender bound exceeded")
			}
		}
		n.senders[d.SSRC] = d.UserID
		return nil
	case 11:
		var d struct {
			IDs []string `json:"user_ids"`
		}
		if err := decode(&d); err != nil {
			return err
		}
		if len(d.IDs) > n.v.config.MaxParticipants {
			return errors.New("participant bound exceeded")
		}
		for _, id := range d.IDs {
			if _, err := strconv.ParseUint(id, 10, 64); err != nil {
				return errors.New("invalid participant ID")
			}
			if _, ok := n.participants[id]; !ok && len(n.participants) >= n.v.config.MaxParticipants {
				return errors.New("participant bound exceeded")
			}
			n.participants[id] = struct{}{}
		}
		return n.dave.SetParticipants(n.ctx, n.roster())
	case 13:
		var d struct {
			ID string `json:"user_id"`
		}
		if err := decode(&d); err != nil {
			return err
		}
		if d.ID == n.v.UserID {
			return errors.New("voice local participant disconnected")
		}
		delete(n.participants, d.ID)
		for ssrc, id := range n.senders {
			if id == d.ID {
				delete(n.senders, ssrc)
			}
		}
		return n.dave.SetParticipants(n.ctx, n.roster())
	case 21:
		var d voicePrepareTransition
		if err := decode(&d); err != nil {
			return err
		}
		if d.Version == 0 || d.Version > dave.MaxSupportedProtocolVersion() {
			return ErrVoiceUnavailable
		}
		if err := n.addTransition(d.ID); err != nil {
			return err
		}
		if err := n.dave.PrepareTransition(n.ctx, d.ID, d.Version); err != nil {
			return err
		}
		n.resetDeadline = time.Time{}
		n.clearPendingResetAuthorization()
		if d.ID == 0 {
			n.finishTransition(0)
			return nil
		}
		return n.writeJSON(n.ctx, 23, voiceTransition{d.ID})
	case 22:
		var d voiceTransition
		if err := decode(&d); err != nil {
			return err
		}
		if _, ok := n.transitions[d.ID]; !ok {
			return errors.New("unprepared voice transition")
		}
		if err := n.dave.ExecuteTransition(n.ctx, d.ID); err != nil {
			return err
		}
		n.finishTransition(d.ID)
		return nil
	case 24:
		var d voicePrepareEpoch
		if err := decode(&d); err != nil {
			return err
		}
		if d.Epoch == 0 {
			return errors.New("invalid DAVE epoch")
		}
		if d.Version == 0 || d.Version > dave.MaxSupportedProtocolVersion() {
			return ErrVoiceUnavailable
		}
		if d.Epoch == 1 {
			return n.initializeDAVE(d.Version, true)
		}
		n.clearPendingResetAuthorization()
		n.beginReset()
		if err := n.dave.SetProtocolVersion(n.ctx, d.Version); err != nil {
			return err
		}
		n.version = d.Version
		return nil
	case 9:
		return errors.New("unsolicited voice Resume")
	case 12, 14, 15, 16, 17, 18, 20: // non-audio/video metadata does not alter media state
		return nil
	case 0, 1, 3, 7, 23, 25, 26, 27, 28, 29, 30, 31:
		return errors.New("unexpected server voice opcode direction or framing")
	default:
		return nil // bounded future metadata; never interpreted as media/key material
	}
}
func (n *nativeVoice) nextRekeyDeadline() time.Time {
	deadline := n.resetDeadline
	if !n.proposalDeadline.IsZero() && (deadline.IsZero() || n.proposalDeadline.Before(deadline)) {
		deadline = n.proposalDeadline
	}
	for _, prepared := range n.transitions {
		if deadline.IsZero() || prepared.Before(deadline) {
			deadline = prepared
		}
	}
	return deadline
}
func (n *nativeVoice) beginReset() {
	deadline := n.nextRekeyDeadline()
	if deadline.IsZero() {
		deadline = time.Now().Add(n.v.config.RekeyTimeout)
	}
	n.resetDeadline = deadline
}
func (n *nativeVoice) addTransition(id uint16) error {
	if _, ok := n.transitions[id]; !ok && len(n.transitions) >= n.v.config.MaxTransitions {
		return errors.New("DAVE transition bound exceeded")
	}
	if _, ok := n.transitions[id]; !ok {
		n.transitions[id] = time.Now().Add(n.v.config.RekeyTimeout)
	}
	return nil
}
func (n *nativeVoice) finishTransition(id uint16) {
	delete(n.transitions, id)
}
func (n *nativeVoice) clearPendingResetAuthorization() {
	n.resetAuthorizationGeneration = 0
	n.resetAuthorizationVersion = 0
	n.resetPackageSent = false
}
func (n *nativeVoice) authorizePendingReset() error {
	if !n.resetPackageSent || n.resetAuthorizationGeneration != n.daveGeneration ||
		n.resetAuthorizationVersion != n.version || len(n.external) == 0 {
		return nil
	}
	return n.dave.AuthorizePendingReset(n.ctx, n.resetAuthorizationVersion)
}
func (n *nativeVoice) initializeDAVE(version uint16, authorizeEpochOne bool) error {
	if version == 0 || version > dave.MaxSupportedProtocolVersion() {
		return ErrVoiceUnavailable
	}
	n.beginReset()
	n.clearPendingResetAuthorization()
	if n.daveGeneration == ^uint64(0) {
		return errors.New("DAVE reset generation exhausted")
	}
	n.daveGeneration++
	n.proposalDeadline = time.Time{}
	n.version = version
	clear(n.transitions)
	if err := n.dave.Initialize(n.ctx, version, n.v.ChannelID, n.v.UserID); err != nil {
		return err
	}
	if len(n.external) > 0 {
		if err := n.dave.SetExternalSender(n.ctx, n.external); err != nil {
			return err
		}
	}
	if err := n.dave.SetParticipants(n.ctx, n.roster()); err != nil {
		return err
	}
	key, err := n.dave.FreshKeyPackage(n.ctx)
	if err != nil {
		return err
	}
	if err := n.writeBinary(n.ctx, 26, key); err != nil {
		return err
	}
	if authorizeEpochOne {
		// Only authenticated24(epoch=1) reaches this record, after this reset
		// generation's fresh package was actually written. A later25 may finish
		// its authorization, but cannot create initial-only or consumed grants.
		n.resetAuthorizationGeneration = n.daveGeneration
		n.resetAuthorizationVersion = version
		n.resetPackageSent = true
		return n.authorizePendingReset()
	}
	return nil
}
func (n *nativeVoice) recoverInvalid(id uint16) error {
	if err := n.writeJSON(n.ctx, 31, voiceTransition{id}); err != nil {
		return err
	}
	return n.initializeDAVE(n.version, false)
}
func (n *nativeVoice) sendHeartbeat() error {
	n.heartbeatMu.Lock()
	if n.heartbeatPending {
		n.heartbeatMu.Unlock()
		return nil
	}
	nonce := time.Now().UnixMilli()
	if nonce <= n.heartbeatNonce {
		nonce = n.heartbeatNonce + 1
	}
	n.heartbeatNonce = nonce
	n.heartbeatSent = time.Now()
	n.heartbeatPending = true
	n.heartbeatMu.Unlock()
	return n.writeJSON(n.ctx, 3, voiceHeartbeatData{nonce, n.seqAck})
}
func (n *nativeVoice) heartbeatLoop() {
	defer n.wg.Done()
	timer := time.NewTimer(n.v.config.HeartbeatTimeout)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-n.heartbeatWake:
		case <-timer.C:
		}
		n.heartbeatMu.Lock()
		interval, pending, sent := n.heartbeatInterval, n.heartbeatPending, n.heartbeatSent
		n.heartbeatMu.Unlock()
		if interval == 0 {
			continue
		}
		elapsed := time.Since(sent)
		if pending && elapsed >= n.v.config.HeartbeatTimeout {
			n.stop(errors.New("voice heartbeat ACK timeout"))
			return
		}
		if !pending && (sent.IsZero() || elapsed >= interval) {
			n.cryptoMu.Lock()
			err := n.sendHeartbeat()
			n.cryptoMu.Unlock()
			if err != nil {
				n.stop(err)
				return
			}
			n.heartbeatMu.Lock()
			pending, sent = n.heartbeatPending, n.heartbeatSent
			n.heartbeatMu.Unlock()
		}
		delay := time.Until(sent.Add(interval))
		if pending {
			delay = time.Until(sent.Add(n.v.config.HeartbeatTimeout))
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(max(0, delay))
	}
}
