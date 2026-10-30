# Voice Stream 큐 기반 아키텍처
## Queue-Based Voice Streaming Architecture

**작성 날짜**: 2025-02-11
**설계 방향**: 단일 VoiceConnection + 큐 스케줄링 시스템

---

## 🎯 핵심 설계 원칙

### 단순성 우선
- **단일 VoiceConnection**: Guild당 하나의 음성 연결만 사용
- **큐 기반 송신**: 여러 모듈의 음성 데이터를 큐에 적재, 순차 처리
- **브로드캐스트 수신**: 수신 음성은 모든 구독 모듈에 전달
- **멀티플렉싱 없음**: 음성 믹싱/합성 불필요

### 자연스러운 동시성 제어
- 모듈들이 큐를 통해 자동으로 순서 대기
- 우선순위 또는 공정성 정책 적용 가능
- 명시적인 연결 선점 없음 (자연스러운 양보)

---

## 🏗️ 시스템 아키텍처

### 전체 구조

```
Module A                Module B                Module C
   │                       │                       │
   ├─ SendChan            ├─ SendChan            ├─ SendChan
   │     │                │     │                │     │
   │     └────────────────┼─────┴────────────────┘     │
   │                      │                            │
   │                      ↓                            │
   │               VoiceTaskQueue                      │
   │                 │   │   │                         │
   │                 ↓   ↓   ↓                         │
   │              QueueScheduler                       │
   │                      │                            │
   │                      ↓                            │
   │              VoiceConnection ← UDP → Discord      │
   │                      │                            │
   │                      ↓                            │
   │              ReceiveBroadcaster                   │
   │                      │                            │
   │     ┌────────────────┼────────────────────┐      │
   │     ↓                ↓                    ↓      │
   └─ RecvChan         RecvChan             RecvChan ─┘
```

### 주요 컴포넌트

#### 1. VoiceTaskQueue (음성 태스크 큐)
```go
type VoiceTask struct {
    TaskID      string
    ModuleID    string
    OpusData    []byte
    Priority    int          // 우선순위 (높을수록 먼저)
    SubmitTime  time.Time
    Timeout     time.Duration
}

type VoiceTaskQueue struct {
    mu     sync.RWMutex
    tasks  []*VoiceTask  // 우선순위 힙 또는 FIFO 큐
    cond   *sync.Cond    // 태스크 대기용
    closed bool
}

func (q *VoiceTaskQueue) Enqueue(task *VoiceTask) error {
    q.mu.Lock()
    defer q.mu.Unlock()

    if q.closed {
        return fmt.Errorf("queue closed")
    }

    // 우선순위 큐 또는 FIFO 삽입
    q.tasks = append(q.tasks, task)
    q.sortByPriority()  // 또는 그냥 append (FIFO)

    q.cond.Signal()  // 대기 중인 스케줄러 깨우기
    return nil
}

func (q *VoiceTaskQueue) Dequeue() (*VoiceTask, error) {
    q.mu.Lock()
    defer q.mu.Unlock()

    for len(q.tasks) == 0 && !q.closed {
        q.cond.Wait()  // 태스크가 올 때까지 대기
    }

    if q.closed {
        return nil, fmt.Errorf("queue closed")
    }

    task := q.tasks[0]
    q.tasks = q.tasks[1:]
    return task, nil
}
```

#### 2. QueueScheduler (큐 스케줄러)
```go
type QueueScheduler struct {
    queue           *VoiceTaskQueue
    voiceConnection *discordgo.VoiceConnection

    ctx    context.Context
    cancel context.CancelFunc

    // 통계
    mu            sync.RWMutex
    tasksProcessed map[string]uint64  // module_id → count
}

func (s *QueueScheduler) Start() {
    go s.processLoop()
}

func (s *QueueScheduler) processLoop() {
    for {
        select {
        case <-s.ctx.Done():
            return
        default:
        }

        // 큐에서 태스크 가져오기 (블로킹)
        task, err := s.queue.Dequeue()
        if err != nil {
            continue
        }

        // 타임아웃 체크
        if time.Since(task.SubmitTime) > task.Timeout {
            log.Warn("Task %s from module %s timed out", task.TaskID, task.ModuleID)
            continue
        }

        // VoiceConnection으로 전송
        select {
        case s.voiceConnection.OpusSend <- task.OpusData:
            // 전송 성공
            s.mu.Lock()
            s.tasksProcessed[task.ModuleID]++
            s.mu.Unlock()

        case <-time.After(100 * time.Millisecond):
            // 전송 실패 (채널 가득 참)
            log.Warn("Failed to send task %s (channel full)", task.TaskID)

        case <-s.ctx.Done():
            return
        }

        // 프레임 간격 유지 (20ms per Opus frame)
        time.Sleep(20 * time.Millisecond)
    }
}
```

#### 3. ReceiveBroadcaster (수신 브로드캐스터)
```go
type VoiceReceiver struct {
    ModuleID   string
    RecvChan   chan *pb.OpusPacket
    Filter     func(*discordgo.Packet) bool  // 선택적 필터링
}

type ReceiveBroadcaster struct {
    voiceConnection *discordgo.VoiceConnection

    mu        sync.RWMutex
    receivers map[string]*VoiceReceiver  // module_id → receiver

    ctx       context.Context
    cancel    context.CancelFunc
}

func (b *ReceiveBroadcaster) Start() {
    go b.broadcastLoop()
}

func (b *ReceiveBroadcaster) broadcastLoop() {
    for {
        select {
        case <-b.ctx.Done():
            return

        case packet, ok := <-b.voiceConnection.OpusRecv:
            if !ok {
                return
            }

            // 모든 구독자에게 브로드캐스트
            b.mu.RLock()
            for moduleID, receiver := range b.receivers {
                // 필터 적용
                if receiver.Filter != nil && !receiver.Filter(packet) {
                    continue
                }

                // Non-blocking 전송
                select {
                case receiver.RecvChan <- convertToProto(packet):
                    // 성공
                default:
                    // 수신자 채널이 가득 참 (skip)
                    log.Debug("Receiver %s channel full, dropping packet", moduleID)
                }
            }
            b.mu.RUnlock()
        }
    }
}

func (b *ReceiveBroadcaster) Subscribe(moduleID string, recvChan chan *pb.OpusPacket, filter func(*discordgo.Packet) bool) {
    b.mu.Lock()
    defer b.mu.Unlock()

    b.receivers[moduleID] = &VoiceReceiver{
        ModuleID: moduleID,
        RecvChan: recvChan,
        Filter:   filter,
    }
}

func (b *ReceiveBroadcaster) Unsubscribe(moduleID string) {
    b.mu.Lock()
    defer b.mu.Unlock()

    delete(b.receivers, moduleID)
}
```

#### 4. VoiceHelper (통합 관리)
```go
type VoiceHelper struct {
    pb.UnimplementedVoiceStreamServer

    dgSession *discordgo.Session

    // Guild별 음성 세션 관리
    mu            sync.RWMutex
    voiceSessions map[string]*VoiceSession  // guild_id → session
}

type VoiceSession struct {
    GuildID    string
    ChannelID  string

    vc         *discordgo.VoiceConnection

    // 큐 시스템
    queue      *VoiceTaskQueue
    scheduler  *QueueScheduler
    broadcaster *ReceiveBroadcaster

    // 모듈 구독 관리
    mu         sync.RWMutex
    subscribers map[string]*ModuleSubscription  // module_id → subscription

    ctx        context.Context
    cancel     context.CancelFunc
}

type ModuleSubscription struct {
    ModuleID       string
    ConnectionID   string
    SubmitChan     chan *VoiceTask      // 모듈 → 큐
    RecvChan       chan *pb.OpusPacket  // 브로드캐스터 → 모듈
    Stream         pb.VoiceStream_VoiceOpusStreamServer
    CreatedAt      time.Time
    LastActivity   time.Time
}
```

---

## 📋 프로토콜 정의

### voice.proto

```protobuf
syntax = "proto3";
package discord_v1;
option go_package = "github.com/thirdscam/chatanium-flexmodule/proto/discord-v1";

import "common.proto";

service VoiceStream {
    // 음성 세션 조인 (Guild + Channel)
    rpc VoiceJoin(VoiceJoinRequest) returns (VoiceJoinResponse);

    // 양방향 음성 스트림 (큐 기반 송신 + 브로드캐스트 수신)
    rpc VoiceStream(stream VoicePacket) returns (stream VoicePacket);

    // 음성 세션 탈퇴
    rpc VoiceLeave(VoiceLeaveRequest) returns (common.Empty);

    // Speaking 상태 변경
    rpc VoiceSpeaking(VoiceSpeakingRequest) returns (common.Empty);

    // 큐 상태 조회 (디버깅용)
    rpc GetQueueStatus(QueueStatusRequest) returns (QueueStatusResponse);
}

// 조인 요청
message VoiceJoinRequest {
    string guild_id = 1;
    string channel_id = 2;
    bool mute = 3;
    bool deaf = 4;

    // 선택적 설정
    int32 priority = 5;        // 기본 우선순위 (0 = 보통)
    int32 buffer_size = 6;     // 수신 버퍼 크기 (기본 100)
}

// 조인 응답
message VoiceJoinResponse {
    string connection_id = 1;  // 구독 세션 ID
    string guild_id = 2;
    string channel_id = 3;
    bool ready = 4;

    // 큐 정보
    int32 queue_length = 5;    // 현재 큐 길이
    int32 active_subscribers = 6;  // 활성 구독자 수
}

// 음성 패킷
message VoicePacket {
    string connection_id = 1;
    bytes opus_data = 2;

    // 송신 시 설정 가능
    int32 priority = 3;        // 태스크 우선순위 (-10 ~ 10)
    int64 timeout_ms = 4;      // 타임아웃 (밀리초)

    // 수신 시 포함됨
    uint32 ssrc = 5;           // 발신자 SSRC
    uint32 sequence = 6;
    uint32 timestamp = 7;
    bytes type = 8;
}

// 탈퇴 요청
message VoiceLeaveRequest {
    string connection_id = 1;
}

// Speaking 요청
message VoiceSpeakingRequest {
    string connection_id = 1;
    bool speaking = 2;
}

// 큐 상태 요청
message QueueStatusRequest {
    string guild_id = 1;
}

// 큐 상태 응답
message QueueStatusResponse {
    int32 queue_length = 1;
    int32 active_subscribers = 2;
    repeated ModuleStats module_stats = 3;
}

message ModuleStats {
    string module_id = 1;
    uint64 tasks_submitted = 2;
    uint64 tasks_processed = 3;
    uint64 bytes_sent = 4;
    uint64 bytes_received = 5;
}
```

---

## 🔄 동작 흐름

### 1. 음성 세션 조인

```
Module A                              Runtime
   │                                     │
   ├─→ VoiceJoin(guild, channel) ──────→│
   │                                     ├─ 기존 VoiceSession 확인
   │                                     │  (없으면 새로 생성)
   │                                     │
   │                                     ├─ dgSession.ChannelVoiceJoin()
   │                                     ├─ VoiceTaskQueue 생성
   │                                     ├─ QueueScheduler 시작
   │                                     ├─ ReceiveBroadcaster 시작
   │                                     │
   │                                     ├─ ModuleSubscription 생성
   │                                     │  - SubmitChan 생성
   │                                     │  - RecvChan 생성
   │                                     │  - Broadcaster에 구독 등록
   │                                     │
   │   ←── VoiceJoinResponse ────────────┤
   │       (connection_id, queue_length) │
```

### 2. 음성 데이터 송신 (큐 기반)

```
Module A                              Runtime
   │                                     │
   ├─→ VoiceStream (bidirectional) ─────→│
   │                                     │
   ├─→ Send(VoicePacket)                 │
   │   - opus_data                       │
   │   - priority = 5 ──────────────────→│
   │                                     ├─ VoiceTask 생성
   │                                     │  - ModuleID, OpusData
   │                                     │  - Priority, Timeout
   │                                     │
   │                                     ├─ queue.Enqueue(task)
   │                                     │     │
   │                                     │     ↓
   │                                QueueScheduler
   │                                     │
   │                              [대기 중 태스크들]
   │                              - Module B task (priority 3)
   │                              - Module A task (priority 5) ← 먼저 처리
   │                              - Module C task (priority 0)
   │                                     │
   │                                     ↓
   │                              scheduler.Dequeue()
   │                                     │
   │                                     ↓
   │                            vc.OpusSend ← opus_data
   │                                     │
   │                                     ↓
   │                                  Discord
```

### 3. 음성 데이터 수신 (브로드캐스트)

```
Discord                               Runtime                  Modules
   │                                     │                        │
   ├─→ UDP packet ──────────────────────→│                        │
   │                                     │                        │
   │                              vc.OpusRecv                     │
   │                                     │                        │
   │                              ReceiveBroadcaster              │
   │                                     │                        │
   │                       ┌─────────────┼─────────────┐         │
   │                       ↓             ↓             ↓         │
   │               Module A.RecvChan  Module B  Module C         │
   │                       │                                     │
   │                       └─→ VoiceStream.Send(packet) ────────→│
   │                                                          Module A
```

---

## 🎯 구현 세부사항

### VoiceHelper.VoiceJoin

```go
func (h *VoiceHelper) VoiceJoin(ctx context.Context, req *pb.VoiceJoinRequest) (*pb.VoiceJoinResponse, error) {
    moduleID, err := getModuleIDFromContext(ctx)
    if err != nil {
        return nil, err
    }

    h.mu.Lock()
    defer h.mu.Unlock()

    // 1. VoiceSession 획득 또는 생성
    session, exists := h.voiceSessions[req.GuildId]
    if !exists || session.ChannelID != req.ChannelId {
        // 새 세션 생성
        if exists {
            // 다른 채널로 이동
            session.cancel()  // 기존 세션 종료
        }

        session, err = h.createVoiceSession(req.GuildId, req.ChannelId, req.Mute, req.Deaf)
        if err != nil {
            return nil, err
        }

        h.voiceSessions[req.GuildId] = session
    }

    // 2. 모듈 구독 생성
    subscription := &ModuleSubscription{
        ModuleID:     moduleID,
        ConnectionID: generateConnectionID(),
        SubmitChan:   make(chan *VoiceTask, 100),
        RecvChan:     make(chan *pb.OpusPacket, req.BufferSize),
        CreatedAt:    time.Now(),
        LastActivity: time.Now(),
    }

    // 3. 구독 등록
    session.mu.Lock()
    session.subscribers[moduleID] = subscription
    session.mu.Unlock()

    // 4. 브로드캐스터에 등록
    session.broadcaster.Subscribe(moduleID, subscription.RecvChan, nil)

    // 5. 태스크 제출 goroutine 시작
    go h.submitTaskLoop(session, subscription)

    return &pb.VoiceJoinResponse{
        ConnectionId:      subscription.ConnectionID,
        GuildId:           req.GuildId,
        ChannelId:         req.ChannelId,
        Ready:             session.vc.Ready,
        QueueLength:       int32(len(session.queue.tasks)),
        ActiveSubscribers: int32(len(session.subscribers)),
    }, nil
}

func (h *VoiceHelper) createVoiceSession(guildID, channelID string, mute, deaf bool) (*VoiceSession, error) {
    // VoiceConnection 생성
    vc, err := h.dgSession.ChannelVoiceJoin(guildID, channelID, mute, deaf)
    if err != nil {
        return nil, err
    }

    ctx, cancel := context.WithCancel(context.Background())

    // 큐 시스템 생성
    queue := NewVoiceTaskQueue()
    scheduler := NewQueueScheduler(queue, vc, ctx)
    broadcaster := NewReceiveBroadcaster(vc, ctx)

    session := &VoiceSession{
        GuildID:     guildID,
        ChannelID:   channelID,
        vc:          vc,
        queue:       queue,
        scheduler:   scheduler,
        broadcaster: broadcaster,
        subscribers: make(map[string]*ModuleSubscription),
        ctx:         ctx,
        cancel:      cancel,
    }

    // 시작
    scheduler.Start()
    broadcaster.Start()

    return session, nil
}
```

### VoiceHelper.VoiceStream

```go
func (h *VoiceHelper) VoiceStream(stream pb.VoiceStream_VoiceStreamServer) error {
    // 1. 모듈 ID 획득
    moduleID, err := getModuleIDFromContext(stream.Context())
    if err != nil {
        return err
    }

    // 2. 첫 패킷에서 connection_id 획득
    firstPacket, err := stream.Recv()
    if err != nil {
        return err
    }
    connectionID := firstPacket.ConnectionId

    // 3. 구독 찾기
    subscription, session, err := h.findSubscription(connectionID, moduleID)
    if err != nil {
        return err
    }

    // 4. 스트림 저장
    subscription.Stream = stream

    // 5. 송신 goroutine (모듈 → 큐)
    errChan := make(chan error, 2)
    go func() {
        // 첫 패킷 처리
        if len(firstPacket.OpusData) > 0 {
            task := &VoiceTask{
                TaskID:     generateTaskID(),
                ModuleID:   moduleID,
                OpusData:   firstPacket.OpusData,
                Priority:   int(firstPacket.Priority),
                SubmitTime: time.Now(),
                Timeout:    time.Duration(firstPacket.TimeoutMs) * time.Millisecond,
            }

            select {
            case subscription.SubmitChan <- task:
            case <-stream.Context().Done():
                errChan <- stream.Context().Err()
                return
            }
        }

        // 이후 패킷들
        for {
            packet, err := stream.Recv()
            if err != nil {
                errChan <- err
                return
            }

            task := &VoiceTask{
                TaskID:     generateTaskID(),
                ModuleID:   moduleID,
                OpusData:   packet.OpusData,
                Priority:   int(packet.Priority),
                SubmitTime: time.Now(),
                Timeout:    time.Duration(packet.TimeoutMs) * time.Millisecond,
            }

            select {
            case subscription.SubmitChan <- task:
                subscription.LastActivity = time.Now()

            case <-stream.Context().Done():
                errChan <- stream.Context().Err()
                return
            }
        }
    }()

    // 6. 수신 goroutine (브로드캐스터 → 모듈)
    go func() {
        for {
            select {
            case packet, ok := <-subscription.RecvChan:
                if !ok {
                    errChan <- nil
                    return
                }

                err := stream.Send(packet)
                if err != nil {
                    errChan <- err
                    return
                }

                subscription.LastActivity = time.Now()

            case <-stream.Context().Done():
                errChan <- stream.Context().Err()
                return
            }
        }
    }()

    // 7. 에러 대기
    err = <-errChan

    // 8. 정리
    h.cleanupSubscription(subscription, session)

    return err
}

func (h *VoiceHelper) submitTaskLoop(session *VoiceSession, subscription *ModuleSubscription) {
    for {
        select {
        case task := <-subscription.SubmitChan:
            // 큐에 제출
            err := session.queue.Enqueue(task)
            if err != nil {
                log.Error("Failed to enqueue task: %v", err)
            }

        case <-session.ctx.Done():
            return
        }
    }
}
```

### VoiceHelper.VoiceLeave

```go
func (h *VoiceHelper) VoiceLeave(ctx context.Context, req *pb.VoiceLeaveRequest) (*pb.Empty, error) {
    moduleID, err := getModuleIDFromContext(ctx)
    if err != nil {
        return nil, err
    }

    subscription, session, err := h.findSubscription(req.ConnectionId, moduleID)
    if err != nil {
        return nil, err
    }

    h.cleanupSubscription(subscription, session)

    // 마지막 구독자면 세션 종료
    session.mu.RLock()
    subscriberCount := len(session.subscribers)
    session.mu.RUnlock()

    if subscriberCount == 0 {
        h.mu.Lock()
        delete(h.voiceSessions, session.GuildID)
        h.mu.Unlock()

        session.cancel()
        session.vc.Disconnect()
    }

    return &pb.Empty{}, nil
}

func (h *VoiceHelper) cleanupSubscription(subscription *ModuleSubscription, session *VoiceSession) {
    // 1. 구독 제거
    session.mu.Lock()
    delete(session.subscribers, subscription.ModuleID)
    session.mu.Unlock()

    // 2. 브로드캐스터에서 제거
    session.broadcaster.Unsubscribe(subscription.ModuleID)

    // 3. 채널 닫기
    close(subscription.SubmitChan)
    close(subscription.RecvChan)
}
```

---

## 📊 큐 스케줄링 정책

### 정책 1: FIFO (First-In-First-Out)

**구현**:
```go
func (q *VoiceTaskQueue) Enqueue(task *VoiceTask) error {
    q.mu.Lock()
    defer q.mu.Unlock()

    q.tasks = append(q.tasks, task)  // 단순 append
    q.cond.Signal()
    return nil
}
```

**특징**:
- ✅ 공정성: 먼저 제출한 순서대로 처리
- ✅ 단순성: 구현이 매우 간단
- ❌ 우선순위 없음

### 정책 2: Priority Queue (우선순위 큐)

**구현**:
```go
type PriorityQueue []*VoiceTask

func (pq PriorityQueue) Len() int { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool {
    // 우선순위 높은 것이 먼저
    if pq[i].Priority != pq[j].Priority {
        return pq[i].Priority > pq[j].Priority
    }
    // 우선순위 같으면 먼저 제출된 것
    return pq[i].SubmitTime.Before(pq[j].SubmitTime)
}
func (pq PriorityQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

func (q *VoiceTaskQueue) Enqueue(task *VoiceTask) error {
    q.mu.Lock()
    defer q.mu.Unlock()

    heap.Push(&q.tasks, task)  // 힙에 삽입
    q.cond.Signal()
    return nil
}

func (q *VoiceTaskQueue) Dequeue() (*VoiceTask, error) {
    q.mu.Lock()
    defer q.mu.Unlock()

    for q.tasks.Len() == 0 && !q.closed {
        q.cond.Wait()
    }

    if q.closed {
        return nil, fmt.Errorf("queue closed")
    }

    task := heap.Pop(&q.tasks).(*VoiceTask)
    return task, nil
}
```

**특징**:
- ✅ 우선순위: 중요한 태스크 먼저 처리
- ✅ 유연성: 모듈별 우선순위 설정 가능
- ⚠️ 기아 가능성: 낮은 우선순위 태스크가 무한 대기 가능

### 정책 3: Weighted Round-Robin (가중 라운드로빈)

**구현**:
```go
type ModuleWeight struct {
    ModuleID string
    Weight   int  // 가중치 (높을수록 많은 슬롯)
}

type WeightedRRQueue struct {
    mu      sync.RWMutex
    queues  map[string][]*VoiceTask  // module_id → tasks
    weights map[string]int            // module_id → weight

    currentModule string
    currentSlot   int
    order         []string  // 모듈 순서
}

func (q *WeightedRRQueue) Dequeue() (*VoiceTask, error) {
    q.mu.Lock()
    defer q.mu.Unlock()

    // 현재 모듈에서 태스크 가져오기
    for {
        if len(q.order) == 0 {
            return nil, fmt.Errorf("no active modules")
        }

        moduleID := q.currentModule
        tasks := q.queues[moduleID]

        if len(tasks) > 0 {
            task := tasks[0]
            q.queues[moduleID] = tasks[1:]

            q.currentSlot--
            if q.currentSlot <= 0 {
                q.nextModule()
            }

            return task, nil
        }

        // 현재 모듈에 태스크 없으면 다음 모듈
        q.nextModule()
    }
}

func (q *WeightedRRQueue) nextModule() {
    // 다음 모듈로 이동
    for i, id := range q.order {
        if id == q.currentModule {
            nextIdx := (i + 1) % len(q.order)
            q.currentModule = q.order[nextIdx]
            q.currentSlot = q.weights[q.currentModule]
            return
        }
    }
}
```

**특징**:
- ✅ 공정성: 모든 모듈에 처리 기회 보장
- ✅ 가중치: 중요한 모듈에 더 많은 슬롯
- ⚠️ 복잡도: 구현이 복잡

---

## 🔧 설정 예제

### config/voice-queue.yaml

```yaml
queue_config:
  # 큐 정책: "fifo", "priority", "weighted_rr"
  policy: "priority"

  # 큐 크기 제한
  max_queue_size: 1000

  # 태스크 타임아웃 (기본값)
  default_timeout_ms: 5000

  # 우선순위 범위
  min_priority: -10
  max_priority: 10

  # 모듈별 가중치 (weighted_rr 정책용)
  module_weights:
    module-music: 3      # 음악 봇은 3배 슬롯
    module-tts: 2        # TTS는 2배
    module-recorder: 1   # 녹음은 1배

  # 브로드캐스트 설정
  broadcast_buffer_size: 100
  drop_on_full: true  # 버퍼 가득 차면 패킷 버림

# 모니터링
monitoring:
  enable_metrics: true
  metrics_interval_sec: 60

  alerts:
    queue_length_threshold: 500
    task_timeout_rate_threshold: 0.1  # 10% 이상
```

---

## 📈 성능 고려사항

### 1. 큐 크기 제한

```go
const MaxQueueSize = 1000

func (q *VoiceTaskQueue) Enqueue(task *VoiceTask) error {
    q.mu.Lock()
    defer q.mu.Unlock()

    if len(q.tasks) >= MaxQueueSize {
        return fmt.Errorf("queue full (max %d)", MaxQueueSize)
    }

    q.tasks = append(q.tasks, task)
    q.cond.Signal()
    return nil
}
```

### 2. 타임아웃 자동 정리

```go
func (s *QueueScheduler) cleanupExpiredTasks() {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            s.queue.mu.Lock()

            newTasks := make([]*VoiceTask, 0, len(s.queue.tasks))
            for _, task := range s.queue.tasks {
                if time.Since(task.SubmitTime) <= task.Timeout {
                    newTasks = append(newTasks, task)
                } else {
                    log.Debug("Task %s expired", task.TaskID)
                }
            }

            s.queue.tasks = newTasks
            s.queue.mu.Unlock()

        case <-s.ctx.Done():
            return
        }
    }
}
```

### 3. 메트릭 수집

```go
type QueueMetrics struct {
    mu                sync.RWMutex

    TasksEnqueued     uint64
    TasksProcessed    uint64
    TasksExpired      uint64
    TasksDropped      uint64

    AvgQueueLength    float64
    AvgWaitTime       time.Duration

    ModuleStats       map[string]*ModuleMetrics
}

type ModuleMetrics struct {
    TasksSubmitted  uint64
    TasksProcessed  uint64
    BytesSent       uint64
    BytesReceived   uint64
}

func (s *QueueScheduler) collectMetrics() {
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        s.metrics.mu.Lock()

        // 평균 큐 길이
        s.metrics.AvgQueueLength = float64(len(s.queue.tasks))

        // 로그 출력
        log.Info("Queue metrics: avg_length=%.2f, processed=%d, expired=%d",
            s.metrics.AvgQueueLength,
            s.metrics.TasksProcessed,
            s.metrics.TasksExpired)

        s.metrics.mu.Unlock()
    }
}
```

---

## 🧪 테스트 시나리오

### 시나리오 1: 다중 모듈 동시 송신

```go
func TestMultiModuleConcurrentSend(t *testing.T) {
    // 3개 모듈 조인
    clientA := NewVoiceClient(conn, "module-a")
    clientB := NewVoiceClient(conn, "module-b")
    clientC := NewVoiceClient(conn, "module-c")

    clientA.Join(ctx, "guild-1", "channel-1")
    clientB.Join(ctx, "guild-1", "channel-1")
    clientC.Join(ctx, "guild-1", "channel-1")

    // 동시 송신
    var wg sync.WaitGroup
    wg.Add(3)

    go func() {
        defer wg.Done()
        for i := 0; i < 100; i++ {
            clientA.Send(opusData, priority: 5)
            time.Sleep(20 * time.Millisecond)
        }
    }()

    go func() {
        defer wg.Done()
        for i := 0; i < 100; i++ {
            clientB.Send(opusData, priority: 3)
            time.Sleep(20 * time.Millisecond)
        }
    }()

    go func() {
        defer wg.Done()
        for i := 0; i < 100; i++ {
            clientC.Send(opusData, priority: 0)
            time.Sleep(20 * time.Millisecond)
        }
    }()

    wg.Wait()

    // 검증: 모든 태스크가 처리됨 (순서는 우선순위에 따름)
}
```

### 시나리오 2: 브로드캐스트 수신

```go
func TestBroadcastReceive(t *testing.T) {
    // 3개 모듈 조인
    clientA := NewVoiceClient(conn, "module-a")
    clientB := NewVoiceClient(conn, "module-b")
    clientC := NewVoiceClient(conn, "module-c")

    clientA.Join(ctx, "guild-1", "channel-1")
    clientB.Join(ctx, "guild-1", "channel-1")
    clientC.Join(ctx, "guild-1", "channel-1")

    // 수신 대기
    receivedA := 0
    receivedB := 0
    receivedC := 0

    go func() {
        for packet := range clientA.RecvChan {
            receivedA++
        }
    }()

    go func() {
        for packet := range clientB.RecvChan {
            receivedB++
        }
    }()

    go func() {
        for packet := range clientC.RecvChan {
            receivedC++
        }
    }()

    time.Sleep(5 * time.Second)

    // 검증: 모든 모듈이 같은 수의 패킷 수신
    assert.Equal(t, receivedA, receivedB)
    assert.Equal(t, receivedB, receivedC)
}
```

---

## ✅ 체크리스트

### Core Components
- [ ] VoiceTaskQueue 구현 (FIFO/Priority)
- [ ] QueueScheduler 구현
- [ ] ReceiveBroadcaster 구현
- [ ] VoiceSession 구조체
- [ ] ModuleSubscription 구조체

### VoiceHelper Methods
- [ ] VoiceJoin 구현
- [ ] VoiceStream 구현
- [ ] VoiceLeave 구현
- [ ] VoiceSpeaking 구현
- [ ] GetQueueStatus 구현

### Protocol
- [ ] voice.proto 작성
- [ ] VoicePacket에 priority, timeout 추가
- [ ] QueueStatus 메시지 정의
- [ ] make buf 실행

### Queue Policies
- [ ] FIFO 정책
- [ ] Priority Queue 정책
- [ ] Weighted Round-Robin 정책
- [ ] 정책 설정 로딩

### Performance
- [ ] 큐 크기 제한
- [ ] 타임아웃 정리
- [ ] 메트릭 수집
- [ ] 알림 시스템

### Testing
- [ ] 다중 모듈 송신 테스트
- [ ] 브로드캐스트 수신 테스트
- [ ] 우선순위 처리 테스트
- [ ] 타임아웃 테스트
- [ ] 부하 테스트

---

## 🎯 결론

**큐 기반 아키텍처의 장점**:

1. ✅ **단순성**: 멀티플렉싱/믹싱 불필요
2. ✅ **공정성**: 큐 정책으로 공정한 리소스 분배
3. ✅ **확장성**: 우선순위, 가중치 등 다양한 정책 적용 가능
4. ✅ **보안**: 런타임이 모든 연결 관리
5. ✅ **모니터링**: 큐 상태, 처리량 등 쉽게 추적

**사용 사례**:
- 음악 재생 봇 (우선순위: 중간)
- TTS 봇 (우선순위: 높음, 즉시 응답)
- 녹음 봇 (수신만, 송신 안 함)
- 효과음 봇 (우선순위: 높음, 짧은 사운드)

이 아키텍처로 `voice-stream-implementation-plan.md`를 수정하겠습니다!
