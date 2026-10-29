//go:build voice && cgo

package discordgo

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"net"
	"time"
)

func (n *nativeVoice) discoverUDP(ip string, port int) error {
	address := net.ParseIP(ip)
	if address == nil || port <= 0 || port > 65535 {
		return errors.New("invalid voice UDP endpoint")
	}
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: address, Port: port})
	if err != nil {
		return errors.New("voice UDP dial failed")
	}
	n.mu.Lock()
	if n.failure != nil {
		n.mu.Unlock()
		conn.Close()
		return n.err()
	}
	n.udp = conn
	n.mu.Unlock()
	if err = conn.SetDeadline(time.Now().Add(n.v.config.UDPDiscoveryTimeout)); err != nil {
		return err
	}
	var packet [74]byte
	binary.BigEndian.PutUint16(packet[:2], 1)
	binary.BigEndian.PutUint16(packet[2:4], 70)
	binary.BigEndian.PutUint32(packet[4:8], n.ssrc)
	if count, err := conn.Write(packet[:]); err != nil || count != len(packet) {
		return errors.New("voice UDP discovery write failed")
	}
	var response [75]byte
	size, _, flags, _, err := conn.ReadMsgUDP(response[:], nil)
	if err != nil || flags != 0 || size != 74 {
		return errors.New("voice UDP discovery response failed")
	}
	if binary.BigEndian.Uint16(response[:2]) != 2 || binary.BigEndian.Uint16(response[2:4]) != 70 || binary.BigEndian.Uint32(response[4:8]) != n.ssrc {
		return errors.New("invalid voice UDP discovery response")
	}
	end := 8
	for end < 72 && response[end] != 0 {
		end++
	}
	if end == 72 || end == 8 {
		return errors.New("invalid voice discovery address")
	}
	discovered := string(response[8:end])
	if net.ParseIP(discovered) == nil {
		return errors.New("invalid voice discovery IP")
	}
	discoveredPort := binary.BigEndian.Uint16(response[72:74])
	if discoveredPort == 0 {
		return errors.New("invalid voice discovery port")
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		return err
	}
	var seed [6]byte
	if _, err = rand.Read(seed[:]); err != nil {
		return errors.New("voice RTP initialization failed")
	}
	n.rtpSequence = binary.BigEndian.Uint16(seed[:2])
	n.rtpTimestamp = binary.BigEndian.Uint32(seed[2:])
	selectData := struct {
		Protocol string `json:"protocol"`
		Data     struct {
			Address string `json:"address"`
			Port    uint16 `json:"port"`
			Mode    string `json:"mode"`
		} `json:"data"`
	}{Protocol: "udp"}
	selectData.Data.Address = discovered
	selectData.Data.Port = discoveredPort
	selectData.Data.Mode = n.mode
	if err = n.writeJSON(n.ctx, 1, selectData); err != nil {
		return err
	}
	n.wg.Add(2)
	go n.receiverLoop()
	go n.keepAliveLoop()
	return nil
}

func (n *nativeVoice) writeUDP(ctx context.Context, p []byte) error {
	// Both media and NAT keepalive use the same deadline/writer admission gate.
	select {
	case n.udpWriter <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-n.ctx.Done():
		return n.err()
	}
	defer func() { <-n.udpWriter }()
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(n.v.config.WriteTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := n.udp.SetWriteDeadline(deadline); err != nil {
		return err
	}
	count, err := n.udp.Write(p)
	if err != nil {
		return errors.New("voice UDP write failed")
	}
	if count != len(p) {
		return errors.New("short voice UDP write")
	}
	return nil
}
func (n *nativeVoice) keepAliveLoop() {
	defer n.wg.Done()
	timer := time.NewTicker(n.v.config.UDPKeepAliveInterval)
	defer timer.Stop()
	var seq uint64
	var b [8]byte
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-timer.C:
			if seq == math.MaxUint64 {
				n.stop(errors.New("voice UDP keepalive counter exhausted"))
				return
			}
			binary.LittleEndian.PutUint64(b[:], seq)
			seq++
			if err := n.writeUDP(n.ctx, b[:]); err != nil {
				n.stop(err)
				return
			}
		}
	}
}
func (n *nativeVoice) speaking(ctx context.Context, active bool) error {
	mask := 0
	if active {
		mask = 1
	}
	return n.writeJSON(ctx, 5, struct {
		Speaking int    `json:"speaking"`
		Delay    int    `json:"delay"`
		SSRC     uint32 `json:"ssrc"`
	}{mask, 0, n.ssrc})
}
func (n *nativeVoice) senderLoop() {
	defer n.wg.Done()
	packetStorage := make([]byte, n.v.config.MaxUDPPacketBytes)
	encrypted := packetStorage[12 : 12+n.v.config.MaxEncryptedFrameBytes]
	packet := packetStorage[:0]
	var lastWrite time.Time
	speaking := false
	idle := time.NewTimer(20 * time.Millisecond)
	if !idle.Stop() {
		<-idle.C
	}
	defer idle.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-idle.C:
			if speaking {
				if err := n.speaking(n.ctx, false); err != nil {
					n.stop(err)
					return
				}
				speaking = false
			}
		case request := <-n.handoffs:
			if err := request.ctx.Err(); err != nil {
				request.reply <- handoffResult{err: err}
				continue
			}
			if !n.ready() {
				err := n.err()
				if err == nil {
					err = ErrVoiceNotReady
				}
				request.reply <- handoffResult{err: err}
				continue
			}
			if err := request.ctx.Err(); err != nil {
				request.reply <- handoffResult{err: err}
				continue
			}
			submission := &OpusSubmission{done: make(chan OpusWriteResult, 1)}
			request.reply <- handoffResult{submission: submission}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			// The packet is now non-retractably owned. Only the connection lifetime and
			// receipt deadline apply; canceling the submission call cannot replay it.
			receiptCtx, cancel := context.WithTimeout(n.ctx, n.v.config.ReceiptTimeout)
			result := OpusWriteResult{}
			if !lastWrite.IsZero() {
				wait := time.Until(lastWrite.Add(20 * time.Millisecond))
				if wait > 0 {
					timer := time.NewTimer(wait)
					select {
					case <-timer.C:
					case <-receiptCtx.Done():
						result.Err = receiptCtx.Err()
					}
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
				}
			}
			if result.Err == nil && !speaking {
				result.Err = n.speaking(receiptCtx, true)
				speaking = result.Err == nil
			}
			if result.Err == nil {
				if err := n.cryptoMu.lockContext(receiptCtx); err != nil {
					result.Err = err
				} else {
					if n.aead == nil || !n.dave.Ready() {
						result.Err = ErrVoiceNotReady
					} else {
						maxSize, err := n.dave.MaxCiphertextSize(len(request.opus))
						if err != nil || maxSize > len(encrypted) {
							result.Err = errors.New("DAVE encrypted frame exceeds bound")
						} else {
							count, err := n.dave.EncryptOpus(receiptCtx, n.ssrc, request.opus, encrypted)
							if err != nil {
								result.Err = err
							} else if count <= 0 || count > len(encrypted) {
								result.Err = errors.New("invalid DAVE encrypted frame length")
							} else {
								packet, result.Err = sealVoiceRTP(packet[:0], n.aead, n.ssrc, n.rtpSequence, n.rtpTimestamp, n.nonce, encrypted[:count])
								n.nonce++
								n.rtpSequence++
								n.rtpTimestamp += voiceFrameIntervalSamples
							}
						}
					}
					n.cryptoMu.Unlock()
				}
			}
			if result.Err == nil {
				result.Err = n.writeUDP(receiptCtx, packet)
				if result.Err == nil {
					lastWrite = time.Now()
					result.WrittenAt = lastWrite
				}
			}
			cancel()
			n.outstanding <- struct{}{}
			submission.done <- result
			close(submission.done)
			if result.Err != nil {
				n.stop(result.Err)
				return
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(20 * time.Millisecond)
		}
	}
}
func (n *nativeVoice) receiverLoop() {
	defer n.wg.Done()
	packet := make([]byte, n.v.config.MaxUDPPacketBytes+1)
	transportPlain := make([]byte, n.v.config.MaxUDPPacketBytes)
	opus := make([]byte, n.v.config.MaxEncryptedFrameBytes)
	for {
		size, _, flags, _, err := n.udp.ReadMsgUDP(packet, nil)
		if err != nil {
			if n.ctx.Err() == nil {
				n.stop(errors.New("voice UDP receive failed"))
			}
			return
		}
		if flags != 0 || size > n.v.config.MaxUDPPacketBytes || size < 12 {
			continue
		}
		received := time.Now()
		n.cryptoMu.Lock()
		if n.aead == nil {
			n.cryptoMu.Unlock()
			continue
		}
		frame, ssrc, seq, timestamp, err := openVoiceRTP(transportPlain, n.aead, packet[:size])
		if err != nil {
			n.cryptoMu.Unlock()
			continue
		}
		sender, known := n.senders[ssrc]
		_, recognized := n.participants[sender]
		if !known || !recognized || len(frame) > n.v.config.MaxEncryptedFrameBytes {
			n.cryptoMu.Unlock()
			continue
		}
		maxSize, err := n.dave.MaxPlaintextSize(sender, len(frame))
		if err != nil || maxSize > len(opus) {
			n.cryptoMu.Unlock()
			continue
		}
		count, err := n.dave.DecryptOpus(n.ctx, sender, frame, opus)
		n.cryptoMu.Unlock()
		if err != nil || count <= 0 || count > len(opus) {
			continue
		}
		if err := validateOpusPacket(opus[:count], n.v.config.MaxOpusBytes, 0); err != nil {
			continue
		}
		p := &Packet{SSRC: ssrc, Sequence: seq, Timestamp: timestamp, Opus: append([]byte(nil), opus[:count]...), ReceivedAt: received}
		n.queueMu.Lock()
		if n.queueBytes > n.v.config.ReceiveQueueBytes-len(p.Opus) {
			n.queueMu.Unlock()
			n.stop(errors.New("voice receive byte queue exhausted"))
			return
		}
		n.queueBytes += len(p.Opus)
		select {
		case n.packets <- p:
			n.queueMu.Unlock()
		case <-n.ctx.Done():
			n.queueBytes -= len(p.Opus)
			n.queueMu.Unlock()
			return
		default:
			n.queueBytes -= len(p.Opus)
			n.queueMu.Unlock()
			n.stop(errors.New("voice receive packet queue exhausted"))
			return
		}
	}
}
