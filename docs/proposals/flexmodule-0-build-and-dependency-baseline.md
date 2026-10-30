# 빌드 및 의존성 기준선 정비 - Spec Proposal

| Item       | Detail                           |
|------------|----------------------------------|
| Author     | Seonguk Moon                     |
| Created    | 2026-10-29                       |
| Status     | **Draft** / In Review / Approved |
| Reviewers  |                                  |

---

## 1. Summary

FlexModule의 모든 Go 모듈이 같은 공개 DiscordGo 릴리스를 사용하도록 정리하고, 기존 감사에서 확인한 네 가지 변환기 컴파일 오류를 수정합니다. 빌드 도구와 Protobuf 생성기를 고정하고 기존 파일을 먼저 지우지 않는 생성 절차를 마련하여, 후속 프로토콜 및 런타임 변경의 재현 가능한 기준선을 만듭니다. 이 제안의 완료는 빌드 기반의 복구를 뜻하며, 변환 정확성, Discord API 동작 또는 OCI 격리의 완료를 뜻하지 않습니다.

## 2. Background & Motivation

### 기존 감사 기준선과 현재 의존성 계약

아래 오류와 구형 버전은 업그레이드 전 감사의 기록입니다. 당시 루트 모듈은 Go `1.24.0`, protobuf `v1.36.5`, gRPC `v1.71.0`, go-plugin `v1.6.3`을 선언했습니다. 소비 모듈의 DiscordGo `require v0.28.1`과 달리 실제 로컬 SDK 기준선은 `v0.29.0`, commit `6e8fa27c7917ea54d8b9ec26f126becae59058d2`였습니다. 이 기록을 현재 버전이나 최종 API 범위로 해석하지 않습니다.

2026-10-31 폴더 및 SDK 전환 뒤 현재 계약은 로컬 Go 모듈 세 개가 `github.com/chatanium/discordgo v0.30.0`을 직접 요구하며 DiscordGo `replace`를 사용하지 않는 것입니다. 공개 태그 commit은 `f2d1806d4599ff674163be703c0d62a23d4bb524`입니다. 이 fork는 upstream `f43dd94faaacd5b163e9e783f14b5bd8be639fc9`에 Gateway/context/ingress/observer 및 native DAVE 변경을 포함합니다. [SDK 해석기](../../tools/sdk.py)가 실제 버전, source와 checksum을 확인합니다. API 및 이벤트 범위의 근거는 [source inventory](../../tools/protocol/inventory/source-refresh-summary.json)와 P1/P3의 현행 분류이며, 의존성 메타데이터 확인은 빌드나 실제 호환성 검증의 성공을 뜻하지 않습니다.

기존 감사에서 루트와 `chatanium-musicbot` 빌드는 다음 네 오류로 실패했습니다. 이 문서를 작성하면서 빌드를 다시 실행하지 않았습니다.

| 위치 | 확인된 컴파일 오류 |
|---|---|
| `shared/discord-v1/convert/struct2buf/interactions.go:419` | 정의되지 않은 `MessageComponents` 호출 |
| 같은 파일 `:420` | 정의되지 않은 `MessageEmbeds` 호출 |
| 같은 파일 `:422` | `int64(s.Flags)`를 `*proto.MessageFlags` 필드에 대입 |
| `shared/discord-v1/convert/buf2struct/interactions.go:407` | `*proto.MessageFlags`를 직접 `discordgo.MessageFlags`로 변환 |

`voice-player-module`은 루트 모듈만 로컬로 대체하고 DiscordGo는 upstream에서 가져옵니다. Go의 `replace`는 의존 모듈에서 소비 모듈로 전파되지 않으므로 `Section`, `TextDisplay` 등 로컬 컴포넌트 심벌을 찾지 못하는 별도 실패가 발생했습니다.

기존 `make build`는 시작하자마자 `bin/*`를 삭제하고, 음성 예제 빌드는 주석 처리되어 있습니다. `make buf`도 생성된 `.pb.go` 파일을 먼저 삭제한 뒤 생성기를 실행합니다. 생성 실패 때 기존 산출물까지 사라질 수 있으며, `protoc-gen-go`는 버전이 고정되어 있지 않습니다. gRPC 생성기는 이미 `v1.3.0`으로 고정되어 있습니다.

기존 감사에서 Buf 스키마 빌드 및 두 실험용 helper 모듈의 빌드는 통과했지만, 전체 시스템의 성공을 입증하지는 않습니다. 특히 생성 파일이 컴파일된다는 사실과 변환기가 값을 보존한다는 사실은 별개입니다.

### 확정된 범위

FlexModule은 일반 메시지, 명령, 관리, 모더레이션 및 선택적 음성 기능을 제공하는 범용 모듈 런타임입니다. 일반 런타임 빌드가 음악 예제의 외부 실행 도구나 Discord 자격 증명을 요구해서는 안 됩니다. 프로토콜 변경은 런타임과 모든 모듈을 함께 다시 빌드하는 단절적 전환이며, 이전 버전 호환 계층을 유지하지 않습니다.

## 3. Goals & Non-Goals

### 3.1 Goals

- [ ] 저장소에 있는 모든 Go 모듈의 목적, 빌드 입력 및 로컬 의존성 경로를 기록하고 누락 없이 관리합니다.
- [ ] DiscordGo 소비 모듈이 모두 같은 공개 `github.com/chatanium/discordgo v0.30.0`을 직접 사용하도록 수정합니다.
- [ ] 확인된 네 컴파일 오류를 기존 변환 함수 재사용으로 수정하고, 오류를 감추기 위한 파일 제외나 stub을 도입하지 않습니다.
- [ ] Go의 최소 언어 버전과 실제 컴파일러 버전을 구분하고, 지원되는 Go 릴리스 및 Buf와 두 Protobuf 생성기의 정확한 실행 버전을 고정합니다.
- [ ] 빌드와 코드 생성 실패가 기존 정상 산출물 또는 관계없는 파일을 삭제하지 않도록 변경합니다.
- [ ] P1부터 P6까지 사용할 바이너리, 생성 파일 및 의존성 기록의 입력과 소유 범위를 명확히 합니다.

### 3.2 Non-Goals

- [ ] 의존성 업그레이드를 이유로 범용 런타임 요구나 기존 감사의 기능 및 보안 범위를 줄이지 않습니다. 기존의 일괄 최신화 제외 조건은 사용자의 전체 의존성 업그레이드 요구로 대체합니다.
- [ ] 빌드 복구를 이유로 변환 필드, 비활성 변환기 또는 Discord API 범위를 줄이지 않습니다.
- [ ] 필드 누락, enum 값 불일치, patch presence 또는 파일 전송 문제를 이 제안만으로 해결했다고 간주하지 않습니다. 전체 동작 수정은 [P2](flexmodule-2-domain-conversions.md) 및 [P3](flexmodule-3-discord-backend.md)의 범위입니다.
- [ ] Docker 이미지 구성, gVisor 실행 및 운영 호스트 준비를 구현하지 않습니다. 이미지와 실행 의존성은 [P4](flexmodule-4-runtime-and-isolation.md), 예제별 패키징 및 검증 설명은 [P6](flexmodule-6-examples-and-validation.md)에서 완성합니다.
- [ ] 비밀 값, 라이브 Discord 연결, 미디어 다운로드, 변환 또는 재생을 빌드의 선행 조건으로 만들지 않습니다.

## 4. Technical Design

### 4.1 Architecture Overview

빌드 경로는 도구 고정, 모듈 의존성 해석, 임시 출력 생성, 성공한 대상의 게시로 나눕니다. 프로토콜 생성은 바이너리 빌드와 분리하며, 평상시 빌드는 저장소에 포함된 생성 파일을 사용합니다. 빌드 과정에서 조용히 스키마를 다시 생성하거나 `go mod tidy`로 의존성 파일을 변경하지 않습니다.

기존 감사의 Go 모듈 여덟 개 목록은 역사적 기준선입니다. 현재 이 저장소의 Go 모듈은 다음 세 개입니다. 루트의 `go build ./...`는 중첩 모듈을 모두 빌드하는 명령이 아니므로 모듈별 실행 결과를 구분합니다.

| 모듈 경로 | 현재 빌드 입력과 산출물 |
|---|---|
| `.` | `cmd/flexmodule`, `cmd/flexmodule-supervisor`, `examples/general-bot`, `tests/integration`을 포함합니다. |
| `modules/music-bot` | 선택적 미디어 모듈이며 출력 이름은 `bin/chatanium-musicbot`입니다. 루트 모듈의 `../..` 대체만 유지합니다. |
| `modules/audio-player` | 선택적 미디어 모듈이며 출력 이름은 `bin/voice-player-module`입니다. 루트 모듈의 `../..` 대체만 유지합니다. |

세 모듈은 같은 공개 SDK를 직접 요구합니다. SDK 자체와 `examples/linked_roles`, `examples/voice_receive` 중첩 모듈 및 해당 CI는 [별도 SDK 저장소](https://github.com/chatanium/discordgo/tree/v0.30.0)가 관리합니다. 로컬 `discordgo` 소스와 실험 helper 두 모듈은 제거되었습니다. 기존 SDK 중첩 Git 이력은 ignored `.tools/retired-discordgo-git-3158838`에 보존하며 빌드 입력으로 사용하지 않습니다.

### 4.2 Data Model Changes

애플리케이션 도메인 데이터 및 DB 스키마 변경은 없습니다. P0에서는 v1 Protobuf 스키마를 동작 수정 목적으로 재설계하지 않습니다. P1의 `core-v2` 및 `discord-v2` 전환 뒤에는 이 제안의 같은 생성 절차로 새 스키마를 생성합니다.

빌드 설정은 기존 `Makefile`과 생성기 설정을 확장하는 수준으로 유지합니다. 별도의 범용 provenance 수집기, dirty-revision 기록, 산출물 checksum 체계 또는 여러 registry 파일을 추가하지 않습니다.

| 기존 설정에 명시할 항목 | 필수 내용 |
|---|---|
| 도구 버전 | 실제 Go 컴파일러 버전과 Buf 및 두 생성기의 정확한 버전. Go 최소 언어 버전과 별도로 관리합니다. |
| 빌드 대상 | 모듈 경로, 출력 이름, 로컬 `replace`, 선택적 예제 또는 폐기 대상 여부를 기존 Make 대상 및 문서에 명시합니다. |
| 관리 출력 목록 | `Makefile`에 빌드 바이너리와 생성 파일의 상대 경로를 명시하여 해당 출력만 교체하거나 제거합니다. |

관리 출력은 저장소 루트 밖의 경로, 심볼릭 링크를 통한 이탈 또는 다른 작업의 파일을 지정할 수 없습니다. staging과 기존 파일 보관은 실패 복구를 위한 임시 작업이며 별도 영속 manifest 서비스가 아닙니다.

### 4.3 Core Logic

#### 도구 버전 기준선

유지 대상 모듈의 최소 언어 버전과 실제 실행 도구는 다음 기준으로 고정합니다. 구형 버전은 위 감사 기록에만 남기고 현재 도구나 라이브러리 pin으로 사용하지 않습니다.

| 항목 | 기준 | 근거 및 처리 |
|---|---|---|
| Go 언어 및 최소 버전 | `go 1.27.0` | `go-ytdlp v1.5.6`의 Go 1.27 요구에 맞춰 여섯 유지 모듈의 floor를 통일합니다. gRPC `v1.85.0`은 Go 1.26.0 이상을 요구하므로 기존 1.24 floor는 유지할 수 없습니다. |
| 실제 Go 컴파일러 | `go1.27.3` | 최소 버전과 별도로 고정한 실제 patch 릴리스입니다. |
| Buf CLI | `1.75.0` | Go 소스 빌드 floor는 1.26.7입니다. 설치된 버전이나 생성 성공을 이 문서에서 새로 검증한 것은 아닙니다. |
| `protoc-gen-go` 및 protobuf 런타임 | `v1.36.12` | 생성기와 애플리케이션 런타임을 같은 릴리스로 고정합니다. |
| `protoc-gen-go-grpc` | `v1.6.2` | generic stream 인터페이스에 맞게 호출자를 이관하고 `require_unimplemented_servers=false`는 유지합니다. |
| gRPC 및 go-plugin | `v1.85.0`, `v1.8.0` | 각각 현재 고정 릴리스를 사용합니다. go-plugin의 AutoMTLS와 multiplex는 1.8에서 새로 생긴 기능으로 설명하지 않습니다. |
| 기타 직접 의존성 | `go-ytdlp v1.5.6`, `oauth2 v0.37.0`, `websocket v1.5.3`, `x/crypto v0.58.0` | 미디어 wrapper의 실제 API와 SDK 예제도 함께 이관합니다. UUID `v1.6.0`, hclog `v1.6.3`, godotenv `v1.5.1`은 이미 최신 안정 릴리스이므로 유지합니다. |

최신 DCA revision도 `DiscordGo.OpusSend`에 직접 의존하므로 버전만 올려 P5의 공개 채널 제거와 공존시킬 수 없습니다. 모듈 호출자와 의존성에서 DCA를 제거하고 실제 ffmpeg 인코딩과 bounded Opus packet 처리를 모듈 측 `shared/media/opus`로 이관합니다. 코어에 인코더나 host installer를 추가하지 않습니다. 최신 go-plugin이 요구하는 `github.com/golang/protobuf v1.5.4`의 간접 의존성은 로컬 생성 코드를 새 protobuf로 옮겼다는 이유만으로 제거되었다고 주장하지 않습니다.

생성기는 정확한 Go 모듈 버전으로 만든 저장소 전용 도구 디렉터리의 실행 파일을 사용합니다. Buf 설정은 현재의 원격 unpinned Go 플러그인 대신 이 고정된 로컬 생성기를 가리킵니다. `paths=source_relative`와 `require_unimplemented_servers=false`는 유지합니다. PATH에서 우연히 발견한 다른 생성기, `@latest`, 버전 없는 원격 플러그인을 허용하지 않습니다. 모듈 및 도구 배포의 기존 무결성 확인 절차를 사용하며 별도의 범용 checksum 기록 체계는 만들지 않습니다.

동일한 도구, 입력, 환경 및 빌드 플래그에 대한 반복 출력을 재현성 기준으로 삼습니다. 유지 대상 중 CGO가 필요한 모듈은 필요한 C 컴파일러와 시스템 라이브러리를 빌드 전제조건에 명시합니다. 서로 다른 호스트의 임의 C 도구 체인에서 바이너리가 byte-for-byte 같다고 약속하지 않습니다.

#### 네 컴파일 오류의 제한된 수정

1. `InteractionResponseData`의 컴포넌트 목록은 기존 단일 `MessageComponent` 변환 함수를 사용하여 입력 순서대로 변환합니다.
2. embed 목록은 기존 `MessageEmbed`를 재사용합니다. 같은 역할의 두 번째 변환기나 빈 결과를 돌려주는 함수를 추가하지 않습니다.
3. 정방향 flags는 현재 v1 생성 필드의 실제 포인터 타입에 맞는 값을 만듭니다.
4. 역방향 flags는 nil 여부를 처리한 뒤 값을 읽습니다. nil 포인터를 직접 형변환하지 않습니다.

이 단계의 flags 수정은 Go 타입 불일치를 제거하는 작업입니다. 현재 스키마의 ordinal enum과 DiscordGo의 비트마스크가 의미상 같다는 보장은 하지 않습니다. 모든 값, 조합, presence 및 v2 wire 타입의 정확성은 P1과 P2의 완료 기준을 따릅니다. 마찬가지로 파일 Reader, audit-log 동적 값 및 patch의 의미를 P0의 컴파일 성공으로 대체하지 않습니다.

#### 비파괴 코드 생성

1. 입력 스키마, 생성 설정, `Makefile`에 고정한 도구 버전 및 관리 생성 파일 목록을 읽습니다.
2. 현재 스키마를 기준으로 별도 임시 디렉터리에 두 생성기의 전체 결과를 만듭니다. 이 단계에서 기존 `.pb.go` 파일을 지우지 않습니다.
3. 생성기 실패, 출력 누락, 허용 범위 밖 경로 또는 동일 경로 충돌이 있으면 임시 결과만 폐기하고 기존 파일을 유지합니다.
4. 모든 출력이 준비되면 관리 대상 파일만 교체합니다. 이전 소유 목록에 있었고 새 결과에는 없는 파일만 obsolete 파일로 제거합니다. 작업과 관계없는 `.pb.go` 파일을 glob으로 삭제하지 않습니다.
5. 게시 중 실패하면 보관한 기존 관리 파일을 복구하고 실패를 반환합니다. 여러 파일의 교체를 파일시스템의 단일 원자 연산인 것처럼 설명하지 않습니다.
6. 비교 전용 모드에서는 같은 임시 결과를 현재 생성 파일과 비교하고, 변경 여부만 반환합니다. 작업 트리를 변경하지 않습니다.

동시에 두 생성 작업이 같은 출력 집합을 게시하지 않도록 직렬화합니다. 생성에 성공했다고 바이너리를 자동 실행하지 않습니다.

#### 비파괴 바이너리 빌드

각 대상을 임시 파일로 빌드한 뒤 성공한 출력만 최종 이름으로 교체합니다. 대상 실패 시 그 대상의 기존 바이너리를 유지하지만, 명령 결과는 실패로 기록하여 오래된 바이너리를 새 빌드 성공으로 표시하지 않습니다. `bin/*` 전체 삭제는 없애고 명시적인 정리 작업만 이 빌드가 소유한 출력에 적용합니다.

일반 `make build`는 `bin/runtime`, `bin/flexmodule-supervisor`와 비미디어 `bin/test-module`을 대상으로 하며 DAVE, CGO bridge 또는 native crypto 라이브러리를 요구하지 않습니다. 일반 런타임은 voice backend 기능을 협상 결과에 제공하지 않습니다. required voice capability가 있는 모듈은 활성화를 거절하고 optional voice capability는 미허용으로 전달하며, 실제로 동작하지 않는 voice handler를 등록하지 않습니다.

선택적 `make build-voice`는 P5의 현재 Discord voice protocol 및 공식 libdave CGO bridge를 포함한 완전한 voice-enabled runtime bundle을 만듭니다. P5에서 고정한 native toolchain과 runtime library 집합을 명시적으로 준비해야 하며 일반 빌드 또는 공통 offline 검증에서 C++ 도구나 라이브러리를 암묵적으로 내려받지 않습니다. `make build-media`는 모듈 측 음악 및 음성 예제의 Go 바이너리만 만들며 호스트 voice runtime을 대신하지 않습니다.

`build-voice`의 출력은 `bin/voice-runtime/runtime`과 `bin/voice-runtime/lib/`입니다. `CGO_ENABLED=1` 및 `-tags voice`로 선택한 구현에만 DAVE bridge를 포함하며, 필요한 native runtime library와 라이선스의 정확한 집합은 P5의 고정 의존성 계약을 따릅니다. 호스트 설치 프로그램이나 C++ toolchain 자체를 runtime bundle에 넣지 않습니다. 전체 bundle을 staging에서 완성한 뒤 게시하고 실패 시 기존 bundle을 보존합니다. native archive 무결성, ABI/header export 및 대상 Linux 배포판의 loader 호환성 검증 전에는 동작하는 voice 산출물이라고 표시하지 않습니다.

`make build-all`은 로컬 Go 모듈 세 개와 일반/미디어 산출물을 포함하되 SDK 저장소의 예제 및 native voice target은 포함하지 않습니다. `make check-offline`은 같은 세 모듈의 유한 ordinary 시험과 Buf 생성 비교를 수행합니다. voice-enabled 산출물과 실제 backend 기능 협상은 P5/P6의 명시적 검증을 거쳐야 합니다. `private.env`나 Discord 토큰을 빌드 입력으로 읽지 않으며 `make run`만 별도로 실행 동작을 가집니다. OCI 이미지와 모듈별 ffmpeg 또는 다운로드 도구 의존성은 P4/P6에서 정의합니다.

Native lock과 libdave patch는 공개 SDK의 [native-lock.json](https://github.com/chatanium/discordgo/blob/v0.30.0/tools/native/native-lock.json) 및 [libdave.patch](https://github.com/chatanium/discordgo/blob/v0.30.0/tools/native/libdave.patch)에 있습니다. 이 저장소는 EL9 builder와 packaging을 유지하며 `tools/sdk.py`로 해석한 source를 `/sdk`에 mount합니다. `make build-native`는 native 검사만 수행하며 application Go를 컴파일하지 않습니다.

일반 모듈 및 sandbox fixture 이미지에는 digest로 고정한 distro 이미지의 CA bundle을 포함합니다. 읽기 전용 파일을 복사할 때 상위 디렉터리까지 `0444`로 만들지 않으며, 비루트 프로세스가 모든 상위 디렉터리를 통과할 수 있는 실행 권한을 보존합니다. 공개 모듈 이미지의 환경 설정에는 예약된 `TMPDIR`를 선언하지 않습니다. 호스트가 실제 quota mount인 `/var/lib/flexmodule/scratch`를 `TMPDIR`로 제공하며, media 이미지의 `HOME`과 `DENO_DIR`도 이 mount를 사용합니다. 이미지 빌드 단계에서만 쓰는 `TMPDIR`는 실행 시 scratch와 구분하고 최종 이미지 환경에 남기지 않습니다. 별도 호환 mount는 추가하지 않습니다.

## 5. API Design

### 5-1. New / Modified

REST API 변경은 없습니다. 외부 인터페이스는 Make 대상 및 현재 변환 함수의 제한된 수정입니다. 다음 Make 대상은 제안하는 최종 명령 계약입니다.

| 명령 | 입력 및 산출물 | 실패 및 부작용 계약 |
|---|---|---|
| `make tools` | 기존 빌드 설정에 고정한 Buf와 두 생성기를 전용 디렉터리에 준비합니다. | 지정 버전 불일치 또는 설치 실패 시 실패하며 전역 PATH의 도구를 교체하지 않습니다. |
| `make build` | voice 기능이 없는 `bin/runtime`, `bin/flexmodule-supervisor`와 `bin/test-module`을 만듭니다. | DAVE/native crypto/CGO bridge, 생성, 라이브 접속 및 미디어 도구 설치를 요구하거나 실행하지 않습니다. |
| `make build-voice` | `bin/voice-runtime/runtime`과 `bin/voice-runtime/lib/`에 P5의 voice-enabled runtime 및 필요한 고정 native runtime libraries를 만듭니다. | 명시된 native toolchain, `CGO_ENABLED=1`, `-tags voice`가 필요합니다. 일반 runtime이나 fake handler로 대체하지 않으며 별도의 native 연결, voice 기능 협상 및 실제 암호화 통신 검증이 필요합니다. |
| `make build-media` | `chatanium-musicbot`과 `voice-player-module`의 Go 바이너리를 명시적으로 빌드합니다. | ffmpeg/yt-dlp 실행, 다운로드, Discord 접속 또는 미디어 이미지 생성을 자동 수행하지 않습니다. |
| `make build-all` | 로컬 Go 모듈 세 개를 각각 빌드합니다. SDK 저장소의 모듈 및 native voice target은 포함하지 않습니다. | 포함된 유지 대상 중 하나라도 실패하면 전체 결과는 실패입니다. native voice 성공을 함께 주장하지 않으며 C++ 다운로드를 암묵적으로 실행하지 않습니다. |
| `make buf` | 고정된 도구로 Protobuf 파일을 임시 생성 후 게시합니다. | 생성 실패 전후에 기존 관리 파일을 잃지 않습니다. |
| `make buf-check` | 같은 생성 결과와 저장된 파일을 비교합니다. | 차이가 있으면 실패하며 생성 파일을 수정하지 않습니다. |
| `make clean` | 소유 목록에 기록된 빌드 출력만 제거합니다. | 소스, 미등록 파일, 모듈의 영속 데이터 및 비밀 파일은 삭제하지 않습니다. |
| `make run` | 일반 빌드 성공 후 런타임을 실행합니다. | 빌드 실패 시 기존 바이너리를 대신 실행하지 않습니다. 실행 설정과 비밀 관리 계약은 P4를 따릅니다. |

공개 변환 함수의 v1 임시 시그니처는 아래와 같이 유지합니다. 별도의 build 전용 Go 프레임워크는 도입하지 않습니다.

```go
// InteractionResponseData converts the fields supported by the current v1
// response converter. It returns nil for nil input and uses the existing
// component and embed converters. Compile success does not certify that the
// v1 schema preserves every Discord field; full conversion is specified in P2.
func InteractionResponseData(s *discordgo.InteractionResponseData) *discordv1.InteractionResponseData

// InteractionResponseData reconstructs the currently supported v1 response
// fields. It reads the optional flags value without casting its pointer and
// treats an absent flags field as the SDK zero value. The v2 presence and
// bitmask contract replaces this transitional signature at coordinated cutover.
func InteractionResponseData(p *discordv1.InteractionResponseData) *discordgo.InteractionResponseData
```

두 시그니처는 각각 기존 `struct2buf` 및 `buf2struct` 패키지에 있습니다. 의사코드는 다음과 같습니다.

```text
GENERATE(mode):
    verify exact tool versions against the existing build configuration
    acquire exclusive publication ownership
    create isolated staging directory
    generate all declared schema outputs into staging
    if generation or output ownership validation fails:
        discard staging; return failure without changing existing outputs
    if mode is CHECK:
        compare staged outputs and owned existing outputs; return comparison result
    save existing owned outputs needed for rollback
    publish staged outputs; remove only obsolete owned outputs
    if publication fails: restore owned outputs; return failure
    discard temporary staging and backup files; return success

BUILD(target):
    resolve the declared target module and explicit local replacements
    invoke go build with read-only module files into a temporary target file
    if build fails: keep the existing target; return failure
    replace only this target; return success
```

P1 전환 시 런타임, SDK 및 모든 예제의 import를 함께 옮기며 v1 변환 경로와 호환 alias를 제거합니다. P0의 임시 수정만 반영한 빌드를 배포 가능한 v2 런타임으로 표시하지 않습니다.

### 5-2. Error Handling

| 오류 | 처리 |
|---|---|
| 도구 누락 또는 지정 버전 불일치 | 실제 버전과 필요한 버전을 출력하고 nonzero exit로 종료합니다. 다른 버전으로 자동 진행하지 않습니다. |
| 로컬 SDK 경로 누락 또는 upstream 해석 | 실패한 모듈 경로와 기대한 로컬 대체를 출력합니다. upstream으로 fallback하지 않습니다. |
| 의존성 내려받기 또는 무결성 확인 실패 | 실패를 전파하고 `go.mod`나 `go.sum`을 자동 수정하지 않습니다. |
| 컴파일 실패 | 모듈, 대상 및 원본 컴파일러 진단을 보존합니다. 실패한 패키지를 제외한 성공으로 대체하지 않습니다. |
| 스키마 또는 생성기 실패 | 기존 생성 파일을 유지하고 임시 출력만 정리합니다. |
| 출력 경로 충돌 또는 게시 실패 | 게시를 중단하고 소유 파일을 복구합니다. 복구 자체의 실패도 원래 오류와 함께 보고합니다. |
| 생성 결과 차이 | `buf-check`를 실패시켜 코드와 생성 파일의 불일치를 드러냅니다. 자동으로 변경을 숨기지 않습니다. |
| 취소 또는 중단 | 진행 중인 자식 프로세스를 종료하고 임시 자원만 정리합니다. 이미 유효한 다른 대상의 산출물을 삭제하지 않습니다. |

이 명령들은 REST API가 아니므로 HTTP 상태 코드를 정의하지 않습니다. build 성공, schema 생성 성공, converter 동작 성공 및 라이브 통합 성공은 별도 결과로 기록합니다.

## 6. Implementation Plan

### 6-1. Milestones

아래 단계 표는 2026-10-29 제안 당시의 계획을 보존합니다. 로컬 SDK 대체 및 여덟 모듈 표기는 당시 입력이며, 2026-10-31 전환 뒤 실제 모듈 목록과 공개 SDK 계약은 4.1절을 따릅니다.

각 단계는 입력, 산출물 및 완료 조건을 자체적으로 설명하지만 모두 독립 배포 단계는 아닙니다. 마지막 단계는 앞선 빌드 및 생성 변경의 결과에 의존합니다.

| Phase | Task | Estimated Duration | Owner |
|-------|------|--------------------|-------|
| Phase 1 | 여덟 모듈의 현황을 입력으로 기존 Make 대상과 도구 버전을 명시하고 누락된 로컬 SDK 대체를 수정합니다. Go 최소 언어 버전은 유지하되 실제 컴파일러는 지원 릴리스의 정확한 patch 버전으로 고정합니다. 완료 조건은 DiscordGo 소비 모듈마다 같은 로컬 소스가 해석되며 비소비 helper 모듈에 새 Discord 의존성이 생기지 않는 것입니다. | 미정 | 미정 |
| Phase 2 | 현재 네 컴파일 진단을 입력으로 기존 단일 변환기 호출 및 flags 포인터 처리를 수정합니다. 완료 조건은 전체 파일을 포함한 루트, 음악 예제 및 음성 예제 빌드에서 해당 오류가 사라지고 제외 파일이나 stub이 추가되지 않는 것입니다. 전체 변환 동작의 완료 조건은 P2로 남깁니다. | 미정 | 미정 |
| Phase 3 | 고정 도구와 모듈 목록을 사용하여 staging, 관리 파일 게시, 비교 전용 생성 및 대상별 바이너리 교체를 구현합니다. 완료 조건은 생성 실패 주입 및 빌드 실패에서 기존 파일이 보존되고, 같은 입력의 재생성 차이가 없으며, 전체 모듈 대상이 빠짐없이 기록되는 것입니다. | 미정 | 미정 |
| Phase 4 | 일반 빌드, 모듈 측 미디어 빌드 및 선택적 host voice 빌드의 책임을 분리하고 README, 빌드 명령 설명 및 변경 기록을 갱신합니다. P1과 P4/P6에 고정 도구 버전 및 명시적인 빌드 대상을 인계합니다. 일반 기준선은 native voice 의존성 없이 완성하며, `build-voice` bundle과 feature negotiation의 통합 완료는 P5 구현 및 P6의 명시적 검증에 의존합니다. | 미정 | 미정 |

위 완료 조건은 향후 구현 시 수행할 검증 항목입니다. 이 제안서 작성에서 빌드, 테스트, formatter 또는 생성 검증을 실행했다는 의미가 아닙니다.

### 6-2. Dependencies

- 현재 기준 입력은 공개 `github.com/chatanium/discordgo v0.30.0`, 소비 Go 모듈 파일 및 Protobuf 소스입니다. Go 최소 언어 버전 `1.27.0`과 실제 compiler `1.27.3`을 구분하며 정확한 도구 버전은 `tools/versions.mk`를 따릅니다.
- 고정 도구의 배포 파일 또는 Go 모듈을 확보할 수 있는 개발 환경이 필요합니다. 이후 반복 빌드는 검증된 도구 캐시를 사용할 수 있습니다.
- CGO를 실제 사용하는 모듈은 해당 C 빌드 도구를 명시적인 빌드 입력으로 가집니다. 바이너리 및 이미지에 필요한 실행 라이브러리는 P4/P6에서 별도로 기록합니다.
- [P1](flexmodule-1-protocol-contracts.md)은 이 기준선 위에서 새 wire 계약을 고정합니다. [P2](flexmodule-2-domain-conversions.md)는 컴파일 수정 뒤의 전체 필드 및 값 보존을 담당합니다.
- [P5](flexmodule-5-voice-jobs.md)는 선택적 `build-voice`에 필요한 현재 Discord voice gateway, 공식 libdave CGO bridge 및 고정 native source/toolchain/runtime library 계약을 제공합니다. 일반 `build`, `build-all`과 공통 offline 검증은 이 native target을 암묵적으로 실행하지 않습니다.
- [P4](flexmodule-4-runtime-and-isolation.md)는 Docker Engine, runsc/gVisor 및 이미지 실행의 책임을 가집니다. [P6](flexmodule-6-examples-and-validation.md)는 예제별 의존성, 실험 helper 정리 및 통합 검증을 담당합니다.

## 7. References

- [현재 루트 모듈](../../go.mod), [현재 Makefile](../../Makefile), [현재 생성기 설정](../../proto/buf.gen.yaml), [현재 Buf 설정](../../proto/buf.yaml)
- [공개 DiscordGo 모듈](https://github.com/chatanium/discordgo/blob/v0.30.0/go.mod), [음악 예제 모듈](../../modules/music-bot/go.mod), [음성 예제 모듈](../../modules/audio-player/go.mod)
- [linked_roles SDK 예제](https://github.com/chatanium/discordgo/blob/v0.30.0/examples/linked_roles/go.mod), [voice_receive SDK 예제](https://github.com/chatanium/discordgo/blob/v0.30.0/examples/voice_receive/go.mod)
- 기존 helper 및 아래 v1 변환 경로는 2026-10-29 감사의 역사적 소스 근거이며 현재 빌드 입력이 아닙니다.
- [현재 정방향 interaction 변환](../../shared/discord-v1/convert/struct2buf/interactions.go), [현재 역방향 interaction 변환](../../shared/discord-v1/convert/buf2struct/interactions.go)
- [현재 component 변환](../../shared/discord-v1/convert/struct2buf/components.go), [현재 embed 변환](../../shared/discord-v1/convert/struct2buf/message.go), [현재 Discord schema](../../proto/discord-v1/discordgo.proto)
- [protobuf 및 Go 생성기 v1.36.12 공식 모듈 정보](https://proxy.golang.org/google.golang.org/protobuf/@v/v1.36.12.info), [gRPC v1.85.0](https://proxy.golang.org/google.golang.org/grpc/@v/v1.85.0.info)
- [protoc-gen-go-grpc v1.6.2](https://proxy.golang.org/google.golang.org/grpc/cmd/protoc-gen-go-grpc/@v/v1.6.2.info), [Buf v1.75.0](https://proxy.golang.org/github.com/bufbuild/buf/@v/v1.75.0.info), [go-plugin v1.8.0](https://github.com/hashicorp/go-plugin/releases/tag/v1.8.0)
- [고정 DiscordGo upstream 변경 내역](https://github.com/bwmarrin/discordgo/compare/v0.29.0...f43dd94faaacd5b163e9e783f14b5bd8be639fc9), [go-ytdlp v1.5.6 Go 요구](https://proxy.golang.org/github.com/lrstanley/go-ytdlp/@v/v1.5.6.mod)
- [프로토콜 계약 제안](flexmodule-1-protocol-contracts.md), [도메인 변환 제안](flexmodule-2-domain-conversions.md), [런타임 및 격리 제안](flexmodule-4-runtime-and-isolation.md), [예제 및 검증 제안](flexmodule-6-examples-and-validation.md)
