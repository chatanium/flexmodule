# Voice Player Module

Discord 음성 채널에서 음성 파일을 재생하는 FlexModule입니다.

## 기능

- Discord 메시지에 첨부된 음성 파일 자동 감지
- 지정된 음성 채널에 자동 접속
- 음성 파일 다운로드 및 DCA 형식 변환
- Discord Opus 스트리밍으로 재생
- 재생 통계 표시

## 지원하는 음성 파일 형식

- MP3 (`.mp3`)
- WAV (`.wav`)
- OGG (`.ogg`)
- FLAC (`.flac`)
- M4A (`.m4a`)
- AAC (`.aac`)
- Opus (`.opus`)
- WebM (`.webm`)

## 필수 요구사항

### 시스템 요구사항

**libopus 인코더가 포함된 FFmpeg가 필요합니다.** 모듈이 `shared/media/opus`를 통해 실행하며, 실행 중 설치하거나 갱신하지 않습니다.
```bash
# Ubuntu/Debian
sudo apt-get install ffmpeg

# CentOS/RHEL
sudo yum install ffmpeg

# macOS
brew install ffmpeg
```

### 환경 변수 설정

`private.env` 파일 수정:
```bash
DISCORD_TOKEN=your_bot_token
GUILD_ID=919823370600742942
PLUGIN_PATH=./bin/voice-player-module
```

## 빌드 방법

```bash
# voice-player-module 디렉토리에서
GOOS=linux GOARCH=amd64 go build -o ../bin/voice-player-module .

# 또는 프로젝트 루트에서
make build-voice-player  # (Makefile에 추가 필요)
```

## 사용 방법

### 1. 런타임 실행

```bash
# 프로젝트 루트에서
./bin/runtime
```

### 2. 음성 파일 재생

Discord의 **어떤 채널**에서든 음성 파일을 업로드하면:
1. 모듈이 음성 파일을 자동으로 감지
2. 지정된 길드/채널에 접속 (하드코딩됨):
   - Guild ID: `919823370600742942`
   - Channel ID: `1434561578560131245`
3. 음성 파일 다운로드 및 DCA 변환
4. 음성 채널에서 재생
5. 재생 완료 후 통계 표시

### 예시

1. Discord 채널에 MP3 파일 업로드
2. 봇이 자동으로 응답:
   ```
   🎵 음성 파일을 감지했습니다: `song.mp3`
   음성 채널에 접속하여 재생합니다...
   ```
3. 음성 채널에 접속하여 재생:
   ```
   ✅ 음성 채널에 접속했습니다. 재생을 시작합니다...
   ```
4. 재생 완료:
   ```
   ✅ 재생 완료!
   📊 전송: 250 패킷, 50000 바이트
   ```

## 코드 구조

### main.go
- 모듈 메인 로직
- Discord 메시지 이벤트 처리
- 음성 파일 감지 및 재생 orchestration

### audio.go
- `convertToDCA()`: 음성 파일을 DCA 형식으로 변환
- `playDCA()`: DCA 파일을 음성 채널에 재생
- `shared/media/opus` 사용: FFmpeg 인코딩과 크기가 제한된 Ogg/DCA 프레임 읽기. `github.com/jonas747/dca`에는 의존하지 않습니다.

### 주요 상수

```go
const (
    TARGET_GUILD_ID   = "919823370600742942"  // 재생할 길드 ID
    TARGET_CHANNEL_ID = "1434561578560131245" // 재생할 음성 채널 ID
    TEMP_DIR          = "./temp_audio"         // 임시 파일 저장 디렉토리
)
```

## 작동 원리

### 1. 음성 파일 감지
```go
func (vp *voicePlayer) OnCreateChatMessage(m *discordgo.Message) error {
    // 첨부 파일 확인
    for _, attachment := range m.Attachments {
        if isAudioFile(attachment.Filename) {
            // 음성 파일 재생 시작
            go vp.playAudioFile(attachment, m.ChannelID)
        }
    }
}
```

### 2. 음성 파일 변환
```go
// inputPath is the input path; outFile is an output writer owned by the caller.
// The caller closes outFile after Encode returns.
err := opus.Encode(ctx, inputPath, outFile, 128)
```

인코딩은 스테레오, 48kHz, 20ms Opus 패킷을 사용합니다. 출력은 기존 캐시와 같은 raw DCA 프레임이며, 읽기는 raw DCA와 DCA1 메타데이터 형식을 지원합니다. 취소 시 FFmpeg를 종료하고 회수하며, 변환 실패를 성공한 캐시로 남기지 않습니다.

### 3. 음성 재생
```go
// VoiceClient 사용
voiceClient := DiscordModule.NewVoiceClient(vp.voiceStream, "voice-player")
voiceClient.Join(ctx, TARGET_GUILD_ID, TARGET_CHANNEL_ID, false, false)

// DCA 프레임을 20ms 간격으로 전송
for {
    frame, err := decoder.OpusFrame(ctx)
    voiceClient.Send(frame, 5, 5000)
    time.Sleep(20 * time.Millisecond)
}
```

## 로그 예시

```json
{
  "@level": "info",
  "@message": "Audio file detected",
  "@timestamp": "2025-11-03T00:22:15.123456Z",
  "filename": "song.mp3",
  "url": "https://cdn.discordapp.com/attachments/...",
  "size": 3145728
}

{
  "@level": "info",
  "@message": "Audio file downloaded",
  "path": "./temp_audio/song.mp3"
}

{
  "@level": "info",
  "@message": "Successfully converted to DCA",
  "input": "./temp_audio/song.mp3",
  "output": "./temp_audio/song.mp3.dca"
}

{
  "@level": "info",
  "@message": "Joined voice channel",
  "guild": "919823370600742942",
  "channel": "1434561578560131245"
}

{
  "@level": "info",
  "@message": "Finished playing audio",
  "total_frames": 250
}

{
  "@level": "info",
  "@message": "Playback completed",
  "packets_sent": 250,
  "bytes_sent": 50000
}
```

## 주의사항

1. **FFmpeg 필수**: 모듈 이미지의 PATH에 libopus 지원 FFmpeg가 있어야 합니다. 일반 런타임 빌드는 FFmpeg나 native Go 인코딩 라이브러리를 요구하지 않습니다.
2. **임시 파일**: 재생 후 자동으로 삭제되지만, 오류 발생 시 `./temp_audio` 확인 필요
3. **동시 재생**: 현재는 하나의 파일만 재생 가능 (큐 시스템 미구현)
4. **하드코딩된 채널**: 길드/채널 ID가 코드에 하드코딩되어 있음

## 개선 가능 사항

### 우선순위 높음
- [ ] 길드/채널 ID를 환경 변수로 설정
- [ ] 재생 큐 시스템 구현 (여러 파일 순차 재생)
- [ ] 재생 중 취소 명령 추가

### 우선순위 중간
- [ ] 볼륨 조절 기능
- [ ] 재생 진행 상태 표시
- [ ] 슬래시 명령으로 재생 제어

### 우선순위 낮음
- [ ] 재생 목록 관리
- [ ] 반복 재생 기능
- [ ] 음성 효과 (속도, 피치 변경 등)

## 문제 해결

### ffmpeg not found
```bash
# ffmpeg 설치 확인
ffmpeg -version

# 설치되지 않았다면
sudo apt-get install ffmpeg
```

### DCA 변환 실패
- 음성 파일이 손상되었을 수 있음
- 지원하지 않는 코덱일 수 있음
- ffmpeg 로그 확인 (DEBUG 레벨 로그)

### 음성 채널 접속 실패
- 봇에게 음성 채널 접속 권한이 있는지 확인
- 길드/채널 ID가 올바른지 확인
- 봇이 해당 길드에 있는지 확인

## 라이센스

This module follows the same license as the main chatanium-flexmodule project.

## 의존성

- github.com/jonas747/dca - Discord Compatible Audio 인코딩
- github.com/bwmarrin/discordgo - Discord API 클라이언트
- github.com/hashicorp/go-plugin - 플러그인 시스템
- FFmpeg - 음성 파일 변환 (시스템 의존성)
