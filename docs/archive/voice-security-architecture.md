# Voice Stream 보안 아키텍처 개선안
## Runtime-Managed Security Architecture for Voice Streaming

**작성 날짜**: 2025-02-11
**목적**: 모듈에 Session을 노출하지 않고 런타임이 모든 Discord 연결을 관리하는 안전한 아키텍처 설계

---

## 🔒 보안 요구사항

### 기존 방안 A의 보안 문제

**문제점**:
```go
// 방안 A: 각 모듈에 독립 Session 생성
type ModuleVoiceSession struct {
    moduleID    string
    dgSession   *discordgo.Session  // ← 모듈별 Session
}
```

**보안 우려사항**:
1. ❌ **Session 객체 노출**: 모듈이 Session을 통해 예상치 못한 Discord API 호출 가능
2. ❌ **토큰 재사용**: 같은 봇 토큰으로 여러 Session 생성 시 Discord API 정책 위반 가능
3. ❌ **리소스 남용**: 악의적 모듈이 무제한 Session 생성 시도 가능
4. ❌ **감사 추적 어려움**: 어떤 모듈이 어떤 작업을 했는지 추적 복잡

### 보안 원칙

✅ **최소 권한 원칙**: 모듈은 gRPC VoiceStream API만 접근
✅ **중앙 집중 관리**: 런타임이 모든 Discord 연결 생명주기 관리
✅ **리소스 제한**: 모듈당 최대 연결 수, 대역폭 등 제한
✅ **감사 가능성**: 모든 음성 연결 활동 로깅 및 추적

---

## 🎯 개선 방안들

### 방안 B1: 단일 Session + 연결 정책 관리 (권장) ⭐⭐⭐⭐⭐

**핵심 아이디어**: 하나의 Session만 사용하되, 런타임이 연결 충돌을 정책으로 관리

#### 아키텍처

```go
type VoiceHelper struct {
    pb.UnimplementedVoiceStreamServer

    // 단일 마스터 Session
    dgSession *discordgo.Session

    // 런타임 관리 연결 풀
    mu          sync.RWMutex
    connections map[string]*ManagedVoiceConnection  // connection_id → state

    // Guild별 활성 연결 추적
    guildConnections map[string]*ManagedVoiceConnection  // guild_id → active_conn
}

type ManagedVoiceConnection struct {
    connectionID string
    moduleID     string  // 어떤 모듈이 소유하는지
    guildID      string
    channelID    string

    // Discord 연결
    vc           *discordgo.VoiceConnection

    // 스트림 채널
    sendChan     chan []byte
    recvChan     chan *discordgo.Packet

    // gRPC 스트림
    streamMu     sync.RWMutex
    clientStream pb.VoiceStream_VoiceOpusStreamServer

    // 생명주기 관리
    ctx          context.Context
    cancel       context.CancelFunc
    streamDone   chan struct{}

    // 보안 제한
    createdAt    time.Time
    lastActivity time.Time
    bytesSent    uint64
    bytesRecv    uint64
}
```

#### 연결 정책

**정책 1: Guild당 하나의 연결 (FIFO 정책)**
```go
func (h *VoiceHelper) VoiceConnect(ctx context.Context, req *pb.VoiceConnectRequest) (*pb.VoiceConnectResponse, error) {
    moduleID, _ := getModuleIDFromContext(ctx)

    h.mu.Lock()
    defer h.mu.Unlock()

    // 1. 같은 Guild에 기존 연결이 있는지 확인
    if existingConn, exists := h.guildConnections[req.GuildId]; exists {
        // 기존 연결 강제 종료
        log.Warn("Disconnecting existing connection to guild %s (module %s) for new connection (module %s)",
            req.GuildId, existingConn.moduleID, moduleID)

        // 기존 모듈에 통지
        h.notifyConnectionPreempted(existingConn)

        // 기존 연결 정리
        h.disconnectUnsafe(existingConn)
    }

    // 2. 새 연결 생성
    vc, err := h.dgSession.ChannelVoiceJoin(req.GuildId, req.ChannelId, req.Mute, req.Deaf)
    if err != nil {
        return nil, err
    }

    // 3. 관리 객체 생성
    ctx, cancel := context.WithCancel(context.Background())
    conn := &ManagedVoiceConnection{
        connectionID: generateConnectionID(),
        moduleID:     moduleID,
        guildID:      req.GuildId,
        channelID:    req.ChannelId,
        vc:           vc,
        sendChan:     make(chan []byte, 100),
        recvChan:     make(chan *discordgo.Packet, 100),
        ctx:          ctx,
        cancel:       cancel,
        streamDone:   make(chan struct{}),
        createdAt:    time.Now(),
        lastActivity: time.Now(),
    }

    // 4. 채널 연결
    vc.OpusSend = conn.sendChan
    vc.OpusRecv = conn.recvChan

    // 5. 등록
    h.connections[conn.connectionID] = conn
    h.guildConnections[req.GuildId] = conn  // Guild 추적

    return &pb.VoiceConnectResponse{
        ConnectionId: conn.connectionID,
        Ready:        vc.Ready,
    }, nil
}

// 기존 연결이 강제 종료됨을 모듈에 통지
func (h *VoiceHelper) notifyConnectionPreempted(conn *ManagedVoiceConnection) {
    conn.streamMu.RLock()
    stream := conn.clientStream
    conn.streamMu.RUnlock()

    if stream != nil {
        // 특별한 패킷으로 통지
        stream.Send(&pb.OpusPacket{
            ConnectionId: conn.connectionID,
            OpusData:     nil,  // Empty data
            Type:         []byte("PREEMPTED"),  // 특수 타입
        })
    }
}
```

**정책 2: 모듈당 최대 연결 수 제한**
```go
const MaxConnectionsPerModule = 3

func (h *VoiceHelper) VoiceConnect(...) {
    moduleID, _ := getModuleIDFromContext(ctx)

    h.mu.RLock()
    moduleConnCount := h.countModuleConnections(moduleID)
    h.mu.RUnlock()

    if moduleConnCount >= MaxConnectionsPerModule {
        return nil, fmt.Errorf("module %s has reached max connections (%d)",
            moduleID, MaxConnectionsPerModule)
    }

    // 연결 진행...
}

func (h *VoiceHelper) countModuleConnections(moduleID string) int {
    count := 0
    for _, conn := range h.connections {
        if conn.moduleID == moduleID {
            count++
        }
    }
    return count
}
```

**정책 3: 연결 타임아웃 (유휴 연결 자동 정리)**
```go
func (h *VoiceHelper) startConnectionMonitor() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        h.mu.Lock()
        now := time.Now()

        for connID, conn := range h.connections {
            // 30분 동안 활동 없으면 종료
            if now.Sub(conn.lastActivity) > 30*time.Minute {
                log.Warn("Auto-disconnecting idle connection %s (module %s)",
                    connID, conn.moduleID)
                h.disconnectUnsafe(conn)
            }
        }
        h.mu.Unlock()
    }
}

// 활동 업데이트
func (h *VoiceHelper) updateActivity(conn *ManagedVoiceConnection, bytes uint64) {
    conn.lastActivity = time.Now()
    conn.bytesSent += bytes
}
```

#### 장점

✅ **보안 강화**: 모듈은 gRPC API만 접근, Session 노출 없음
✅ **리소스 관리**: 연결 수, 타임아웃, 대역폭 제한 가능
✅ **감사 추적**: 모든 연결에 module_id 기록
✅ **단순성**: 하나의 Session만 관리, Discord API 정책 준수
✅ **예측 가능성**: 명확한 연결 정책 (FIFO)

#### 단점

⚠️ **동시성 제약**: 같은 Guild에 여러 모듈 동시 연결 불가
⚠️ **연결 선점**: 기존 연결이 강제 종료될 수 있음 (통지는 함)

#### 적용성

**적합한 경우**:
- 대부분의 사용 사례 (음악 봇, 녹음 봇 등)
- 같은 Guild에 여러 모듈이 동시 연결할 필요가 거의 없는 경우
- 보안과 단순성이 우선인 경우

---

### 방안 B2: 모듈별 봇 계정 (최고 보안) ⭐⭐⭐⭐

**핵심 아이디어**: 각 모듈에 독립적인 Discord 봇 계정 할당

#### 아키텍처

```go
type VoiceHelper struct {
    pb.UnimplementedVoiceStreamServer

    // 모듈 → 봇 토큰 매핑
    mu             sync.RWMutex
    moduleBotTokens map[string]string  // module_id → bot_token

    // 모듈 → Session 매핑
    moduleSessions  map[string]*ModuleSession  // module_id → session

    // 연결 관리
    connections     map[string]*ManagedVoiceConnection
}

type ModuleSession struct {
    moduleID    string
    botToken    string
    dgSession   *discordgo.Session  // 모듈 전용 봇의 Session

    connMu      sync.RWMutex
    connections map[string]*ManagedVoiceConnection
}

func NewVoiceHelper(moduleBotTokens map[string]string) *VoiceHelper {
    return &VoiceHelper{
        moduleBotTokens: moduleBotTokens,
        moduleSessions:  make(map[string]*ModuleSession),
        connections:     make(map[string]*ManagedVoiceConnection),
    }
}

func (h *VoiceHelper) VoiceConnect(ctx context.Context, req *pb.VoiceConnectRequest) (*pb.VoiceConnectResponse, error) {
    moduleID, _ := getModuleIDFromContext(ctx)

    // 1. 모듈의 봇 토큰 확인
    h.mu.RLock()
    botToken, exists := h.moduleBotTokens[moduleID]
    h.mu.RUnlock()

    if !exists {
        return nil, fmt.Errorf("module %s has no assigned bot token", moduleID)
    }

    // 2. 모듈 Session 획득 또는 생성
    moduleSession, err := h.getOrCreateModuleSession(moduleID, botToken)
    if err != nil {
        return nil, err
    }

    // 3. 모듈의 봇으로 음성 연결
    vc, err := moduleSession.dgSession.ChannelVoiceJoin(
        req.GuildId, req.ChannelId, req.Mute, req.Deaf,
    )
    if err != nil {
        return nil, err
    }

    // 4. 연결 관리 객체 생성 (방안 B1과 동일)
    // ...

    return &pb.VoiceConnectResponse{
        ConnectionId: conn.connectionID,
        Ready:        vc.Ready,
    }, nil
}
```

#### 봇 토큰 관리

**설정 파일** (`config/module-bots.yaml`):
```yaml
module_bots:
  module-music:
    token: "Bot MTA1..."
    description: "Music playback module bot"
    permissions:
      - VOICE_CONNECT
      - VOICE_SPEAK

  module-recorder:
    token: "Bot MTA2..."
    description: "Voice recording module bot"
    permissions:
      - VOICE_CONNECT

  module-tts:
    token: "Bot MTA3..."
    description: "Text-to-speech module bot"
    permissions:
      - VOICE_CONNECT
      - VOICE_SPEAK
```

**런타임 로딩**:
```go
func LoadModuleBotConfig(path string) (map[string]string, error) {
    config := struct {
        ModuleBots map[string]struct {
            Token       string   `yaml:"token"`
            Description string   `yaml:"description"`
            Permissions []string `yaml:"permissions"`
        } `yaml:"module_bots"`
    }{}

    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    err = yaml.Unmarshal(data, &config)
    if err != nil {
        return nil, err
    }

    tokens := make(map[string]string)
    for moduleID, bot := range config.ModuleBots {
        tokens[moduleID] = bot.Token
    }

    return tokens, nil
}

// main.go
func main() {
    moduleBotTokens, err := LoadModuleBotConfig("config/module-bots.yaml")
    if err != nil {
        log.Fatal(err)
    }

    voiceHelper := NewVoiceHelper(moduleBotTokens)
    // ...
}
```

#### 장점

✅ **완전한 격리**: 각 모듈이 독립적인 Discord 봇 사용
✅ **최고 보안**: 모듈별 권한 분리, 악의적 모듈 영향 최소화
✅ **동시 연결**: 같은 Guild에 여러 모듈 봇이 동시 연결 가능
✅ **감사 추적**: Discord 측에서도 봇별로 활동 추적 가능
✅ **확장성**: 모듈 추가 시 봇만 추가하면 됨

#### 단점

⚠️ **운영 복잡도**: 모듈마다 Discord 봇 생성 및 관리 필요
⚠️ **비용**: Discord 봇 수 증가 (무료이지만 관리 부담)
⚠️ **서버 부담**: 각 봇이 서버에 조인되어 멤버 수 증가

#### 적용성

**적합한 경우**:
- 엔터프라이즈급 보안 요구사항
- 모듈 간 완전한 권한 분리 필요
- 같은 Guild에 여러 음성 기능 동시 실행 필요
- 모듈 수가 제한적 (< 10개)

---

### 방안 B3: 하이브리드 (유연성) ⭐⭐⭐

**핵심 아이디어**: 기본은 단일 Session, 필요 시 모듈별 봇 지원

#### 아키텍처

```go
type VoiceHelper struct {
    pb.UnimplementedVoiceStreamServer

    // 기본 마스터 봇
    masterSession *discordgo.Session

    // 선택적 모듈별 봇
    mu             sync.RWMutex
    moduleSessions map[string]*ModuleSession  // 모듈별 봇이 있으면 여기

    // 연결 관리
    connections      map[string]*ManagedVoiceConnection
    guildConnections map[string]*ManagedVoiceConnection  // FIFO 정책용
}

func (h *VoiceHelper) VoiceConnect(ctx context.Context, req *pb.VoiceConnectRequest) (*pb.VoiceConnectResponse, error) {
    moduleID, _ := getModuleIDFromContext(ctx)

    // 1. 모듈 전용 봇이 있는지 확인
    h.mu.RLock()
    moduleSession, hasOwnBot := h.moduleSessions[moduleID]
    h.mu.RUnlock()

    var vc *discordgo.VoiceConnection
    var err error

    if hasOwnBot {
        // 모듈 전용 봇 사용
        vc, err = moduleSession.dgSession.ChannelVoiceJoin(...)
    } else {
        // 마스터 봇 사용 (FIFO 정책 적용)
        h.mu.Lock()
        if existingConn, exists := h.guildConnections[req.GuildId]; exists {
            h.notifyConnectionPreempted(existingConn)
            h.disconnectUnsafe(existingConn)
        }
        h.mu.Unlock()

        vc, err = h.masterSession.ChannelVoiceJoin(...)
    }

    // 연결 관리 동일...
}
```

#### 설정

```yaml
# 마스터 봇 (필수)
master_bot:
  token: "Bot MASTER_TOKEN"

# 모듈별 봇 (선택)
module_bots:
  module-premium-music:  # 프리미엄 기능은 전용 봇
    token: "Bot PREMIUM_TOKEN"

  # module-basic은 마스터 봇 공유
```

#### 장점

✅ **유연성**: 중요 모듈은 전용 봇, 일반 모듈은 공유 봇
✅ **점진적 확장**: 처음엔 단순하게, 필요시 모듈별 봇 추가
✅ **비용 효율**: 필요한 경우만 추가 봇 생성

#### 단점

⚠️ **복잡도 증가**: 두 방식 혼재로 인한 관리 복잡도
⚠️ **정책 혼란**: 어떤 모듈은 FIFO, 어떤 모듈은 독립 연결

#### 적용성

**적합한 경우**:
- 초기에는 단순하게, 성장 시 확장 필요
- 일부 프리미엄/중요 기능만 전용 봇 필요
- 점진적 마이그레이션 선호

---

## 📊 방안 비교

| 항목 | B1: 단일 Session + 정책 | B2: 모듈별 봇 | B3: 하이브리드 |
|------|----------------------|------------|------------|
| **보안** | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **동시 연결** | ❌ Guild당 1개 | ✅ 무제한 | ⚠️ 혼재 |
| **구현 복잡도** | ⭐⭐ 낮음 | ⭐⭐⭐⭐ 높음 | ⭐⭐⭐ 중간 |
| **운영 복잡도** | ⭐⭐ 낮음 | ⭐⭐⭐⭐ 높음 | ⭐⭐⭐ 중간 |
| **리소스 사용** | ⭐⭐⭐⭐⭐ 최소 | ⭐⭐ 높음 | ⭐⭐⭐ 중간 |
| **확장성** | ⭐⭐⭐ 제한적 | ⭐⭐⭐⭐⭐ 우수 | ⭐⭐⭐⭐ 좋음 |
| **감사 추적** | ⭐⭐⭐⭐ 좋음 | ⭐⭐⭐⭐⭐ 최고 | ⭐⭐⭐⭐ 좋음 |

---

## 🎯 권장 방안: B1 (단일 Session + 정책 관리)

### 선택 이유

1. **보안 요구사항 충족**: 모듈은 gRPC API만 접근, Session 노출 없음
2. **실용성**: 대부분의 사용 사례에서 Guild당 하나의 음성 연결로 충분
3. **단순성**: 하나의 Session만 관리, 운영 부담 최소
4. **확장 가능**: 필요시 방안 B2로 마이그레이션 가능

### 구현 우선순위

**Phase 1: 기본 정책 구현**
- [x] ManagedVoiceConnection 구조체
- [x] Guild당 FIFO 정책
- [x] 연결 선점 통지 메커니즘
- [x] 모듈별 연결 수 제한

**Phase 2: 보안 강화**
- [ ] 연결 타임아웃 및 자동 정리
- [ ] 대역폭 사용량 추적
- [ ] 감사 로깅 (connection lifecycle)
- [ ] Rate limiting (연결 시도 제한)

**Phase 3: 모니터링**
- [ ] 연결 상태 메트릭
- [ ] 모듈별 리소스 사용량 대시보드
- [ ] 이상 탐지 (비정상 연결 패턴)

---

## 🔐 보안 강화 기능

### 1. 연결 승인 (Authorization)

```go
type ConnectionPolicy struct {
    AllowedGuilds   []string  // 모듈이 접근 가능한 Guild 목록
    AllowedChannels []string  // 모듈이 접근 가능한 Channel 목록
    MaxConnections  int       // 최대 동시 연결 수
    MaxBandwidth    uint64    // 최대 대역폭 (bytes/sec)
}

func (h *VoiceHelper) checkConnectionAuthorization(moduleID, guildID, channelID string) error {
    policy := h.loadModulePolicy(moduleID)

    // Guild 권한 확인
    if len(policy.AllowedGuilds) > 0 {
        if !contains(policy.AllowedGuilds, guildID) {
            return fmt.Errorf("module %s not authorized for guild %s", moduleID, guildID)
        }
    }

    // Channel 권한 확인
    if len(policy.AllowedChannels) > 0 {
        if !contains(policy.AllowedChannels, channelID) {
            return fmt.Errorf("module %s not authorized for channel %s", moduleID, channelID)
        }
    }

    return nil
}

func (h *VoiceHelper) VoiceConnect(...) {
    moduleID, _ := getModuleIDFromContext(ctx)

    // 권한 확인
    if err := h.checkConnectionAuthorization(moduleID, req.GuildId, req.ChannelId); err != nil {
        return nil, status.Error(codes.PermissionDenied, err.Error())
    }

    // 연결 진행...
}
```

### 2. Rate Limiting

```go
type RateLimiter struct {
    mu        sync.Mutex
    buckets   map[string]*tokenBucket  // module_id → bucket
}

type tokenBucket struct {
    tokens        int
    lastRefill    time.Time
    maxTokens     int
    refillRate    time.Duration
}

func (rl *RateLimiter) Allow(moduleID string) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    bucket, exists := rl.buckets[moduleID]
    if !exists {
        bucket = &tokenBucket{
            tokens:     5,
            maxTokens:  5,
            refillRate: 1 * time.Minute,
            lastRefill: time.Now(),
        }
        rl.buckets[moduleID] = bucket
    }

    // 토큰 리필
    now := time.Now()
    if now.Sub(bucket.lastRefill) > bucket.refillRate {
        bucket.tokens = bucket.maxTokens
        bucket.lastRefill = now
    }

    // 토큰 소비
    if bucket.tokens > 0 {
        bucket.tokens--
        return true
    }

    return false
}

func (h *VoiceHelper) VoiceConnect(...) {
    moduleID, _ := getModuleIDFromContext(ctx)

    if !h.rateLimiter.Allow(moduleID) {
        return nil, status.Error(codes.ResourceExhausted,
            "connection rate limit exceeded")
    }

    // 연결 진행...
}
```

### 3. 감사 로깅

```go
type AuditLog struct {
    Timestamp    time.Time
    ModuleID     string
    Action       string  // "CONNECT", "DISCONNECT", "SEND", "RECEIVE"
    GuildID      string
    ChannelID    string
    BytesCount   uint64
    Success      bool
    ErrorMessage string
}

func (h *VoiceHelper) auditLog(log AuditLog) {
    // 데이터베이스 또는 파일에 기록
    h.auditLogger.Log(log)

    // 중요 이벤트는 메트릭으로도 전송
    if !log.Success {
        h.metrics.RecordError(log.ModuleID, log.Action)
    }
}

func (h *VoiceHelper) VoiceConnect(...) {
    moduleID, _ := getModuleIDFromContext(ctx)

    // 연결 시도 로깅
    h.auditLog(AuditLog{
        Timestamp: time.Now(),
        ModuleID:  moduleID,
        Action:    "CONNECT",
        GuildID:   req.GuildId,
        ChannelID: req.ChannelId,
        Success:   false,  // 아직 실패
    })

    // 연결 진행...

    // 연결 성공 로깅
    h.auditLog(AuditLog{
        Timestamp: time.Now(),
        ModuleID:  moduleID,
        Action:    "CONNECT",
        GuildID:   req.GuildId,
        ChannelID: req.ChannelId,
        Success:   true,
    })
}
```

---

## 📋 구현 체크리스트

### Core Architecture (방안 B1)
- [ ] ManagedVoiceConnection 구조체 정의
- [ ] VoiceHelper 단일 Session 관리
- [ ] Guild 연결 추적 맵 (guildConnections)
- [ ] 연결 선점 로직 구현
- [ ] 선점 통지 메커니즘

### Security Features
- [ ] 모듈 ID 추출 (gRPC metadata)
- [ ] ConnectionPolicy 구조 및 로딩
- [ ] Authorization 체크 로직
- [ ] Rate limiter 구현
- [ ] Audit logging 시스템

### Resource Management
- [ ] 모듈별 최대 연결 수 제한
- [ ] 연결 타임아웃 모니터
- [ ] 유휴 연결 자동 정리
- [ ] 대역폭 사용량 추적
- [ ] 메모리 사용량 모니터링

### Error Handling
- [ ] gRPC status code 표준화
- [ ] 연결 실패 시 재시도 정책
- [ ] 선점 시 graceful shutdown
- [ ] 에러 복구 메커니즘

### Documentation
- [ ] 연결 정책 문서화
- [ ] 모듈 개발 가이드 (제약사항)
- [ ] 보안 설정 가이드
- [ ] 트러블슈팅 가이드

---

## 🔄 방안 B2로 마이그레이션 경로

만약 추후 방안 B1의 제약이 문제가 된다면:

**Step 1**: 모듈별 봇 토큰 설정 시스템 추가
```yaml
# config/module-bots.yaml 생성
```

**Step 2**: ModuleSession 구조 도입
```go
type ModuleSession struct {
    moduleID    string
    botToken    string
    dgSession   *discordgo.Session
}
```

**Step 3**: VoiceConnect 로직 수정
```go
// 모듈 봇 토큰이 있으면 전용 Session 사용
if botToken := h.getModuleBotToken(moduleID); botToken != "" {
    session := h.getOrCreateModuleSession(moduleID, botToken)
    vc = session.dgSession.ChannelVoiceJoin(...)
} else {
    vc = h.dgSession.ChannelVoiceJoin(...)  // 기존 로직
}
```

**Step 4**: 점진적 마이그레이션
- 중요 모듈부터 전용 봇 할당
- 일반 모듈은 기존 마스터 봇 계속 사용

---

## 🎯 결론

**권장**: 방안 B1 (단일 Session + 정책 관리)

**이유**:
1. ✅ 보안 요구사항 충족 (모듈 Session 노출 없음)
2. ✅ 실용적 (대부분 사용 사례에 충분)
3. ✅ 단순함 (운영 복잡도 최소)
4. ✅ 확장 가능 (필요시 B2로 마이그레이션)

**핵심 설계 원칙**:
- 런타임이 모든 Discord 연결 관리
- 모듈은 gRPC VoiceStream API만 접근
- 명확한 연결 정책 (Guild당 FIFO)
- 리소스 제한 및 감사 추적

이 아키텍처로 `voice-stream-implementation-plan.md`를 전면 수정해야 합니다.
