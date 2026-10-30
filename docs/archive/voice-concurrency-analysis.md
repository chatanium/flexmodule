# Voice Stream 동시성 분석 및 개선안
## Concurrency Analysis for Multi-Module Voice Streaming

**분석 날짜**: 2025-02-11
**분석 대상**: `voice-stream-implementation-plan.md`의 다중 모듈 환경 동시성 안전성

---

## 🚨 발견된 치명적인 문제들

### 문제 1: Guild당 하나의 VoiceConnection 제약 ⚠️

**discordgo 아키텍처 제약사항**:
```go
// discordgo/structs.go:90-91
// Stores a mapping of guild id's to VoiceConnections
VoiceConnections map[string]*VoiceConnection
```

**ChannelVoiceJoin 동작** (discordgo/wsapi.go:704-717):
```go
func (s *Session) ChannelVoiceJoin(gID, cID string, mute, deaf bool) (*VoiceConnection, err error) {
    s.RLock()
    voice, _ = s.VoiceConnections[gID]  // ← Guild ID로 조회
    s.RUnlock()

    if voice == nil {
        voice = &VoiceConnection{}
        s.Lock()
        s.VoiceConnections[gID] = voice  // ← Guild당 하나만 저장
        s.Unlock()
    }

    // 기존 연결이 있으면 재사용하고 채널만 변경
    voice.Lock()
    voice.ChannelID = cID  // ← 채널 덮어쓰기
    voice.Unlock()
    // ...
}
```

**문제 시나리오**:
```
시간  | 모듈 A                         | 모듈 B                         | Session.VoiceConnections[guildX]
-----|--------------------------------|--------------------------------|----------------------------------
T1   | Connect(guildX, channelA)      |                                | → VoiceConnection(channelA)
T2   | OpusSend ← music data          |                                | ← 정상 작동
T3   |                                | Connect(guildX, channelB)      | → VoiceConnection(channelB) ⚠️
T4   | OpusSend ← music data          |                                | ← channelB로 전송됨! (의도와 다름)
T5   |                                | OpusSend ← voice data          | ← channelB로 전송 (정상)
```

**영향**:
- ❌ **모듈 A의 연결이 강제로 channelB로 이동**
- ❌ **모듈 A는 자신이 channelA에 연결되어 있다고 착각**
- ❌ **channelA에 음악을 재생하려 했으나 channelB에서 재생됨**
- ❌ **예측 불가능한 음성 채널 전환**

**현재 구현 계획의 문제**:
```go
// shared/discord-v1/runtime/voice_helper.go (계획안)
func (h *VoiceHelper) VoiceConnect(...) (*pb.VoiceConnectResponse, error) {
    // 1. 같은 dgSession 사용
    vc, err := h.dgSession.ChannelVoiceJoin(req.GuildId, req.ChannelId, ...)

    // 2. 각 모듈마다 독립적인 connection_id 부여
    connectionID := generateConnectionID()  // "voice_123", "voice_456"

    // 3. 별도 state로 저장
    h.connections[connectionID] = &VoiceConnectionState{
        vc: vc,  // ← 모든 모듈이 같은 Guild면 같은 vc를 가리킴!
        // ...
    }
}
```

**실제 동작**:
```
h.connections["voice_123"].vc → Session.VoiceConnections["guildX"]
                                       ↓
h.connections["voice_456"].vc → 같은 VoiceConnection 객체!
```

---

### 문제 2: OpusSend/OpusRecv 채널 공유 충돌 ⚠️

**VoiceConnection 구조** (discordgo/voice.go:30-69):
```go
type VoiceConnection struct {
    // ...
    OpusSend chan []byte          // ← 하나의 채널
    OpusRecv chan *Packet         // ← 하나의 채널
    // ...
}
```

**현재 계획의 채널 연결 방식**:
```go
// Phase 2 계획안
state := &VoiceConnectionState{
    vc:       vc,
    sendChan: make(chan []byte, 100),
    recvChan: make(chan *discordgo.Packet, 100),
}

// discordgo 채널 연결
vc.OpusSend = state.sendChan  // ← 덮어쓰기!
vc.OpusRecv = state.recvChan  // ← 덮어쓰기!
```

**문제 시나리오**:
```
모듈 A: Connect(guildX, channelA)
  → state_A.sendChan 생성
  → vc.OpusSend = state_A.sendChan

모듈 B: Connect(guildX, channelB)  (같은 Guild!)
  → state_B.sendChan 생성
  → vc.OpusSend = state_B.sendChan  ← 모듈 A의 채널 덮어씀!

모듈 A: SendChan ← opus_data
  → state_A.sendChan ← opus_data
  → BUT vc.OpusSend는 state_B.sendChan을 가리킴!
  → 모듈 A의 데이터가 전송되지 않음! ❌
```

**데이터 흐름 충돌**:
```
Module A              Module B              VoiceConnection
   │                     │                        │
   ├─ SendChan ←──────────┼─ SendChan              │
   │     │               │     │                  │
   │     ↓               │     ↓                  │
   │  state_A.sendChan   │  state_B.sendChan      │
   │     ❌ 버려짐!      │     │                  │
   │                     │     └─→ OpusSend ←──────┤
   │                     │                   opusSender()
   │                     │                        │
   │                     │                        ↓
   │                     │                     Discord
```

---

### 문제 3: 스트림 상태 Race Condition ⚠️

**현재 계획의 스트림 관리**:
```go
type VoiceConnectionState struct {
    // ...
    streamMu     sync.RWMutex
    clientStream pb.VoiceStream_VoiceOpusStreamServer
    // ...
}

func (h *VoiceHelper) VoiceOpusStream(stream pb.VoiceStream_VoiceOpusStreamServer) error {
    // 1. connection_id 획득
    firstPacket, err := stream.Recv()
    connectionID := firstPacket.ConnectionId

    // 2. 연결 찾기
    h.mu.RLock()
    state := h.connections[connectionID]
    h.mu.RUnlock()

    // 3. 스트림 저장
    state.streamMu.Lock()
    state.clientStream = stream  // ← 여기서 race condition!
    state.streamMu.Unlock()
}
```

**Race Condition 시나리오**:
```
Goroutine 1: VoiceOpusStream(stream1)
  │
  ├─ firstPacket.ConnectionId = "voice_123"
  ├─ state = connections["voice_123"]
  │
  ├─ state.streamMu.Lock()
  ├─ state.clientStream = stream1
  ├─ state.streamMu.Unlock()
  │
  └─ go sendLoop(state, stream1) ─┐
                                   │
Goroutine 2: VoiceOpusStream(stream2) (재연결 시도)
  │                                │
  ├─ firstPacket.ConnectionId = "voice_123"  (같은 ID!)
  ├─ state = connections["voice_123"]        (같은 state!)
  │                                │
  ├─ state.streamMu.Lock()         │
  ├─ state.clientStream = stream2  ← 덮어씀!
  ├─ state.streamMu.Unlock()       │
  │                                │
  └─ go sendLoop(state, stream2) ─┤
                                   │
                                   ↓
                        sendLoop(state, stream1):
                          stream.Send() ← stream1 사용 (유효)

                        sendLoop(state, stream2):
                          stream.Send() ← stream2 사용 (유효)

                        recvChan에서 패킷 수신:
                          state.clientStream.Send() ← stream2로만 전송!
                          stream1은 데이터를 받지 못함! ❌
```

**문제점**:
- ❌ 동일 connection_id로 여러 스트림 생성 가능
- ❌ 이전 스트림이 자동으로 닫히지 않음
- ❌ sendLoop이 두 개 동시 실행되어 데이터 중복 전송
- ❌ recvChan 데이터가 마지막 스트림으로만 전송

---

### 문제 4: 연결 해제 시 타이밍 이슈 ⚠️

**계획된 Disconnect 구현**:
```go
func (h *VoiceHelper) VoiceDisconnect(...) (*pb.Empty, error) {
    h.mu.Lock()
    state := h.connections[req.ConnectionId]
    delete(h.connections, req.ConnectionId)
    h.mu.Unlock()

    state.cancel()  // ← Context 취소
    state.vc.Disconnect()  // ← discordgo 연결 종료
    close(state.sendChan)
    close(state.recvChan)
}
```

**Race Condition with OpusStream**:
```
Thread 1: Disconnect("voice_123")        Thread 2: OpusStream receive loop
    │                                         │
    ├─ delete(connections["voice_123"])      │
    │                                         ├─ packet := stream.Recv()
    ├─ state.cancel()                         │
    │                                         ├─ state.sendChan ← packet.Data
    ├─ close(state.sendChan) ───────────────→ │ PANIC! send on closed channel! ❌
```

**문제점**:
- ❌ Disconnect와 스트림 goroutine 간 동기화 부재
- ❌ 채널 close 시점이 불명확 (goroutine 종료 전에 close 가능)
- ❌ Context cancel 후에도 goroutine이 즉시 종료되지 않을 수 있음

---

## 📊 동시성 문제 분류

### Level 1: 아키텍처 제약 (Critical) 🔴
- **문제 1**: Guild당 VoiceConnection 제약
- **영향**: 다중 모듈이 같은 Guild의 다른 채널에 연결 불가
- **해결**: 아키텍처 재설계 필수

### Level 2: 데이터 무결성 (High) 🟠
- **문제 2**: OpusSend/OpusRecv 채널 덮어쓰기
- **영향**: 음성 데이터 손실, 잘못된 채널 전송
- **해결**: 채널 멀티플렉싱 필요

### Level 3: Race Condition (Medium) 🟡
- **문제 3**: 스트림 상태 race condition
- **문제 4**: 연결 해제 타이밍 이슈
- **영향**: 패닉, 메모리 누수, 예측 불가능한 동작
- **해결**: 동기화 메커니즘 강화

---

## ✅ 해결 방안

### 방안 A: Session 분리 (권장) ⭐

**핵심 아이디어**: 각 모듈에게 독립적인 discordgo.Session 제공

```go
type VoiceHelper struct {
    pb.UnimplementedVoiceStreamServer

    // 하나의 마스터 Session (메시지 등 다른 기능용)
    masterSession *discordgo.Session

    // 각 모듈별 독립 Session
    mu               sync.RWMutex
    moduleSessions   map[string]*ModuleVoiceSession  // module_id -> session
}

type ModuleVoiceSession struct {
    moduleID     string
    dgSession    *discordgo.Session  // 독립 Session
    connections  map[string]*VoiceConnectionState  // connection_id -> state
}

func (h *VoiceHelper) VoiceConnect(ctx context.Context, req *pb.VoiceConnectRequest) (*pb.VoiceConnectResponse, error) {
    // 1. 모듈 ID 획득 (gRPC metadata에서)
    moduleID := getModuleIDFromContext(ctx)

    // 2. 모듈 전용 Session 생성 또는 재사용
    h.mu.Lock()
    moduleSession, exists := h.moduleSessions[moduleID]
    if !exists {
        // 새로운 Session 생성 (같은 봇 토큰 사용)
        dgSession, err := discordgo.New("Bot " + h.masterSession.Token)
        if err != nil {
            h.mu.Unlock()
            return nil, err
        }

        moduleSession = &ModuleVoiceSession{
            moduleID:    moduleID,
            dgSession:   dgSession,
            connections: make(map[string]*VoiceConnectionState),
        }
        h.moduleSessions[moduleID] = moduleSession
    }
    h.mu.Unlock()

    // 3. 모듈 전용 Session으로 음성 연결
    vc, err := moduleSession.dgSession.ChannelVoiceJoin(
        req.GuildId, req.ChannelId, req.Mute, req.Deaf,
    )
    if err != nil {
        return nil, err
    }

    // 4. 연결 상태 저장
    connectionID := generateConnectionID()
    state := &VoiceConnectionState{
        connectionID: connectionID,
        vc:           vc,
        sendChan:     make(chan []byte, 100),
        recvChan:     make(chan *discordgo.Packet, 100),
    }

    // 채널 연결 (이제 충돌 없음!)
    vc.OpusSend = state.sendChan
    vc.OpusRecv = state.recvChan

    moduleSession.connections[connectionID] = state

    return &pb.VoiceConnectResponse{
        ConnectionId: connectionID,
        Ready:        vc.Ready,
    }, nil
}
```

**장점**:
- ✅ 각 모듈이 독립적인 VoiceConnections 맵 소유
- ✅ Guild당 VoiceConnection 제약이 모듈별로 적용 (충돌 없음)
- ✅ OpusSend/OpusRecv 채널 충돌 해결
- ✅ 모듈 간 완전한 격리

**단점**:
- ⚠️ Session 개수 증가 (메모리 사용량 증가)
- ⚠️ Discord 연결 수 증가 (rate limit 고려 필요)

**적용 가능성**: ⭐⭐⭐⭐⭐ (가장 안전하고 명확한 해결책)

---

### 방안 B: 채널 멀티플렉싱 (복잡도 높음)

**핵심 아이디어**: 하나의 VoiceConnection을 여러 모듈이 공유하되, 데이터를 멀티플렉싱

```go
type VoiceConnectionState struct {
    connectionID string
    vc           *discordgo.VoiceConnection

    // 여러 모듈이 공유
    mu           sync.RWMutex
    subscribers  map[string]*Subscriber  // subscriber_id -> subscriber

    // 공유 채널
    sharedSend   chan []byte
    sharedRecv   chan *discordgo.Packet
}

type Subscriber struct {
    subscriberID string
    sendChan     chan []byte           // 모듈 → 공유
    recvChan     chan *discordgo.Packet  // 공유 → 모듈
}

// 믹서: 여러 모듈의 음성을 합성
func (s *VoiceConnectionState) mixer() {
    for {
        select {
        case data1 := <-subscriber1.sendChan:
        case data2 := <-subscriber2.sendChan:
            // Opus 프레임 믹싱 (매우 복잡!)
            mixed := mixOpusFrames(data1, data2)
            s.sharedSend <- mixed
        }
    }
}

// 브로드캐스터: 수신 음성을 모든 모듈에 전달
func (s *VoiceConnectionState) broadcaster() {
    for packet := range s.sharedRecv {
        s.mu.RLock()
        for _, sub := range s.subscribers {
            select {
            case sub.recvChan <- packet:
            default:
                // 버퍼 가득 차면 skip
            }
        }
        s.mu.RUnlock()
    }
}
```

**문제점**:
- ❌ Opus 프레임 믹싱이 매우 복잡 (디코딩 → PCM 믹싱 → 인코딩 필요)
- ❌ CPU 사용량 급증
- ❌ 지연 시간 증가
- ❌ 여전히 Guild당 하나의 채널에만 연결 가능 (근본 해결 안 됨)

**적용 가능성**: ⭐⭐ (구현 복잡도 대비 효과 낮음)

---

### 방안 C: 연결 제약 명시 (최소 변경)

**핵심 아이디어**: 다중 모듈 시나리오를 제한하고 명확히 문서화

```go
func (h *VoiceHelper) VoiceConnect(...) (*pb.VoiceConnectResponse, error) {
    // 1. 같은 Guild에 이미 연결이 있는지 확인
    h.mu.RLock()
    for _, state := range h.connections {
        if state.vc.GuildID == req.GuildId {
            h.mu.RUnlock()
            return nil, fmt.Errorf(
                "VOICE_ALREADY_CONNECTED: Another module is already connected to guild %s. "+
                "Only one voice connection per guild is supported.", req.GuildId)
        }
    }
    h.mu.RUnlock()

    // 2. 정상 연결 진행
    // ...
}
```

**사용 제약**:
- ⚠️ **한 Guild당 하나의 모듈만 음성 연결 가능**
- ⚠️ 다른 모듈은 해당 Guild 연결 해제될 때까지 대기 필요

**적용 가능성**: ⭐⭐⭐ (현실적이지만 제한적)

---

## 🎯 권장 해결 방안: 방안 A (Session 분리)

### 상세 구현 계획

#### 1. ModuleVoiceSession 구조 추가

**파일**: `shared/discord-v1/runtime/voice_helper.go`

```go
package runtime

import (
    "context"
    "fmt"
    "sync"

    "github.com/bwmarrin/discordgo"
    pb "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1"
    "google.golang.org/grpc/metadata"
)

type VoiceHelper struct {
    pb.UnimplementedVoiceStreamServer

    botToken       string
    masterSession  *discordgo.Session

    mu             sync.RWMutex
    moduleSessions map[string]*ModuleVoiceSession
}

type ModuleVoiceSession struct {
    moduleID    string
    dgSession   *discordgo.Session

    connMu      sync.RWMutex
    connections map[string]*VoiceConnectionState
}

type VoiceConnectionState struct {
    connectionID string
    vc           *discordgo.VoiceConnection

    sendChan     chan []byte
    recvChan     chan *discordgo.Packet

    streamMu     sync.RWMutex
    clientStream pb.VoiceStream_VoiceOpusStreamServer

    ctx          context.Context
    cancel       context.CancelFunc

    // 스트림 종료 대기용
    streamDone   chan struct{}
}

func NewVoiceHelper(masterSession *discordgo.Session) *VoiceHelper {
    return &VoiceHelper{
        botToken:       masterSession.Token,
        masterSession:  masterSession,
        moduleSessions: make(map[string]*ModuleVoiceSession),
    }
}
```

#### 2. 모듈 ID 추출

```go
func getModuleIDFromContext(ctx context.Context) (string, error) {
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return "", fmt.Errorf("no metadata in context")
    }

    moduleIDs := md.Get("module-id")
    if len(moduleIDs) == 0 {
        return "", fmt.Errorf("module-id not found in metadata")
    }

    return moduleIDs[0], nil
}
```

#### 3. 모듈별 Session 생성/획득

```go
func (h *VoiceHelper) getOrCreateModuleSession(moduleID string) (*ModuleVoiceSession, error) {
    h.mu.RLock()
    ms, exists := h.moduleSessions[moduleID]
    h.mu.RUnlock()

    if exists {
        return ms, nil
    }

    // 새 Session 생성
    h.mu.Lock()
    defer h.mu.Unlock()

    // Double-check pattern
    ms, exists = h.moduleSessions[moduleID]
    if exists {
        return ms, nil
    }

    dgSession, err := discordgo.New("Bot " + h.botToken)
    if err != nil {
        return nil, fmt.Errorf("failed to create session: %w", err)
    }

    // Gateway 연결은 필요 없음 (음성만 사용)
    // dgSession.Open() 호출 안 함

    ms = &ModuleVoiceSession{
        moduleID:    moduleID,
        dgSession:   dgSession,
        connections: make(map[string]*VoiceConnectionState),
    }

    h.moduleSessions[moduleID] = ms
    return ms, nil
}
```

#### 4. 개선된 VoiceConnect

```go
func (h *VoiceHelper) VoiceConnect(ctx context.Context, req *pb.VoiceConnectRequest) (*pb.VoiceConnectResponse, error) {
    // 1. 모듈 ID 획득
    moduleID, err := getModuleIDFromContext(ctx)
    if err != nil {
        return nil, fmt.Errorf("module identification failed: %w", err)
    }

    // 2. 모듈 세션 획득
    moduleSession, err := h.getOrCreateModuleSession(moduleID)
    if err != nil {
        return nil, err
    }

    // 3. 음성 채널 연결
    vc, err := moduleSession.dgSession.ChannelVoiceJoin(
        req.GuildId, req.ChannelId, req.Mute, req.Deaf,
    )
    if err != nil {
        return nil, fmt.Errorf("voice join failed: %w", err)
    }

    // 4. 연결 상태 생성
    ctx, cancel := context.WithCancel(context.Background())
    state := &VoiceConnectionState{
        connectionID: generateConnectionID(),
        vc:           vc,
        sendChan:     make(chan []byte, 100),
        recvChan:     make(chan *discordgo.Packet, 100),
        ctx:          ctx,
        cancel:       cancel,
        streamDone:   make(chan struct{}),
    }

    // 5. discordgo 채널 연결
    vc.OpusSend = state.sendChan
    vc.OpusRecv = state.recvChan

    // 6. 저장
    moduleSession.connMu.Lock()
    moduleSession.connections[state.connectionID] = state
    moduleSession.connMu.Unlock()

    return &pb.VoiceConnectResponse{
        ConnectionId: state.connectionID,
        Ready:        vc.Ready,
    }, nil
}
```

#### 5. 개선된 VoiceDisconnect (안전한 종료)

```go
func (h *VoiceHelper) VoiceDisconnect(ctx context.Context, req *pb.VoiceDisconnectRequest) (*pb.Empty, error) {
    // 1. 모듈 ID 획득
    moduleID, err := getModuleIDFromContext(ctx)
    if err != nil {
        return nil, err
    }

    // 2. 모듈 세션 찾기
    h.mu.RLock()
    moduleSession, exists := h.moduleSessions[moduleID]
    h.mu.RUnlock()
    if !exists {
        return nil, fmt.Errorf("module session not found")
    }

    // 3. 연결 상태 찾기 및 제거
    moduleSession.connMu.Lock()
    state, exists := moduleSession.connections[req.ConnectionId]
    if exists {
        delete(moduleSession.connections, req.ConnectionId)
    }
    moduleSession.connMu.Unlock()

    if !exists {
        return nil, fmt.Errorf("connection not found: %s", req.ConnectionId)
    }

    // 4. 종료 시퀀스 (순서 중요!)

    // 4-1. Context 취소 (goroutine 종료 신호)
    state.cancel()

    // 4-2. 스트림 goroutine 종료 대기 (타임아웃 있음)
    select {
    case <-state.streamDone:
        // 정상 종료
    case <-time.After(5 * time.Second):
        // 타임아웃 (강제 종료)
        log.Warn("stream goroutine did not terminate within timeout")
    }

    // 4-3. VoiceConnection 종료
    if state.vc != nil {
        err := state.vc.Disconnect()
        if err != nil {
            log.Error("voice disconnect error:", err)
        }
    }

    // 4-4. 채널 닫기 (goroutine 종료 후에만!)
    close(state.sendChan)
    close(state.recvChan)

    return &pb.Empty{}, nil
}
```

#### 6. 개선된 VoiceOpusStream (스트림 중복 방지)

```go
func (h *VoiceHelper) VoiceOpusStream(stream pb.VoiceStream_VoiceOpusStreamServer) error {
    // 1. 모듈 ID 획득
    moduleID, err := getModuleIDFromContext(stream.Context())
    if err != nil {
        return err
    }

    // 2. 첫 패킷 수신
    firstPacket, err := stream.Recv()
    if err != nil {
        return err
    }
    connectionID := firstPacket.ConnectionId

    // 3. 모듈 세션 찾기
    h.mu.RLock()
    moduleSession, exists := h.moduleSessions[moduleID]
    h.mu.RUnlock()
    if !exists {
        return fmt.Errorf("module session not found")
    }

    // 4. 연결 상태 찾기
    moduleSession.connMu.RLock()
    state, exists := moduleSession.connections[connectionID]
    moduleSession.connMu.RUnlock()
    if !exists {
        return fmt.Errorf("connection not found: %s", connectionID)
    }

    // 5. 스트림 중복 확인 및 설정
    state.streamMu.Lock()
    if state.clientStream != nil {
        state.streamMu.Unlock()
        return fmt.Errorf("stream already active for connection %s", connectionID)
    }
    state.clientStream = stream
    state.streamMu.Unlock()

    // 6. 종료 시 정리
    defer func() {
        state.streamMu.Lock()
        state.clientStream = nil
        state.streamMu.Unlock()

        close(state.streamDone)  // Disconnect 대기 해제
    }()

    // 7. 송신/수신 goroutine 시작
    errChan := make(chan error, 2)

    // 7-1. 수신 루프 (Discord → 모듈)
    go func() {
        for {
            select {
            case packet, ok := <-state.recvChan:
                if !ok {
                    errChan <- nil
                    return
                }

                err := stream.Send(&pb.OpusPacket{
                    ConnectionId: connectionID,
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
                errChan <- nil
                return
            }
        }
    }()

    // 7-2. 송신 루프 (모듈 → Discord)
    go func() {
        // 첫 패킷 처리
        if len(firstPacket.OpusData) > 0 {
            select {
            case state.sendChan <- firstPacket.OpusData:
            case <-state.ctx.Done():
                errChan <- nil
                return
            }
        }

        // 이후 패킷 처리
        for {
            packet, err := stream.Recv()
            if err != nil {
                errChan <- err
                return
            }

            select {
            case state.sendChan <- packet.OpusData:
            case <-state.ctx.Done():
                errChan <- nil
                return
            }
        }
    }()

    // 8. 에러 대기
    return <-errChan
}
```

---

## 📝 Module Client 수정사항

**파일**: `shared/discord-v1/module/voice_client.go`

```go
import (
    "google.golang.org/grpc/metadata"
)

type VoiceClient struct {
    client   pb.VoiceStreamClient
    moduleID string  // 추가

    // ... 나머지 동일
}

func NewVoiceClient(client pb.VoiceStreamClient, moduleID string) *VoiceClient {
    return &VoiceClient{
        client:   client,
        moduleID: moduleID,
        SendChan: make(chan []byte, 100),
        RecvChan: make(chan *OpusPacket, 100),
    }
}

func (vc *VoiceClient) Connect(ctx context.Context, guildID, channelID string, mute, deaf bool) error {
    // gRPC metadata에 module-id 추가
    ctx = metadata.AppendToOutgoingContext(ctx, "module-id", vc.moduleID)

    // VoiceConnect RPC (metadata 포함)
    resp, err := vc.client.VoiceConnect(ctx, &pb.VoiceConnectRequest{
        GuildId:   guildID,
        ChannelId: channelID,
        Mute:      mute,
        Deaf:      deaf,
    })
    // ... 나머지 동일
}

func (vc *VoiceClient) startStream(ctx context.Context) error {
    // metadata 추가
    ctx = metadata.AppendToOutgoingContext(ctx, "module-id", vc.moduleID)

    vc.ctx, vc.cancel = context.WithCancel(ctx)

    stream, err := vc.client.VoiceOpusStream(vc.ctx)
    // ... 나머지 동일
}
```

---

## 🧪 테스트 시나리오

### 시나리오 1: 다중 모듈, 같은 Guild, 다른 채널

```go
func TestMultiModuleSameGuildDifferentChannels(t *testing.T) {
    // Module A
    clientA := NewVoiceClient(conn, "module-a")
    err := clientA.Connect(ctx, "guild-1", "channel-a", false, false)
    assert.NoError(t, err)

    // Module B (같은 Guild, 다른 채널)
    clientB := NewVoiceClient(conn, "module-b")
    err = clientB.Connect(ctx, "guild-1", "channel-b", false, false)
    assert.NoError(t, err)  // ← 방안 A: 성공해야 함!

    // 데이터 전송 테스트
    clientA.SendChan <- opusDataA
    clientB.SendChan <- opusDataB

    // 검증: channelA에 dataA, channelB에 dataB 전송됨
}
```

### 시나리오 2: 연결 중 Disconnect

```go
func TestDisconnectDuringStream(t *testing.T) {
    client := NewVoiceClient(conn, "module-test")
    client.Connect(ctx, "guild-1", "channel-1", false, false)

    // 스트림 활성화
    go func() {
        for {
            client.SendChan <- opusData
            time.Sleep(20 * time.Millisecond)
        }
    }()

    time.Sleep(100 * time.Millisecond)

    // Disconnect
    client.Disconnect()

    // 검증: Panic 없음, goroutine 정상 종료
}
```

---

## 📚 참고 자료

### discordgo 소스 분석
- `discordgo/structs.go:90` - VoiceConnections 맵 (Guild ID 키)
- `discordgo/wsapi.go:704-717` - ChannelVoiceJoin 구현
- `discordgo/voice.go:30-69` - VoiceConnection 구조체

### gRPC Metadata
- [gRPC Metadata Guide](https://grpc.io/docs/guides/metadata/)
- Context를 통한 메타데이터 전달 패턴

---

## ✅ 체크리스트

### 아키텍처 변경
- [ ] `ModuleVoiceSession` 구조체 추가
- [ ] `getModuleIDFromContext()` 구현
- [ ] `getOrCreateModuleSession()` 구현

### VoiceHelper 수정
- [ ] `moduleSessions` 맵 추가
- [ ] `VoiceConnect` - 모듈별 세션 사용
- [ ] `VoiceOpusStream` - 스트림 중복 방지
- [ ] `VoiceDisconnect` - 안전한 종료 시퀀스
- [ ] `VoiceSpeaking` - 모듈 세션에서 연결 찾기

### VoiceClient 수정
- [ ] `moduleID` 필드 추가
- [ ] gRPC metadata에 module-id 전달

### 테스트
- [ ] 다중 모듈 연결 테스트
- [ ] 동시 스트림 테스트
- [ ] Disconnect race condition 테스트
- [ ] 메모리 누수 테스트

### 문서화
- [ ] 사용 제약사항 명시 (Session 수 증가)
- [ ] 모듈 ID 할당 방법 문서화
- [ ] 예제 코드 업데이트

---

## 🎯 결론

**현재 구현 계획의 문제점**:
1. ❌ Guild당 VoiceConnection 제약 무시
2. ❌ OpusSend/OpusRecv 채널 충돌
3. ❌ 스트림 상태 race condition
4. ❌ 연결 해제 타이밍 이슈

**해결 방안 A (Session 분리)의 효과**:
1. ✅ 모듈별 독립 Session → Guild 제약 회피
2. ✅ 채널 충돌 완전 제거
3. ✅ 모듈 간 완전 격리
4. ✅ 안전한 동시성 보장

**구현 우선순위**:
1. **High Priority**: 방안 A 적용 (Session 분리)
2. **Medium Priority**: 스트림 중복 방지 로직
3. **Medium Priority**: 안전한 Disconnect 시퀀스
4. **Low Priority**: 성능 최적화 (Session 풀링 등)

이 분석을 바탕으로 `voice-stream-implementation-plan.md`를 전면 수정해야 합니다.
