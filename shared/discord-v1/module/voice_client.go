package module

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	pb "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
	"google.golang.org/grpc/metadata"
)

// VoiceClient provides a client interface for voice streaming
type VoiceClient struct {
	client   pb.VoiceStreamClient
	moduleID string

	mu           sync.RWMutex
	connectionID string
	stream       pb.VoiceStream_VoiceStreamClient

	// Public channels for module usage
	SendChan chan *VoiceSendRequest
	RecvChan chan *VoicePacket

	ctx    context.Context
	cancel context.CancelFunc

	// Statistics
	statsMu       sync.RWMutex
	packetsSent   uint64
	packetsRecv   uint64
	bytesSent     uint64
	bytesRecv     uint64
}

// VoiceSendRequest represents a voice packet to send
type VoiceSendRequest struct {
	OpusData  []byte
	Priority  int32
	TimeoutMs int64
}

// VoicePacket represents a received voice packet
type VoicePacket struct {
	OpusData  []byte
	SSRC      uint32
	Sequence  uint32
	Timestamp uint32
}

// NewVoiceClient creates a new voice client
func NewVoiceClient(client pb.VoiceStreamClient, moduleID string) *VoiceClient {
	return &VoiceClient{
		client:   client,
		moduleID: moduleID,
		SendChan: make(chan *VoiceSendRequest, 100),
		RecvChan: make(chan *VoicePacket, 100),
	}
}

// Join joins a voice channel
func (vc *VoiceClient) Join(ctx context.Context, guildID, channelID string, mute, deaf bool) error {
	// Add module-id to context
	ctx = metadata.AppendToOutgoingContext(ctx, "module-id", vc.moduleID)

	// Join voice channel
	resp, err := vc.client.VoiceJoin(ctx, &pb.VoiceJoinRequest{
		GuildId:   guildID,
		ChannelId: channelID,
		Mute:      mute,
		Deaf:      deaf,
	})
	if err != nil {
		return fmt.Errorf("voice join failed: %w", err)
	}

	vc.mu.Lock()
	vc.connectionID = resp.ConnectionId
	vc.mu.Unlock()

	// Start stream
	return vc.startStream(ctx)
}

// Leave leaves the voice channel
func (vc *VoiceClient) Leave() error {
	vc.cancel()

	ctx := metadata.AppendToOutgoingContext(context.Background(), "module-id", vc.moduleID)

	_, err := vc.client.VoiceLeave(ctx, &pb.VoiceLeaveRequest{
		ConnectionId: vc.connectionID,
	})

	close(vc.SendChan)
	close(vc.RecvChan)

	return err
}

// Speaking sets the speaking state
func (vc *VoiceClient) Speaking(speaking bool) error {
	ctx := metadata.AppendToOutgoingContext(context.Background(), "module-id", vc.moduleID)

	_, err := vc.client.VoiceSpeaking(ctx, &pb.VoiceSpeakingRequest{
		ConnectionId: vc.connectionID,
		Speaking:     speaking,
	})

	return err
}

// Send is a convenience method to send opus data
func (vc *VoiceClient) Send(opusData []byte, priority int32, timeoutMs int64) {
	vc.SendChan <- &VoiceSendRequest{
		OpusData:  opusData,
		Priority:  priority,
		TimeoutMs: timeoutMs,
	}
}

// GetStats returns client statistics
func (vc *VoiceClient) GetStats() (packetsSent, packetsRecv, bytesSent, bytesRecv uint64) {
	vc.statsMu.RLock()
	defer vc.statsMu.RUnlock()
	return vc.packetsSent, vc.packetsRecv, vc.bytesSent, vc.bytesRecv
}

// startStream starts the bidirectional stream
func (vc *VoiceClient) startStream(ctx context.Context) error {
	vc.ctx, vc.cancel = context.WithCancel(ctx)

	// Add module-id to context
	streamCtx := metadata.AppendToOutgoingContext(vc.ctx, "module-id", vc.moduleID)

	// Open bidirectional stream
	stream, err := vc.client.VoiceStream(streamCtx)
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	vc.mu.Lock()
	vc.stream = stream
	vc.mu.Unlock()

	// Send initial packet with connection_id
	err = stream.Send(&pb.VoicePacket{
		ConnectionId: vc.connectionID,
		OpusData:     []byte{}, // Empty initial packet
	})
	if err != nil {
		return fmt.Errorf("failed to send initial packet: %w", err)
	}

	// Start send and receive loops
	go vc.sendLoop()
	go vc.receiveLoop()

	return nil
}

// sendLoop handles sending packets to the runtime
func (vc *VoiceClient) sendLoop() {
	for {
		select {
		case req := <-vc.SendChan:
			err := vc.stream.Send(&pb.VoicePacket{
				ConnectionId: vc.connectionID,
				OpusData:     req.OpusData,
				Priority:     req.Priority,
				TimeoutMs:    req.TimeoutMs,
			})
			if err != nil {
				if log != nil {
					log.Error("sendLoop: failed to send voice packet", "error", err)
				}
				return
			}

			vc.statsMu.Lock()
			vc.packetsSent++
			vc.bytesSent += uint64(len(req.OpusData))
			vc.statsMu.Unlock()

		case <-vc.ctx.Done():
			return
		}
	}
}

// receiveLoop handles receiving packets from the runtime
func (vc *VoiceClient) receiveLoop() {
	for {
		packet, err := vc.stream.Recv()
		if err == io.EOF {
			if log != nil {
				log.Debug("receiveLoop: stream closed (EOF)")
			}
			return
		}
		if err != nil {
			if log != nil {
				log.Error("receiveLoop: failed to receive voice packet", "error", err)
			}
			return
		}

		select {
		case vc.RecvChan <- &VoicePacket{
			OpusData:  packet.OpusData,
			SSRC:      packet.Ssrc,
			Sequence:  packet.Sequence,
			Timestamp: packet.Timestamp,
		}:
			vc.statsMu.Lock()
			vc.packetsRecv++
			vc.bytesRecv += uint64(len(packet.OpusData))
			vc.statsMu.Unlock()

		case <-vc.ctx.Done():
			return
		}
	}
}

// SendWithDefaults sends opus data with default settings (priority=0, timeout=5s)
func (vc *VoiceClient) SendWithDefaults(opusData []byte) {
	vc.Send(opusData, 0, 5000)
}

// WaitForReady waits until send/receive channels are ready (useful for testing)
func (vc *VoiceClient) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if vc.stream != nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for stream to be ready")
}
