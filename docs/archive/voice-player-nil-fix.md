# Voice Player Nil Pointer Fix - 완료 보고

## 문제 분석

### 에러 상황
```
panic: runtime error: invalid memory address or nil pointer dereference
voice_client.go:69 in voiceClient.Join()
```

### 근본 원인
voice-player-module의 `voicePlayer` 구조체에서 `voiceStream` 필드가 nil로 남아있었음:

```go
type voicePlayer struct {
    helper      Discord.Helper        // ✅ OnInit에서 설정됨
    voiceStream pb.VoiceStreamClient  // ❌ 설정되지 않음 → nil
}
```

`playAudioFile()`에서 `NewVoiceClient(vp.voiceStream, ...)`를 호출할 때 nil이 전달되어 Join() 시 크래시 발생.

## 해결 방법

### 1. ModuleClients에 VoiceStream 추가
**파일**: `shared/discord-v1/module/plugin.go`

```go
type ModuleClients struct {
    Helper      shared.Helper
    Hook        shared.Hook
    VoiceStream proto.VoiceStreamClient  // ← 추가
}

func (m *ModuleClients) GetVoiceStream() proto.VoiceStreamClient {
    return m.VoiceStream
}
```

### 2. GRPCClient에서 VoiceStream 생성
**파일**: `shared/discord-v1/module/plugin.go`

```go
func (p *Plugin) GRPCClient(...) (interface{}, error) {
    helperClient := &HelperClientImpl{...}
    hookClient := &HookClient{...}
    voiceClient := proto.NewVoiceStreamClient(c)  // ← 추가

    return &ModuleClients{
        Helper:      helperClient,
        Hook:        hookClient,
        VoiceStream: voiceClient,  // ← 추가
    }, nil
}
```

### 3. OnInit에서 VoiceStream 전달
**파일**: `shared/discord-v1/module/hook.go`

```go
func (m *GRPCServer) OnInit(ctx context.Context, req *proto.InitRequest) (*proto.InitResponse, error) {
    // Broker를 통해 runtime에 연결
    conn, err := m.broker.Dial(req.HelperServerId)

    // Helper 클라이언트 생성
    helperClient := &HelperClientImpl{
        client: proto.NewHelperClient(conn),
        broker: m.broker,
    }

    // VoiceStream 클라이언트 생성 (같은 connection 사용)
    voiceStreamClient := proto.NewVoiceStreamClient(conn)

    // VoiceStreamAware 인터페이스 구현 여부 확인
    type VoiceStreamAware interface {
        SetVoiceStream(proto.VoiceStreamClient)
    }
    if vsAware, ok := m.Impl.(VoiceStreamAware); ok {
        vsAware.SetVoiceStream(voiceStreamClient)
    }

    // 기존 OnInit 호출
    resp := m.Impl.OnInit(helperClient)
    // ...
}
```

### 4. voicePlayer에 SetVoiceStream 구현
**파일**: `voice-player-module/main.go`

```go
// SetVoiceStream implements VoiceStreamAware interface
func (vp *voicePlayer) SetVoiceStream(stream pb.VoiceStreamClient) {
    vp.voiceStream = stream
    log.Debug("VoiceStream client set successfully")
}
```

## 작동 흐름

### Before (Broken):
```
1. main() creates voicePlayer{} with nil fields
2. OnInit() sets helper
3. voiceStream remains nil
4. playAudioFile() → NewVoiceClient(nil) → Join() → CRASH
```

### After (Fixed):
```
1. main() creates voicePlayer{} with nil fields
2. GRPCServer.OnInit:
   a. Dials broker → gets connection to runtime
   b. Creates HelperClient from connection
   c. Creates VoiceStreamClient from same connection  ← NEW
   d. Calls SetVoiceStream() on voicePlayer         ← NEW
   e. Calls OnInit() with helper
3. voicePlayer now has both helper AND voiceStream
4. playAudioFile() → NewVoiceClient(voiceStream) → Join() → SUCCESS
```

## 핵심 설계 원칙

### VoiceStreamAware 인터페이스
```go
type VoiceStreamAware interface {
    SetVoiceStream(proto.VoiceStreamClient)
}
```

- **선택적 구현**: 모듈이 VoiceStream이 필요한 경우에만 구현
- **자동 주입**: plugin 시스템이 OnInit 시 자동으로 감지하고 주입
- **Non-Breaking**: 기존 Hook 인터페이스 변경 없음

### Connection 재사용
Helper와 VoiceStream은 **같은 broker connection**을 사용:
- Runtime이 하나의 gRPC 서버에 Helper와 VoiceStream 서비스를 모두 등록
- Module이 broker를 통해 한 번만 연결
- 같은 connection으로 여러 서비스 클라이언트 생성

## 빌드 결과

```bash
$ make build
✅ bin/test-module
✅ bin/voice-player-module  (23MB)
✅ bin/runtime
```

모든 빌드 성공, 에러 없음.

## 테스트 준비

이제 다음 단계로 테스트 가능:

1. ✅ 빌드 완료
2. ✅ nil pointer 문제 해결
3. ⏳ 실제 음성 파일 업로드 테스트 필요

### 테스트 시나리오:
```
1. ./bin/runtime 실행
2. Discord 채널에 MP3 파일 업로드
3. 예상 결과:
   - "🎵 음성 파일을 감지했습니다" 메시지
   - 길드 919823370600742942의 채널 1434561578560131245에 접속
   - 음성 파일 재생
   - "✅ 재생 완료!" 메시지
```

## 추가 문제 발견 및 해결

### 문제 2: "unknown service discord_v1.VoiceStream"

첫 번째 테스트에서 새로운 에러 발견:
```
error="rpc error: code = Unimplemented desc = unknown service discord_v1.VoiceStream"
```

#### 원인
Runtime이 broker를 통해 Helper 서버를 생성할 때 VoiceStream 서비스를 등록하지 않음.

**Before:**
```go
go broker.AcceptAndServe(helperServerID, func(opts []grpc.ServerOption) *grpc.Server {
    s := grpc.NewServer(opts...)
    proto.RegisterHelperServer(s, &HelperServerImpl{...})
    // VoiceStream 서비스가 등록되지 않음!
    return s
})
```

**After:**
```go
go broker.AcceptAndServe(helperServerID, func(opts []grpc.ServerOption) *grpc.Server {
    s := grpc.NewServer(opts...)

    // Register Helper server
    proto.RegisterHelperServer(s, &HelperServerImpl{...})

    // Register VoiceStream server if available
    if p.VoiceHelper != nil {
        proto.RegisterVoiceStreamServer(s, p.VoiceHelper)
    }

    return s
})
```

#### 해결
**파일**: `shared/discord-v1/runtime/plugin.go`의 `GRPCClient` 메서드 수정

Module이 broker.Dial()로 연결하는 서버에 VoiceStream 서비스를 추가 등록.

## 요약

| 항목 | 상태 | 설명 |
|------|------|------|
| nil pointer 원인 파악 | ✅ | voiceStream이 설정되지 않음 |
| ModuleClients 수정 | ✅ | VoiceStream 필드 추가 |
| GRPCClient 수정 | ✅ | VoiceStreamClient 생성 |
| OnInit 수정 | ✅ | VoiceStreamAware 자동 주입 |
| voicePlayer 수정 | ✅ | SetVoiceStream() 구현 |
| VoiceStream 서비스 등록 | ✅ | broker 서버에 VoiceStream 추가 |
| 빌드 검증 | ✅ | 모든 빌드 성공 (2차) |
| 런타임 테스트 | ⏳ | 실제 재생 테스트 필요 |

### 문제 3: nil Author in OnCreateChatMessage

두 번째 테스트에서 새로운 크래시 발견:
```
panic: runtime error: invalid memory address or nil pointer dereference
main.go:101 in m.Author.Bot
```

#### 원인
Discord가 일부 이벤트(메시지 삭제 등)에서 Author가 nil인 메시지를 전달할 수 있음.

**Before:**
```go
func (vp *voicePlayer) OnCreateChatMessage(m *discordgo.Message) error {
    if m.Author.Bot {  // ← CRASH: Author가 nil일 수 있음
        return nil
    }
    // ...
}
```

**After:**
```go
func (vp *voicePlayer) OnCreateChatMessage(m *discordgo.Message) error {
    // Ignore messages with nil Author
    if m == nil || m.Author == nil {
        return nil
    }

    if m.Author.Bot {
        return nil
    }
    // ...
}
```

#### 해결
**파일**: `voice-player-module/main.go`의 `OnCreateChatMessage` 메서드 수정

nil 체크를 추가하여 방어적 프로그래밍.

**결론**:
1. nil pointer dereference 문제 해결 ✅
2. VoiceStream 서비스 등록 문제 해결 ✅
3. nil Author 방어 코드 추가 ✅
4. VoiceStream이 module에 올바르게 전달되고 서비스 사용 가능 ✅
