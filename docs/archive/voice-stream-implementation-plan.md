# Discord Voice Stream Implementation Plan
# Opus PCM Byte Stream 송수신 기능 추가

## 개요 (Overview)

이 문서는 Chatanium FlexModule의 discord-v1 백엔드에 Discord 음성 채널을 통한 Opus/PCM 바이트 스트림 송수신 기능을 추가하기 위한 구현 계획을 담고 있습니다.

**목표**: discordgo의 VoiceConnection 기능을 참고하여 모듈과 런타임 간 양방향 음성 스트림을 gRPC를 통해 구현

**참고**:
- discordgo 원본: `discordgo/voice.go`
- 현재 프로토콜: `proto/discord-v1/`, `shared/discord-v1/`

---

## 1. 아키텍처 분석 (Architecture Analysis)

### 1.1 discordgo VoiceConnection 구조

discordgo의 음성 처리는 다음과 같은 구조로 동작합니다:

```
VoiceConnection
├─ OpusSend (chan []byte)     ← 모듈이 음성 데이터를 보낼 채널
├─ OpusRecv (chan *Packet)    ← 모듈이 음성 데이터를 받을 채널
├─ opusSender()                ← goroutine: OpusSend 채널 → UDP 전송
└─ opusReceiver()              ← goroutine: UDP 수신 → OpusRecv 채널
```

**핵심 동작 흐름**:

1. **송신 (Sending)**:
   - 모듈 → `OpusSend` 채널에 Opus 인코딩된 바이트 전송
   - `opusSender()` goroutine이 채널에서 데이터 수신
   - RTP 헤더 추가 + NaCl secretbox 암호화
   - UDP로 Discord 서버에 전송

2. **수신 (Receiving)**:
   - `opusReceiver()` goroutine이 UDP에서 데이터 수신
   - NaCl secretbox 복호화 + RTP 헤더 파싱
   - `Packet` 구조체로 변환
   - `OpusRecv` 채널로 모듈에 전달

### 1.2 현재 FlexModule 구조

```
Runtime (main.go)
    │
    ├─→ gRPC Helper Service (shared/discord-v1/runtime/)
    │   └─ DiscordHelper 구현 (SendMessage, GetGuild 등)
    │
    └─→ Module (test-module/)
        └─ Helper 클라이언트로 Runtime 호출
```

**현재 구조의 특징**:
- 단방향 요청/응답 패턴 (RPC)
- 짧은 생명주기의 트랜잭션
- 바이트 스트림 미지원

---

## 2. 설계 결정사항 (Design Decisions)

### 2.1 gRPC 스트리밍 방식 선택

Discord 음성 기능은 **양방향 실시간 스트림**이 필요하므로 다음 방식을 사용:

```protobuf
service VoiceStream {
    // 양방향 스트리밍: 모듈 ↔ 런타임 간 실시간 Opus 데이터 교환
    rpc VoiceOpusStream(stream OpusPacket) returns (stream OpusPacket);
}
```

**선택 이유**:
- ✅ 양방향 실시간 통신 지원
- ✅ 단일 연결로 송신/수신 모두 처리
- ✅ 백프레셔(backpressure) 자동 처리
- ✅ gRPC의 HTTP/2 멀티플렉싱 활용

**대안 (미선택)**:
- ❌ 단방향 스트림 2개: 연결 관리 복잡도 증가
- ❌ 서버 스트리밍만: 송신 전용, 수신 불가
- ❌ 폴링 방식: 실시간성 저하, 오버헤드 증가

### 2.2 연결 생명주기 관리

```
Module                          Runtime
  │                               │
  ├─→ VoiceConnect()             │  (음성 채널 연결 시작)
  │   ├─ guild_id                │
  │   ├─ channel_id              │
  │   └─ mute/deaf flags         │
  │                               ├─→ discordgo.ChannelVoiceJoin()
  │                               ├─ VoiceConnection 생성
  │   ←─ connection_id ─────────┤
  │                               │
  ├─→ VoiceOpusStream() ────────→│  (스트림 시작)
  │   ←────── stream ──────────→ │  (양방향 데이터)
  │                               │
  ├─→ VoiceDisconnect() ─────────│  (연결 종료)
  │   connection_id               │
  │                               └─→ VoiceConnection.Disconnect()
```

### 2.3 데이터 구조

**OpusPacket 메시지**:
```protobuf
message OpusPacket {
    string connection_id = 1;  // 연결 식별자
    bytes opus_data = 2;       // Opus 인코딩된 오디오 데이터
    uint32 ssrc = 3;           // 수신 시 발신자 식별 (선택)
    uint32 sequence = 4;       // 패킷 순서 (선택)
    uint32 timestamp = 5;      // RTP 타임스탬프 (선택)
}
```

---

## 3. 구현 계획 (Implementation Plan)

### Phase 1: Protocol Buffer 정의

**파일**: `proto/discord-v1/voice.proto` (신규 생성)

```protobuf
syntax = "proto3";
package discord_v1;
option go_package = "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1";

import "common.proto";

// 음성 스트림 서비스
service VoiceStream {
    // 음성 채널 연결
    rpc VoiceConnect(VoiceConnectRequest) returns (VoiceConnectResponse);

    // 양방향 Opus 스트림
    rpc VoiceOpusStream(stream OpusPacket) returns (stream OpusPacket);

    // 음성 채널 연결 해제
    rpc VoiceDisconnect(VoiceDisconnectRequest) returns (common.Empty);

    // Speaking 상태 변경
    rpc VoiceSpeaking(VoiceSpeakingRequest) returns (common.Empty);
}

// 연결 요청
message VoiceConnectRequest {
    string guild_id = 1;
    string channel_id = 2;
    bool mute = 3;
    bool deaf = 4;
}

// 연결 응답
message VoiceConnectResponse {
    string connection_id = 1;  // 연결 세션 ID
    bool ready = 2;            // 연결 준비 상태
}

// Opus 패킷
message OpusPacket {
    string connection_id = 1;  // 어떤 음성 연결인지 식별
    bytes opus_data = 2;       // Opus 인코딩된 데이터

    // 수신 패킷에만 포함되는 메타데이터
    uint32 ssrc = 3;           // 발신자 SSRC
    uint32 sequence = 4;       // 패킷 순서
    uint32 timestamp = 5;      // RTP 타임스탬프
    bytes type = 6;            // 패킷 타입 (추가 정보)
}

// 연결 해제 요청
message VoiceDisconnectRequest {
    string connection_id = 1;
}

// Speaking 상태 요청
message VoiceSpeakingRequest {
    string connection_id = 1;
    bool speaking = 2;
}
```

**작업 항목**:
- [ ] `proto/discord-v1/voice.proto` 파일 생성
- [ ] `make buf` 실행하여 gRPC 코드 생성
- [ ] 생성된 `voice.pb.go`, `voice_grpc.pb.go` 확인

---

### Phase 2: Runtime 구현

**파일**: `shared/discord-v1/runtime/voice_helper.go` (신규 생성)

#### 2.1 VoiceHelper 구조체

```go
package runtime

import (
    "context"
    "sync"

    "github.com/bwmarrin/discordgo"
    pb "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
)

type VoiceHelper struct {
    pb.UnimplementedVoiceStreamServer

    dgSession *discordgo.Session

    // 연결 관리
    mu          sync.RWMutex
    connections map[string]*VoiceConnectionState  // connection_id -> state
}

type VoiceConnectionState struct {
    connectionID string
    vc           *discordgo.VoiceConnection

    // 스트림 관리
    streamMu     sync.RWMutex
    clientStream pb.VoiceStream_VoiceOpusStreamServer

    // 채널
    sendChan     chan []byte          // 모듈 → Discord
    recvChan     chan *discordgo.Packet  // Discord → 모듈

    // 종료 관리
    ctx          context.Context
    cancel       context.CancelFunc
}

func NewVoiceHelper(session *discordgo.Session) *VoiceHelper {
    return &VoiceHelper{
        dgSession:   session,
        connections: make(map[string]*VoiceConnectionState),
    }
}
```

#### 2.2 VoiceConnect 구현

```go
func (h *VoiceHelper) VoiceConnect(ctx context.Context, req *pb.VoiceConnectRequest) (*pb.VoiceConnectResponse, error) {
    // 1. discordgo VoiceConnection 생성
    vc, err := h.dgSession.ChannelVoiceJoin(
        req.GuildId,
        req.ChannelId,
        req.Mute,
        req.Deaf,
    )
    if err != nil {
        return nil, err
    }

    // 2. 연결 ID 생성
    connectionID := generateConnectionID()

    // 3. 연결 상태 생성
    ctx, cancel := context.WithCancel(context.Background())
    state := &VoiceConnectionState{
        connectionID: connectionID,
        vc:           vc,
        sendChan:     make(chan []byte, 100),
        recvChan:     make(chan *discordgo.Packet, 100),
        ctx:          ctx,
        cancel:       cancel,
    }

    // 4. 채널 연결
    vc.OpusSend = state.sendChan
    vc.OpusRecv = state.recvChan

    // 5. 수신 goroutine 시작 (Discord → recvChan)
    go h.receiveLoop(state)

    // 6. 연결 저장
    h.mu.Lock()
    h.connections[connectionID] = state
    h.mu.Unlock()

    return &pb.VoiceConnectResponse{
        ConnectionId: connectionID,
        Ready:        vc.Ready,
    }, nil
}

func generateConnectionID() string {
    return fmt.Sprintf("voice_%d", time.Now().UnixNano())
}
```

#### 2.3 VoiceOpusStream 구현 (양방향 스트림)

```go
func (h *VoiceHelper) VoiceOpusStream(stream pb.VoiceStream_VoiceOpusStreamServer) error {
    var state *VoiceConnectionState
    var connectionID string

    // 1. 첫 패킷에서 connection_id 획득
    firstPacket, err := stream.Recv()
    if err != nil {
        return err
    }
    connectionID = firstPacket.ConnectionId

    // 2. 연결 상태 찾기
    h.mu.RLock()
    state, exists := h.connections[connectionID]
    h.mu.RUnlock()
    if !exists {
        return fmt.Errorf("connection not found: %s", connectionID)
    }

    // 3. 스트림 저장
    state.streamMu.Lock()
    state.clientStream = stream
    state.streamMu.Unlock()

    // 4. 송신 goroutine 시작 (모듈 → Discord)
    errChan := make(chan error, 2)
    go h.sendLoop(state, stream, errChan)

    // 5. 첫 패킷 처리
    if len(firstPacket.OpusData) > 0 {
        select {
        case state.sendChan <- firstPacket.OpusData:
        case <-state.ctx.Done():
            return state.ctx.Err()
        }
    }

    // 6. 수신 루프 (모듈로부터 데이터 수신 → Discord 전송)
    go func() {
        for {
            packet, err := stream.Recv()
            if err != nil {
                errChan <- err
                return
            }

            select {
            case state.sendChan <- packet.OpusData:
            case <-state.ctx.Done():
                errChan <- state.ctx.Err()
                return
            }
        }
    }()

    // 7. 에러 대기
    return <-errChan
}

// sendLoop: recvChan (Discord 수신) → gRPC stream (모듈 전송)
func (h *VoiceHelper) sendLoop(state *VoiceConnectionState, stream pb.VoiceStream_VoiceOpusStreamServer, errChan chan error) {
    for {
        select {
        case packet := <-state.recvChan:
            err := stream.Send(&pb.OpusPacket{
                ConnectionId: state.connectionID,
                OpusData:     packet.Opus,
                Ssrc:         packet.SSRC,
                Sequence:     uint32(packet.Sequence),
                Timestamp:    packet.Timestamp,
                Type:         packet.Type,
            })
            if err != nil {
                errChan <- err
                return
            }

        case <-state.ctx.Done():
            errChan <- state.ctx.Err()
            return
        }
    }
}

// receiveLoop: Discord UDP 수신 → recvChan
func (h *VoiceHelper) receiveLoop(state *VoiceConnectionState) {
    // discordgo의 opusReceiver가 이미 recvChan에 데이터를 넣어줌
    // 추가 처리 불필요
}
```

#### 2.4 VoiceDisconnect 구현

```go
func (h *VoiceHelper) VoiceDisconnect(ctx context.Context, req *pb.VoiceDisconnectRequest) (*pb.Empty, error) {
    h.mu.Lock()
    state, exists := h.connections[req.ConnectionId]
    if exists {
        delete(h.connections, req.ConnectionId)
    }
    h.mu.Unlock()

    if !exists {
        return nil, fmt.Errorf("connection not found: %s", req.ConnectionId)
    }

    // 1. Context 취소
    state.cancel()

    // 2. VoiceConnection 종료
    if state.vc != nil {
        state.vc.Disconnect()
    }

    // 3. 채널 닫기
    close(state.sendChan)
    close(state.recvChan)

    return &pb.Empty{}, nil
}
```

#### 2.5 VoiceSpeaking 구현

```go
func (h *VoiceHelper) VoiceSpeaking(ctx context.Context, req *pb.VoiceSpeakingRequest) (*pb.Empty, error) {
    h.mu.RLock()
    state, exists := h.connections[req.ConnectionId]
    h.mu.RUnlock()

    if !exists {
        return nil, fmt.Errorf("connection not found: %s", req.ConnectionId)
    }

    err := state.vc.Speaking(req.Speaking)
    if err != nil {
        return nil, err
    }

    return &pb.Empty{}, nil
}
```

**작업 항목**:
- [ ] `shared/discord-v1/runtime/voice_helper.go` 파일 생성
- [ ] VoiceHelper 구조체 구현
- [ ] 4개 RPC 메서드 구현
- [ ] 에러 처리 및 로깅 추가
- [ ] 연결 타임아웃 처리

---

### Phase 3: Module 클라이언트 구현

**파일**: `shared/discord-v1/module/voice_client.go` (신규 생성)

```go
package module

import (
    "context"
    "io"
    "sync"

    pb "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
)

type VoiceClient struct {
    client pb.VoiceStreamClient

    mu            sync.RWMutex
    connectionID  string
    stream        pb.VoiceStream_VoiceOpusStreamClient

    // Public channels for module usage
    SendChan      chan []byte           // Module writes to this
    RecvChan      chan *OpusPacket      // Module reads from this

    ctx           context.Context
    cancel        context.CancelFunc
}

type OpusPacket struct {
    Data      []byte
    SSRC      uint32
    Sequence  uint32
    Timestamp uint32
}

func NewVoiceClient(client pb.VoiceStreamClient) *VoiceClient {
    return &VoiceClient{
        client:   client,
        SendChan: make(chan []byte, 100),
        RecvChan: make(chan *OpusPacket, 100),
    }
}

// Connect to voice channel
func (vc *VoiceClient) Connect(ctx context.Context, guildID, channelID string, mute, deaf bool) error {
    // 1. VoiceConnect RPC
    resp, err := vc.client.VoiceConnect(ctx, &pb.VoiceConnectRequest{
        GuildId:   guildID,
        ChannelId: channelID,
        Mute:      mute,
        Deaf:      deaf,
    })
    if err != nil {
        return err
    }

    vc.mu.Lock()
    vc.connectionID = resp.ConnectionId
    vc.mu.Unlock()

    // 2. Start stream
    return vc.startStream(ctx)
}

func (vc *VoiceClient) startStream(ctx context.Context) error {
    vc.ctx, vc.cancel = context.WithCancel(ctx)

    // 1. Open bidirectional stream
    stream, err := vc.client.VoiceOpusStream(vc.ctx)
    if err != nil {
        return err
    }

    vc.mu.Lock()
    vc.stream = stream
    vc.mu.Unlock()

    // 2. Send initial packet with connection_id
    err = stream.Send(&pb.OpusPacket{
        ConnectionId: vc.connectionID,
        OpusData:     []byte{}, // Empty initial packet
    })
    if err != nil {
        return err
    }

    // 3. Start send loop (SendChan → stream)
    go vc.sendLoop()

    // 4. Start receive loop (stream → RecvChan)
    go vc.receiveLoop()

    return nil
}

func (vc *VoiceClient) sendLoop() {
    for {
        select {
        case data := <-vc.SendChan:
            err := vc.stream.Send(&pb.OpusPacket{
                ConnectionId: vc.connectionID,
                OpusData:     data,
            })
            if err != nil {
                // Handle error (log, retry, close)
                return
            }

        case <-vc.ctx.Done():
            return
        }
    }
}

func (vc *VoiceClient) receiveLoop() {
    for {
        packet, err := vc.stream.Recv()
        if err == io.EOF {
            return
        }
        if err != nil {
            // Handle error
            return
        }

        select {
        case vc.RecvChan <- &OpusPacket{
            Data:      packet.OpusData,
            SSRC:      packet.Ssrc,
            Sequence:  packet.Sequence,
            Timestamp: packet.Timestamp,
        }:
        case <-vc.ctx.Done():
            return
        }
    }
}

// Disconnect from voice channel
func (vc *VoiceClient) Disconnect() error {
    vc.cancel()

    _, err := vc.client.VoiceDisconnect(context.Background(), &pb.VoiceDisconnectRequest{
        ConnectionId: vc.connectionID,
    })

    close(vc.SendChan)
    close(vc.RecvChan)

    return err
}

// Set speaking state
func (vc *VoiceClient) Speaking(speaking bool) error {
    _, err := vc.client.VoiceSpeaking(context.Background(), &pb.VoiceSpeakingRequest{
        ConnectionId: vc.connectionID,
        Speaking:     speaking,
    })
    return err
}
```

**작업 항목**:
- [ ] `shared/discord-v1/module/voice_client.go` 파일 생성
- [ ] VoiceClient 구조체 구현
- [ ] Connect/Disconnect 메서드 구현
- [ ] 양방향 스트림 루프 구현
- [ ] 에러 처리 및 재연결 로직

---

### Phase 4: 테스트 모듈 예제

**파일**: `test-module/voice_example.go` (신규 생성)

```go
package main

import (
    "context"
    "log"
    "time"

    discord "github.com/thirdscam/chatanium-flexmodule/shared/discord-v1/module"
)

// Example: Voice playback module
func voicePlaybackExample(voiceClient *discord.VoiceClient) {
    ctx := context.Background()

    // 1. Connect to voice channel
    err := voiceClient.Connect(ctx, "GUILD_ID", "CHANNEL_ID", false, false)
    if err != nil {
        log.Fatal("Failed to connect:", err)
    }
    defer voiceClient.Disconnect()

    // 2. Set speaking state
    voiceClient.Speaking(true)

    // 3. Send Opus data (from file, microphone, etc.)
    opusData := loadOpusFile("audio.opus")

    for _, frame := range opusData {
        select {
        case voiceClient.SendChan <- frame:
            time.Sleep(20 * time.Millisecond) // 20ms per frame (standard)
        case <-ctx.Done():
            return
        }
    }

    voiceClient.Speaking(false)
}

// Example: Voice recording module
func voiceRecordingExample(voiceClient *discord.VoiceClient) {
    ctx := context.Background()

    // 1. Connect to voice channel
    err := voiceClient.Connect(ctx, "GUILD_ID", "CHANNEL_ID", false, false)
    if err != nil {
        log.Fatal("Failed to connect:", err)
    }
    defer voiceClient.Disconnect()

    // 2. Receive Opus data
    for {
        select {
        case packet := <-voiceClient.RecvChan:
            // Process received Opus data
            log.Printf("Received packet from SSRC %d: %d bytes",
                packet.SSRC, len(packet.Data))

            // Save to file, decode to PCM, etc.
            saveOpusPacket(packet)

        case <-ctx.Done():
            return
        }
    }
}

func loadOpusFile(filename string) [][]byte {
    // TODO: Implement Opus file reading
    return nil
}

func saveOpusPacket(packet *discord.OpusPacket) {
    // TODO: Implement Opus packet saving
}
```

**작업 항목**:
- [ ] `test-module/voice_example.go` 파일 생성
- [ ] 송신 예제 구현
- [ ] 수신 예제 구현
- [ ] Opus 파일 로더 추가 (선택)

---

### Phase 5: 통합 및 등록

#### 5.1 Runtime 플러그인 등록

**파일**: `shared/discord-v1/runtime/plugin.go` 수정

```go
func (p *GRPCServer) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
    // 기존 Helper 등록
    pb.RegisterHelperServer(s, p.helper)

    // 새로운 VoiceStream 서버 등록
    voiceHelper := NewVoiceHelper(p.helper.dgSession)
    pb.RegisterVoiceStreamServer(s, voiceHelper)

    return nil
}
```

#### 5.2 Module 클라이언트 획득

**파일**: `shared/discord-v1/module/plugin.go` 수정

```go
type DiscordClients struct {
    Helper      pb.HelperClient
    VoiceStream pb.VoiceStreamClient  // 추가
}

func (p *GRPCClient) GetClients(broker *plugin.GRPCBroker, client *grpc.ClientConn) (*DiscordClients, error) {
    return &DiscordClients{
        Helper:      pb.NewHelperClient(client),
        VoiceStream: pb.NewVoiceStreamClient(client),  // 추가
    }, nil
}
```

**작업 항목**:
- [ ] `runtime/plugin.go`에 VoiceStream 서버 등록
- [ ] `module/plugin.go`에 VoiceStream 클라이언트 추가
- [ ] 모듈 인터페이스에 VoiceClient 전달 방법 추가

---

## 4. 테스트 계획 (Testing Plan)

### 4.1 단위 테스트

**파일**: `shared/discord-v1/runtime/voice_helper_test.go`

```go
func TestVoiceConnect(t *testing.T) {
    // Mock discordgo.Session
    // Test VoiceConnect RPC
}

func TestVoiceOpusStream(t *testing.T) {
    // Test bidirectional stream
    // Verify send/receive loops
}

func TestVoiceDisconnect(t *testing.T) {
    // Test cleanup and resource release
}
```

### 4.2 통합 테스트

**파일**: `test-module/voice_integration_test.go`

```go
func TestVoicePlayback(t *testing.T) {
    // 실제 Discord 봇으로 연결
    // 음성 채널에 조인
    // Opus 데이터 전송
    // 정상 종료 확인
}

func TestVoiceRecording(t *testing.T) {
    // 음성 채널 조인
    // 다른 사용자 음성 수신
    // 패킷 검증
}
```

### 4.3 성능 테스트

- **지연시간 측정**: 모듈 → 런타임 → Discord 전체 경로
- **처리량 테스트**: 초당 패킷 수 (50pps = 1초 오디오)
- **메모리 사용량**: 장시간 연결 시 메모리 누수 확인

---

## 5. 마일스톤 및 일정 (Milestones)

### Week 1: Protocol Definition
- [ ] `voice.proto` 작성 및 생성
- [ ] 인터페이스 설계 리뷰

### Week 2: Runtime Implementation
- [ ] VoiceHelper 구현
- [ ] 연결 관리 로직
- [ ] 스트림 처리 로직

### Week 3: Module Client Implementation
- [ ] VoiceClient 구현
- [ ] 송수신 루프 구현
- [ ] 에러 처리

### Week 4: Integration & Testing
- [ ] 플러그인 시스템 통합
- [ ] 테스트 모듈 작성
- [ ] 통합 테스트 수행

### Week 5: Documentation & Optimization
- [ ] 사용 가이드 작성
- [ ] 성능 최적화
- [ ] 코드 리뷰 및 리팩토링

---

## 6. 위험 요소 및 완화 방안 (Risks & Mitigation)

### 6.1 성능 문제

**위험**: gRPC 오버헤드로 인한 지연 증가

**완화**:
- HTTP/2 멀티플렉싱 활용
- 버퍼 크기 최적화 (현재 100으로 설정)
- Protobuf 메시지 크기 최소화

### 6.2 연결 안정성

**위험**: 네트워크 장애 시 연결 끊김

**완화**:
- Context 기반 취소 처리
- 재연결 로직 구현 (향후 추가)
- 타임아웃 설정

### 6.3 동시성 문제

**위험**: 여러 모듈이 동시에 음성 연결 시도

**완화**:
- 연결 맵에 mutex 사용
- 각 연결마다 독립적인 goroutine
- 채널 기반 동기화

### 6.4 리소스 누수

**위험**: 연결 종료 시 goroutine 정리 실패

**완화**:
- Context 기반 생명주기 관리
- defer를 통한 cleanup 보장
- 채널 close 명시적 호출

---

## 7. 향후 개선사항 (Future Enhancements)

### 7.1 PCM 직접 지원
- Opus 인코딩/디코딩을 런타임에서 처리
- 모듈은 raw PCM만 전송/수신

### 7.2 다중 연결 지원
- 하나의 모듈이 여러 음성 채널 동시 연결
- 연결별 독립적인 스트림 관리

### 7.3 재연결 로직
- 네트워크 장애 시 자동 재연결
- 버퍼링을 통한 패킷 손실 최소화

### 7.4 통계 및 모니터링
- 패킷 손실률 추적
- 지연시간 측정
- 대역폭 사용량 모니터링

---

## 8. 참고 자료 (References)

### discordgo 원본 코드
- `discordgo/voice.go`: VoiceConnection 구조체 및 송수신 로직
  - `opusSender()`: line 703-796
  - `opusReceiver()`: line 811-889
  - `VoiceConnection` struct: line 30-69

### gRPC 스트리밍
- [gRPC Bidirectional Streaming](https://grpc.io/docs/languages/go/basics/#bidirectional-streaming-rpc)
- [gRPC Performance Best Practices](https://grpc.io/docs/guides/performance/)

### Discord Voice Protocol
- [Discord Voice Gateway](https://discord.com/developers/docs/topics/voice-connections)
- RTP (Real-time Transport Protocol) over UDP
- Opus audio codec (48kHz, 20ms frames)

---

## 9. 체크리스트 (Implementation Checklist)

### Protocol Definition
- [ ] `proto/discord-v1/voice.proto` 생성
- [ ] VoiceStream service 정의
- [ ] OpusPacket 메시지 정의
- [ ] `make buf` 실행 및 코드 생성 확인

### Runtime Implementation
- [ ] `shared/discord-v1/runtime/voice_helper.go` 생성
- [ ] VoiceHelper 구조체 구현
- [ ] VoiceConnect 메서드 구현
- [ ] VoiceOpusStream 메서드 구현 (양방향)
- [ ] VoiceDisconnect 메서드 구현
- [ ] VoiceSpeaking 메서드 구현
- [ ] 연결 상태 관리 로직
- [ ] 에러 처리 및 로깅

### Module Client Implementation
- [ ] `shared/discord-v1/module/voice_client.go` 생성
- [ ] VoiceClient 구조체 구현
- [ ] Connect 메서드 구현
- [ ] startStream 메서드 구현
- [ ] sendLoop, receiveLoop 구현
- [ ] Disconnect 메서드 구현
- [ ] Speaking 메서드 구현

### Integration
- [ ] `runtime/plugin.go`에 VoiceStream 서버 등록
- [ ] `module/plugin.go`에 VoiceStream 클라이언트 추가
- [ ] 모듈 초기화 시 VoiceClient 전달

### Testing
- [ ] 단위 테스트 작성
- [ ] 통합 테스트 작성
- [ ] 테스트 모듈 예제 작성
- [ ] 성능 테스트 수행

### Documentation
- [ ] 사용 가이드 작성
- [ ] API 문서 업데이트
- [ ] 예제 코드 추가
- [ ] CLAUDE.md 업데이트

---

## 10. 결론 (Conclusion)

이 구현 계획은 discordgo의 VoiceConnection 기능을 FlexModule의 gRPC 기반 아키텍처에 통합하기 위한 체계적인 접근을 제시합니다.

**핵심 설계 원칙**:
1. **양방향 스트리밍**: gRPC bidirectional stream을 사용한 실시간 통신
2. **연결 생명주기 관리**: Context 기반의 안전한 리소스 관리
3. **채널 기반 동기화**: discordgo의 채널 패턴을 gRPC 스트림으로 연결
4. **확장 가능한 구조**: 향후 PCM 지원, 다중 연결 등 추가 가능

이 계획을 단계별로 실행하면 모듈과 런타임 간 안정적이고 효율적인 음성 스트림 통신을 구현할 수 있습니다.
