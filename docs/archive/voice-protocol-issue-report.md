# Voice/Protocol 이슈 리포트
## FlexModule 보이스 스트리밍 및 프로토콜 거친 부분 종합 분석

**작성일**: 2026-06-21
**분석 대상 커밋**: `27d56bb` (main)
**분석 범위**: 보이스 스트리밍 파이프라인 전체 + discord-v1 프로토콜 wiring
**활성 모듈**: `chatanium-musicbot` (`.env`의 `PLUGIN_PATH`)

> 이 문서는 *분석/리포트 전용*이며 코드는 아직 변경하지 않았습니다.
> 기존 설계 문서(`voice-concurrency-analysis.md`, `voice-stream-implementation-plan.md`)가
> "원래 계획의 문제"를 다뤘다면, 이 문서는 **현재 구현(merged)에 실제로 남아있는 문제**를 다룹니다.

---

## 0. 요약 (TL;DR)

현재 보이스 구조는 원래 계획(모듈별 `OpusSend` 덮어쓰기)의 race를 피하려고
**큐 + 스케줄러 + 브로드캐스터** 구조로 바뀌었다. 그러나 이 "작업 큐(task queue)" 모델이
*연속 오디오 스트림*과 맞지 않아 새로운 문제를 만들었다.

| ID | 심각도 | 한 줄 요약 | 증상 |
|----|--------|-----------|------|
| **A** | 🔴 Critical | 삼중 페이싱(triple pacing) | 재생이 실시간보다 점점 밀림 |
| **B** | 🔴 Critical | 무제한 큐 + timeout 드랍 | 수 분 뒤 오디오 완전 끊김, 메모리 누수 |
| **C** | 🔴 Critical | priority-task 모델이 스트림에 부적합 | 다중 송신 시 깨진 소리, 순서 흐트러짐 위험 |
| **D** | 🟠 High | 스트림 종료 시 close-on-send race | "send on closed channel" panic 가능 |
| **E** | 🟠 High | 닫힌 채널 nil 스핀 + goroutine 누수 | CPU 점유, 빈 프레임 무한 송신 |
| **F** | 🟠 High | 길드당 단일 VoiceConnection 미해결 | 다른 모듈이 조용히 채널 탈취 |
| **G** | 🟡 Medium | `module-id` 자가 신고 (신뢰 경계 없음) | 격리/권한이 보안 경계가 아님 |
| **H** | 🟡 Medium | MessageDelete가 잘못된 훅 호출 | 삭제 이벤트가 생성 훅으로 들어감 |
| **I** | 🟡 Low | 프로토콜/코드 설계 거친 부분 | 죽은 코드, 겸용 메시지, Leave() panic 등 |

---

## 1. 보이스 데이터 파이프라인 (현재)

한 오디오 프레임(20ms 분량 Opus)이 거치는 전체 경로:

```
[모듈: chatanium-musicbot]
 audio.go PlayMusic
   ├ decoder.OpusFrame()
   ├ vc.Send(frame, priority=5, timeout=5000)   ── SendChan(buffer 100)
   └ time.Sleep(20ms)                            ◀── 페이싱 #1 (소스)
        │
 voice_client.go sendLoop
   └ stream.Send(VoicePacket)                     ── gRPC bidi stream
        │                                            (yamux over go-plugin pipe)
─────────────────────────────────────────────────────────────────────
[런타임]
 voice_helper.go VoiceStream (recv goroutine)
   └ subscription.SubmitChan <- VoiceTask          (buffer 100)
        │
 voice_helper.go submitTaskLoop
   └ session.queue.Enqueue(task)                   ── VoiceTaskQueue
        │                                             (무제한, 매번 priority 정렬)
 voice_scheduler.go processLoop
   ├ queue.Dequeue()  (blocking)
   ├ if age > timeout: drop                        ◀── timeout 드랍 (이슈 B)
   ├ vc.OpusSend <- task.OpusData                  (opusSender가 읽을 때까지 ≤20ms 블록)
   └ time.Sleep(20ms)                              ◀── 페이싱 #2 (불필요/유해)
        │
 discordgo/voice.go opusSender
   └ 20ms ticker → UDP 송신                        ◀── 페이싱 #3 (유일하게 필요)
```

수신 경로(브로드캐스터)는 구현돼 있으나 현재 소비자가 없음:

```
discordgo OpusRecv ─ ReceiveBroadcaster.broadcastLoop
   └ 각 구독자 RecvChan에 non-blocking fan-out (가득 차면 드랍)
       └ VoiceStream (recv→send goroutine) → stream.Send → 모듈 RecvChan
```

---

## 2. Critical 이슈

### 🔴 A. 삼중 페이싱으로 인한 누적 지연

**증거**
- `chatanium-musicbot/audio.go:99-102`
  ```go
  vc.Send(frame, 5, 5000)
  frameCount++
  time.Sleep(20 * time.Millisecond)   // 소스가 이미 50fps로 공급
  ```
- `shared/discord-v1/runtime/voice_scheduler.go:80-96`
  ```go
  select {
  case s.vc.OpusSend <- task.OpusData:   // opusSender가 읽을 때까지 블록(≤20ms)
      ...
  }
  time.Sleep(20 * time.Millisecond)      // ◀ 그 다음 또 20ms 대기
  ```
- `discordgo/voice.go:733,768-775` — opusSender가 자체 20ms ticker로 페이싱(여기가 정답)
  ```go
  ticker := time.NewTicker(time.Millisecond * time.Duration(size/(rate/1000))) // 960/48 = 20ms
  ...
  select {
  case <-ticker.C:   // 정확히 20ms마다 UDP write
  }
  ```

**분석**
discordgo는 `OpusSend` 채널에 프레임을 넣어주기만 하면 자체 ticker로 정확히 20ms 간격에 송신한다.
그런데 스케줄러는 `OpusSend` 전송(이미 ≤20ms 블록) **후 추가로 20ms를 더 sleep**한다.
→ 스케줄러 소비 속도 ≈ 프레임당 40ms (≈25fps), 소스 생산 ≈ 50fps.
→ **생산 > 소비**이므로 큐가 계속 쌓이고 재생 시점이 실시간보다 점점 뒤처진다.

**증상**: 오디오가 "느리고 점점 밀리는" 현상.

**수정 방향**: 스케줄러의 `time.Sleep(20ms)` 제거. 페이싱은 discordgo opusSender에 일임.
스케줄러는 단순히 "큐 → OpusSend" 전달만 담당.

---

### 🔴 B. 무제한 큐 + timeout 드랍 → 일정 시간 후 완전 무음

**증거**
- `shared/discord-v1/runtime/voice_queue.go:43-64` — 상한 없음 + 매 Enqueue마다 전체 정렬
  ```go
  func (q *VoiceTaskQueue) Enqueue(task *VoiceTask) error {
      ...
      q.tasks = append(q.tasks, task)        // 상한 없음 → 무한 증가
      sort.Slice(q.tasks, func(i, j int) bool { ... })  // 매 프레임 O(n log n)
      ...
  }
  ```
- `shared/discord-v1/runtime/voice_scheduler.go:74-77` — 나이 초과 프레임 드랍
  ```go
  if task.Timeout > 0 && time.Since(task.SubmitTime) > task.Timeout {
      s.log.Warn("Task timed out", ...)
      continue   // ◀ 프레임 폐기
  }
  ```
- musicbot은 `timeout=5000ms` 사용 (`audio.go:99`)

**분석**
이슈 A의 드리프트로 큐가 무한정 증가한다(메모리 누수). 동시에 큐가 커질수록 매 프레임의
`sort.Slice` 비용이 O(n log n)으로 늘어 CPU가 증가한다. 그리고 backlog가 5초를 넘는 순간부터
큐에 들어온 프레임들의 나이가 timeout(5s)을 초과해 **대량으로 드랍**된다.

**증상**: "처음엔 그럭저럭 재생 → 수 분 후 점점 밀리다가 → 통째로 끊김". A와 B의 합작.

**수정 방향**: 큐를 per-connection FIFO ring buffer(상한 있음)로 교체. 우선순위 정렬 제거.
연속 스트림에는 "오래된 프레임 드랍"이 아니라 "버퍼 overrun 시 가장 오래된 것 1개만 drop"이 맞다.

---

### 🔴 C. priority-task 모델 자체가 연속 스트림에 부적합

**증거**
- `proto/discord-v1/voice.proto:54-56`
  ```proto
  int32 priority = 3;        // Task priority (-10 ~ 10)
  int64 timeout_ms = 4;      // Timeout in milliseconds
  ```
- `voice_queue.go:55-60` — priority로 재정렬

**분석**
`priority`/`timeout`은 "서로 독립적인 작업(task)" 개념이다. 그러나 오디오는 **순서가 고정된
연속 프레임 시퀀스**다.
- 우선순위 재정렬은 프레임 순서를 흐트러뜨릴 수 있다(현재는 동일 priority면 SubmitTime tie-break라
  단일 스트림에선 순서가 대체로 보존되지만, 설계상 위험이 내재).
- 여러 모듈이 같은 길드에 송신하면 큐는 프레임을 **단순 직렬화할 뿐 믹싱하지 않는다.**
  → A 모듈 프레임과 B 모듈 프레임이 번갈아 OpusSend로 나가 깨진 소리가 된다.

**수정 방향**: connection별 FIFO + 작은 jitter buffer 모델로 교체.
다중 모듈 동시 송신을 지원하려면 PCM 디코드 → 믹싱 → 재인코딩(고비용) 또는 "길드당 1 송신자" 정책 필요.
(설계 결정 필요 — 이슈 F와 연결)

---

## 3. High 이슈 (동시성/안전성)

### 🟠 D. 스트림 종료 시 close-on-send race (panic 가능)

**증거** — `shared/discord-v1/runtime/voice_helper.go`
- send goroutine이 `SubmitChan`에 write (`:186`, `:214`)
  ```go
  case subscription.SubmitChan <- task:
  ```
- 메인은 errChan에서 **하나만** 받고(`:258`) 곧바로 cleanup
  ```go
  err = <-errChan                       // :258
  h.cleanupSubscription(subscription, session)  // :261
  ```
- cleanup이 `SubmitChan`을 close (`:447`)
  ```go
  close(subscription.SubmitChan)   // :447
  close(subscription.RecvChan)     // :448
  ```

**분석**
send/recv 두 goroutine이 `errChan`(cap 2)에 결과를 보내는데 메인은 하나만 소비한다.
recv goroutine이 먼저 에러를 내면 메인이 cleanup → `SubmitChan` close. 이때 send goroutine이
아직 살아서 `subscription.SubmitChan <- task` 실행 중이면 **"send on closed channel" panic**.
두 goroutine의 종료를 보장하는 `WaitGroup`이 없다.

이것은 `voice-concurrency-analysis.md`의 "문제 4(연결 해제 타이밍)"가 큐 구조에서도 그대로 남은 것이다.

**수정 방향**: send/recv goroutine을 `sync.WaitGroup`으로 join한 뒤 채널 close.
또는 채널 close 책임자를 단일 goroutine으로 일원화(close는 송신자 측에서만).

---

### 🟠 E. 닫힌 채널 nil 스핀 + goroutine 누수

**증거** — `voice_helper.go:405-418`
```go
func (h *VoiceHelper) submitTaskLoop(...) {
    for {
        select {
        case task := <-subscription.SubmitChan:   // ◀ ok 미검사
            err := session.queue.Enqueue(task)     // 채널 닫히면 nil 무한 Enqueue
            ...
        case <-session.ctx.Done():
            return
        }
    }
}
```

**분석**
`SubmitChan`이 close되면 `<-subscription.SubmitChan`이 즉시 `nil, false`를 반환한다.
`ok`를 검사하지 않으므로 **무한 루프로 `Enqueue(nil)`**, 스케줄러는 `OpusSend <- nil`로 빈 프레임을
계속 송신한다. 탈출 조건은 `session.ctx.Done()`뿐인데, `session.ctx`는 **마지막 구독자가
`VoiceLeave`를 호출할 때만** 취소된다(`:289-296`). 스트림이 `Leave()` 없이 gRPC 에러로 죽으면
session이 살아있어 이 goroutine이 영구 누수된다.

**수정 방향**: `task, ok := <-subscription.SubmitChan; if !ok { return }`. 그리고 스트림 종료 시
구독 정리에서 관련 goroutine을 확실히 종료시키는 lifecycle 정리(이슈 D와 함께).

---

### 🟠 F. 길드당 단일 VoiceConnection 미해결 (조용한 채널 탈취)

**증거** — `voice_helper.go:88-103`
```go
session, exists := h.voiceSessions[req.GuildId]
if !exists || session.ChannelID != req.ChannelId {
    if exists {
        // 다른 채널로 이동 → 기존 세션 강제 종료
        session.cancel()       // ◀ 모듈 A가 재생 중이어도 끊김
    }
    session, err = h.createVoiceSession(...)
    h.voiceSessions[req.GuildId] = session
}
```

**분석**
같은 길드에서 다른 채널 Join 요청이 오면 기존 세션을 `cancel()`하고 채널을 옮긴다.
모듈 A가 채널 A에서 재생 중인데 모듈 B가 채널 B로 Join하면 **A가 말없이 끊긴다.**
이는 `voice-concurrency-analysis.md`의 "문제 1"이며, 문서 권장안인 "모듈별 Session 분리"는
적용되지 않았다(여전히 단일 `h.dgSession` 사용).

**수정 방향 (설계 결정 필요)**:
- (a) 분석 문서 권장안: 모듈별 독립 `discordgo.Session` → 길드 제약을 모듈 단위로 우회
- (b) 명시적 "길드당 1모듈" 정책: 이미 점유 중이면 `VOICE_ALREADY_CONNECTED` 에러 반환
- 현재의 "조용한 탈취"는 둘 중 어느 쪽도 아니라 가장 나쁨

---

## 4. Medium / Low 이슈

### 🟡 G. `module-id` 자가 신고 (신뢰 경계 없음)
- `chatanium-musicbot/main.go:476-482` — 모듈이 직접 메타데이터 설정
- `voice_helper.go:453-465` — 그 값을 신뢰
- 격리/통계/권한이 module-id 기반인데, 모듈이 아무 값이나 주장 가능 → 보안 경계가 아님.
  매니페스트 권한(`DISCORD_V1_CREATE_VOICE_STREAM`)도 런타임에서 실제 강제되지 않음.

### 🟡 H. MessageDelete가 잘못된 훅 호출
- `main.go:202-206` — 삭제 핸들러가 `OnCreateChatMessage`를 호출
  ```go
  dgSession.AddHandler(func(s *discordgo.Session, i *discordgo.MessageDelete) {
      ...
      hook.OnCreateChatMessage(i.Message)   // ◀ 삭제인데 생성 훅
  })
  ```
- 삭제 전용 훅이 인터페이스에 없음. (`interface.go:13-18` Hook에 OnDeleteMessage 부재)

### 🟡 I. 프로토콜/코드 설계 거친 부분
- **겸용 메시지**: `VoicePacket` 하나로 송신/수신 겸용 + `bytes type`(`voice.proto:62`) 같은
  모호한 필드. 송신 전용/수신 전용 분리가 깔끔.
- **죽은 코드**: `module/plugin.go:61-68`에서 메인 conn `c`로 만든 VoiceStream 클라이언트는 미사용.
  실제 경로는 `module/hook.go:51`의 broker conn. 혼란 유발.
- **핸드셰이크 취약**: 클라이언트가 첫 패킷에 `connection_id`를 다시 넣어 보내야 매칭됨
  (`voice_client.go:149-155` ↔ `voice_helper.go:152-156`). 빈 초기 패킷 의존.
- **`VoiceClient.Leave()` panic 위험**: `voice_client.go:88-101`이 `cancel()` 직후 공개 채널
  `SendChan`을 close → 모듈이 그 뒤 `Send()` 호출하면 "send on closed channel" panic.
- **수신 경로 무소비**: 브로드캐스터는 완성돼 있으나 소비자 모듈이 없음. STT 등에 쓰려면
  모듈 측 수신 API + VoiceState 이벤트 훅 정리 필요(현재는 `helper.Guild().VoiceStates` 폴링).

---

## 5. 권장 수정 순서

효과 대비 비용 순:

1. **즉효 (스무딩 핵심) — A·B**
   - `voice_scheduler.go`의 `time.Sleep(20ms)` 제거 (페이싱을 discordgo에 일임)
   - `VoiceTaskQueue`를 상한 있는 FIFO ring으로 교체, 우선순위 정렬 제거
   - → 재생 밀림·끊김의 직접 원인 대부분 해소

2. **구조 정리 — C·D·E**
   - priority-task 모델 → connection별 FIFO + 작은 jitter buffer로 교체
   - send/recv goroutine 정리에 `WaitGroup` + 단일 close 책임자 (race 제거)
   - `submitTaskLoop` `ok` 검사 + lifecycle 정리

3. **격리/안전/정책 — F·G·H·I**
   - 길드당 다중 모듈 정책 결정 (모듈별 Session 분리 vs 명시적 1모듈 거부)
   - `module-id` 신뢰 경계 정리, 권한 런타임 강제
   - 삭제 훅 분리, 죽은 코드/겸용 메시지 정리

---

## 6. 재현 시나리오 (수정 검증용)

### 시나리오 1: 장시간 재생 드리프트 (A·B)
1. musicbot으로 3분 이상 곡 재생
2. **기대**: 처음 수십 초는 정상 → 점점 재생이 실시간보다 밀림 → backlog가 5초 초과 후 끊김
3. **계측**: `GetQueueStatus`의 `queue_length`가 시간에 따라 단조 증가하면 A·B 확정

### 시나리오 2: 채널 탈취 (F)
1. 모듈 A가 길드 G 채널 1에서 재생
2. 모듈 B가 길드 G 채널 2로 Join
3. **현재 동작**: A가 조용히 끊기고 봇이 채널 2로 이동
4. **기대 동작(정책에 따라)**: A 유지 + B 거부, 또는 A·B 독립 세션

### 시나리오 3: 스트림 비정상 종료 race (D·E)
1. 재생 중 모듈 프로세스를 강제로 죽이거나 gRPC 스트림을 끊음 (`Leave()` 없이)
2. **기대 결함**: 런타임에서 "send on closed channel" panic 또는
   submitTaskLoop/scheduler goroutine 누수 + 빈 프레임 무한 송신
3. **계측**: 종료 후에도 `OpusSend`로 트래픽이 계속 나가거나 goroutine 수가 줄지 않으면 E 확정

---

## 7. 참고 파일 인덱스

| 영역 | 파일 |
|------|------|
| 프로토콜 정의 | `proto/discord-v1/voice.proto` |
| 런타임 서비스 | `shared/discord-v1/runtime/voice_helper.go` |
| 큐 | `shared/discord-v1/runtime/voice_queue.go` |
| 스케줄러(송신 페이싱) | `shared/discord-v1/runtime/voice_scheduler.go` |
| 수신 브로드캐스터 | `shared/discord-v1/runtime/voice_broadcaster.go` |
| 모듈 클라이언트 | `shared/discord-v1/module/voice_client.go` |
| 모듈 wiring | `shared/discord-v1/module/plugin.go`, `module/hook.go` |
| 런타임 wiring | `shared/discord-v1/runtime/plugin.go`, `main.go` |
| 소비자(음악봇) | `chatanium-musicbot/main.go`, `chatanium-musicbot/audio.go` |
| discordgo 송신부 | `discordgo/voice.go:703-800` (opusSender) |
| 선행 설계 문서 | [voice-concurrency-analysis.md](voice-concurrency-analysis.md), [voice-stream-implementation-plan.md](voice-stream-implementation-plan.md) |
</content>
</invoke>
