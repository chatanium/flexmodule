# FlexModule v2 프로토콜과 전체 전송 계약 - Spec Proposal

| Item | Detail |
|------------|----------------------------------|
| Author | Seonguk Moon |
| Created | 2026-10-29 |
| Status | **Draft** / In Review / Approved |
| Reviewers | |

---

## 1. Summary

FlexModule 런타임과 모듈을 함께 재빌드하는 v2 전환을 통해 수명주기, 실제 승인 권한, 이벤트, 자격증명 참조, 파일 전송의 공통 계약을 정의합니다. 기존 선언형 도메인 전체와 P0에 고정한 공개 DiscordGo가 제공하는 공식 봇 API 및 이벤트에서 도달하는 타입을 모두 다루되, 런타임 객체와 원문 자격증명은 명시적인 경계 타입으로 대체합니다. 코어는 Discord나 음악 기능을 알지 않으며, Discord 백엔드와 선택적 음성 기능은 동일한 인스턴스 소유권 및 취소 규칙을 따릅니다.

## 2. Background & Motivation

현재 코어 스키마의 manifest에는 문자열 권한 목록이 있고, status에는 준비 여부만 있습니다. Go 인터페이스의 `OnStage(stage string)`에는 context와 오류 반환이 없으며, 런타임 클라이언트는 해당 RPC의 오류를 호출자에게 전달하지 않습니다. 이 상태에서는 필수 권한 협상, 초기화 실패, readiness 제한시간과 정상 종료를 하나의 계약으로 설명할 수 없습니다.

기존 감사의 Discord v1 스키마에는 메시지 154개와 enum 23개가 선언되어 있었습니다. 양방향 named 변환 쌍 72개, 단방향 2개, 내부 변환 2개, 비활성 전용 대상 39개, 변환 없는 대상 28개, scalar wrapper 11개는 이 역사적 기준선의 분류입니다. 기존 helper RPC 40개와 함께 감사 근거로 보존하며 최종 API, 이벤트 또는 DTO의 상한으로 사용하지 않습니다.

기존 SDK `v0.29.0` 감사는 `Session` 메서드 229개, event row 73개, API root 57개와 reachable named type 199개를 기록했습니다. 현재 입력은 P0의 태그 없는 upstream commit `f43dd94faaacd5b163e9e783f14b5bd8be639fc9`이며 [source summary](../../tools/protocol/inventory/source-refresh-summary.json), [API catalog](../../tools/protocol/inventory/source-api.json), [event catalog](../../tools/protocol/inventory/source-events.json), [type closure](../../tools/protocol/inventory/source-type-closure.json)를 기준으로 합니다. 현재 `Session` 메서드 232개는 canonical 152개, alias 43개, backend helper 13개, 제외 24개로 분류합니다. canonical은 REST 149개와 Gateway 3개이며 노출하는 상태 계산 `UserChannelPermissions` 1개를 합한 SDK-derived Helper 작업은 153개입니다. 새 route 등록/해제와 운영자 credential binding 조회 3개는 이 SDK 메서드 집계와 별도입니다.

현재 event row는 74개이며 모듈 전달 대상은 67개입니다. API root 59개, API named-type closure 204개, event closure 228개, 두 closure의 합집합 282개를 추적합니다. 이는 Protobuf 메시지 개수나 변환 완료율이 아닙니다. 최신 입력을 반영하지 않은 역사적 229/73/199 집계로 범위를 닫지 않습니다.

사용자는 일반 봇, 관리 및 운영 모듈, 메시지와 명령 모듈, 음성 모듈을 모두 지원하는 범용 런타임을 요구했습니다. 따라서 지금 사용하는 예제의 DTO만 고치거나, 음성 기능을 코어 모듈 종류로 삼거나, 임의 JSON과 HTTP 전달로 누락된 API를 대신하는 방식은 범위에 맞지 않습니다. 아래 v2 패키지, 필드와 인터페이스는 구현 계약 제안이며 현재 저장소에 이미 존재하는 API라는 뜻이 아닙니다.

## 3. Goals & Non-Goals

### 3.1 Goals

- [ ] 런타임과 모든 모듈을 application protocol version 2로 함께 전환하고 v1 실행 경로를 제거합니다.
- [ ] 필수 및 선택 capability, 백엔드 버전, 실제 grants, 초기화와 준비 상태를 명시적으로 전달합니다.
- [ ] 기존 선언형 도메인 전체와 허용한 API 및 이벤트에서 도달하는 타입의 필드, enum, variant를 빠짐없이 분류합니다.
- [ ] absent, zero, empty, clear, 명시적 JSON null을 구분하고 정수, permission mask와 문자열 enum의 의미를 보존합니다.
- [ ] 원문 Discord 자격증명, 라이브 런타임 객체, 임의 전송 우회 경로를 모듈 경계에서 제거합니다.
- [ ] 전체 봇 작업을 typed request/response로 연결하고 편의 메서드 및 제외 작업마다 근거를 남깁니다.
- [ ] 일반 이벤트와 선택적 음성 기능이 동일한 인증 인스턴스, 권한 범위, context 및 공통 오류 계약을 사용하게 합니다.
- [ ] 기존 선언 목록, API 목록, 이벤트 목록을 서로 다른 완전성 기준으로 유지하고 통합 수용 조건을 정의합니다.

### 3.2 Non-Goals

- [ ] v1 호환 shim, 구형 package 재수출, 오래된 RPC 또는 잘못된 이벤트 이름의 호환 alias를 제공하지 않습니다.
- [ ] 사용자 계정 자동화, OAuth 소유자 전용 작업, 폐지되거나 공식 봇 경로가 아닌 작업을 추가하지 않습니다.
- [ ] 현재 로컬 SDK에 없는 모든 최신 Discord 기능까지 지원한다고 주장하지 않습니다.
- [ ] raw Gateway 이벤트, 임의 URL HTTP, 임의 opcode, SQL, shell 또는 credential export helper를 제공하지 않습니다.
- [ ] 이벤트의 영속 로그, 재시작 후 replay, exactly-once 또는 자동 재시작을 제공하지 않습니다.
- [ ] `Session`, `State`, `VoiceConnection`, `io.Reader`를 값으로 직렬화하지 않습니다.
- [ ] 이 문서에서 컨테이너 격리, multipart 전송, 변환 함수 또는 음성 스케줄러의 구현을 완료한 것으로 간주하지 않습니다. 해당 구현은 연결된 제안서의 범위입니다.

## 4. Technical Design

### 4.1 Architecture Overview

#### 패키지와 책임 경계

새 protobuf namespace는 `flexmodule.core.v2`와 `flexmodule.discord.v2`입니다. 생성 코드와 Go wrapper는 각각 `proto/core-v2`, `proto/discord-v2`, `shared/core-v2`, `shared/discord-v2`에 둡니다. 이는 새 경로 제안이며 현재 존재하는 v1 경로를 설명하는 소스 링크와 구분합니다.

| 계층 | 소유하는 계약 | 소유하지 않는 계약 |
|---|---|---|
| 코어 | 인증된 `InstancePrincipal`, manifest, grants, stage, config, backend-neutral envelope, 파일 전송 및 자원 소유권 | Discord intent, Discord 권한 계산, 명령, 음성, 음악 처리 |
| Discord 백엔드 | typed DTO, 허용된 REST/Gateway 작업, 이벤트 등록, 리소스 범위 해석, 자격증명 projection | 모듈이 주장한 ID에 의한 인증, 범용 HTTP 전달 |
| 모듈 SDK | context 전달, typed 이벤트 decode, 안전한 Go view, 명시적 파일 업로드 호출 | 원문 런타임 자격증명, credential resolver, 호스트 경로 |
| 음성 백엔드 | 승인된 재생 job과 수신 subscription, Opus 전송 및 소유권 | 코어 모듈 종류 구분, 자동 트랜스코딩, 임의 voice connection 전달 |

코어는 등록된 백엔드의 문자열 ID와 major version, protobuf `Any`의 허용된 type URL만 다룹니다. Discord DTO를 코어 proto 또는 코어 런타임 인터페이스에서 import하지 않습니다. 애플리케이션 composition root가 Discord adapter를 등록합니다.

#### v2 전환

1. [P0 빌드 기준선](flexmodule-0-build-and-dependency-baseline.md)의 도구 및 의존성 기준을 고정합니다.
2. go-plugin application handshake의 `ProtocolVersion`을 `2`로 올립니다. manifest의 `protocol_major`도 `2`여야 하며 둘 중 하나라도 다르면 초기화하지 않습니다. 이는 go-plugin 라이브러리 자체의 내부 프로토콜 번호와 구분합니다.
3. runtime, test-module, 일반 봇 예제, 기존 음악 예제, voice-player와 통합 검증의 모든 import, plugin map, helper 및 hook 호출을 새 계약으로 바꿉니다.
4. 통합 전환 시 구형 v1 schema, 생성 코드, wrapper, void hook, 불완전 helper 구현과 병행 초기화 경로를 제거합니다. 역사적 소스 근거는 문서에 남길 수 있지만 실행 경로로 유지하지 않습니다.
5. 런타임과 모듈을 함께 재빌드합니다. v1 모듈의 조용한 fallback이나 일부 기능만 실행하는 혼합 배포는 허용하지 않습니다.

상세 변환은 [P2](flexmodule-2-domain-conversions.md), Discord 작업과 파일 전송은 [P3](flexmodule-3-discord-backend.md), 인증과 수명주기는 [P4](flexmodule-4-runtime-and-isolation.md), 음성 job은 [P5](flexmodule-5-voice-jobs.md), 예제 및 수용 검증은 [P6](flexmodule-6-examples-and-validation.md)에 연결합니다.

### 4.2 Data Model Changes

공개 런타임 DB나 KV schema는 추가하지 않습니다. 모듈의 영속 디렉터리 형식은 모듈이 소유하며 일반 종료나 재시작으로 삭제하지 않습니다. 아래 wire DTO와 인스턴스에 속한 메모리 상태, credential vault entry 및 upload entry는 새 계약입니다. 명령 소유권 같은 백엔드 내부 영속 메타데이터의 필요 여부와 저장 방식은 P3에서 다루며 공개 코어 저장 서비스로 노출하지 않습니다.

#### 4.2.1 Manifest, capability 및 실제 grants

다음 코어 schema는 규범적 필드 정의입니다. `Any`, `Timestamp`, `Duration`, `Empty`, `NullValue`는 Google protobuf well-known type을 사용합니다. enum의 `UNSPECIFIED` 값은 정상 요청 값이 아니며, 등록된 의미가 없는 숫자를 정상 상태로 해석하지 않습니다.

```proto
syntax = "proto3";
package flexmodule.core.v2;

message ResourceSelector {
  map<string, string> resource_ids = 1;
}

message ResourceScope {
  oneof selector_kind {
    bool all_resources = 1;
    ResourceSelector selector = 2;
  }
}

message CapabilityRequest {
  string name = 1;
  repeated ResourceScope resource_scopes = 2;
}

message BackendRequirement {
  string backend_id = 1;
  uint32 protocol_major = 2;
  bool required = 3;
  repeated CapabilityRequest required_capabilities = 4;
  repeated CapabilityRequest optional_capabilities = 5;
}

message ModuleManifest {
  uint32 protocol_major = 1;
  string name = 2;
  string version = 3;
  string author = 4;
  string repository = 5;
  repeated CapabilityRequest required_capabilities = 6;
  repeated CapabilityRequest optional_capabilities = 7;
  repeated BackendRequirement backend_requirements = 8;
}

message CapabilityGrant {
  string name = 1;
  repeated ResourceScope resource_scopes = 2;
  google.protobuf.Any constraints = 3;
}

message BackendGrant {
  string backend_id = 1;
  uint32 protocol_major = 2;
  repeated CapabilityGrant capabilities = 3;
}

enum ResourceClass {
  RESOURCE_CLASS_UNSPECIFIED = 0;
  RESOURCE_CLASS_GENERAL = 1;
  RESOURCE_CLASS_MEDIA = 2;
}

enum EgressPolicy {
  EGRESS_POLICY_UNSPECIFIED = 0;
  EGRESS_POLICY_NONE = 1;
  EGRESS_POLICY_PUBLIC_INTERNET = 2;
}

message ResourceLimits {
  uint32 cpu_millicores = 1;
  uint64 memory_bytes = 2;
  uint32 max_pids = 3;
  uint64 persistent_bytes = 4;
  uint64 scratch_bytes = 5;
}

message Grants {
  repeated CapabilityGrant core_capabilities = 1;
  repeated BackendGrant backends = 2;
  ResourceClass resource_class = 3;
  ResourceLimits resource_limits = 4;
  EgressPolicy egress_policy = 5;
}
```

- `ModuleManifest.name`은 배포 산출물의 이름입니다. 다른 모듈의 신원을 주장하는 `ModuleID`로 사용하지 않습니다. 설치 설정이 안정적인 `ModuleID`를 정하고 매 실행마다 새 `InstanceID`를 만듭니다.
- 필수 core capability의 요청 범위를 모두 승인하지 못하면 활성화를 거부합니다. `required=true`인 백엔드는 해당 major와 필수 capability를 모두 만족해야 합니다. `required=false`인 백엔드는 이 조건을 만족할 때만 제공하며, 그렇지 않으면 해당 `BackendGrant` 자체를 생략합니다. 제공된 백엔드의 선택 capability는 승인된 범위만 전달합니다.
- 실제 grants는 요청과 운영자 승인 및 백엔드가 제공할 수 있는 기능의 교집합입니다. 승인하지 않은 권한을 manifest에서 그대로 복사하지 않습니다. 선택 capability의 미승인 여부는 요청과 실제 grants의 차이로 알 수 있습니다.
- `ResourceSelector.resource_ids`의 항목은 AND 조건이고 여러 `ResourceScope`는 OR 조건입니다. Discord에서는 `guild_id`, `channel_id`, `application_id` 등 작업별 허용 키를 adapter가 정합니다. 채널이 어느 guild에 속하는지는 신뢰할 수 있는 백엔드 조회 결과로 확인합니다.
- 비어 있는 scope 목록은 권한이 없다는 뜻입니다. 비어 있는 selector는 registry가 명시적으로 리소스 비지정 작업으로 등록한 경우만 허용합니다. core config, 자신의 파일 전송, 네트워크 capability에는 이 방식의 인스턴스 범위를 사용합니다. 빈 map을 Discord 전체 리소스 wildcard로 해석하지 않습니다.
- `all_resources`는 값이 `true`인 경우에만 유효하며 운영자가 명시적으로 승인해야 합니다. 문자열 prefix나 `*`를 묵시적 wildcard로 해석하지 않습니다.
- `constraints`는 capability별 등록된 type URL만 허용합니다. 예를 들어 `discord.voice.play`에는 Discord namespace의 `VoicePlaybackConstraints{max_priority}`를 사용합니다. 타입이 필요한데 없거나 잘못된 타입이면 승인하지 않으며 무제한 권한으로 간주하지 않습니다.
- core capability 이름은 `core.config.read`, `core.upload`, `core.network.public_egress`로 고정합니다. `core.upload`는 자신의 업로드 생성과 삭제에만 적용합니다. Discord 작업은 `discord.rest.<CanonicalMethod>`, `discord.gateway.<CanonicalMethod>`, `discord.state.UserChannelPermissions`, `discord.event.<공식 이벤트 이름>`, `discord.voice.play`, `discord.voice.receive`로 등록합니다. 새 라우팅 및 운영자 credential binding helper는 각각 `discord.routes.manage`, `discord.credentials.reference`를 사용합니다.
- `ResourceClass`는 운영자가 승인하는 자원 프로필입니다. 음악 모듈이나 기능 종류를 나타내지 않습니다. 기본값은 general이 500 millicores, 256 MiB, PID/thread 64, 영속 1 GiB, scratch 256 MiB이고 media는 2,000 millicores, 1 GiB, 128, 5 GiB, 2 GiB입니다. 값은 사용자 확인 기본값이며 운영자가 설정할 수 있습니다.
- public egress에는 해당 capability 승인과 `PUBLIC_INTERNET` 정책이 모두 필요합니다. 선택 승인을 받지 못하면 `NONE`이고 필수 요청이 거절되면 시작하지 않습니다. 이 정책은 호스트, 사설, link-local, metadata, 내부 목적지 접근을 허용한다는 뜻이 아닙니다.

#### 4.2.2 초기화, 상태와 이벤트 envelope

```proto
message InitContext {
  string module_id = 1;
  string instance_id = 2;
  Grants grants = 3;
  string persistent_directory = 4;
  string scratch_directory = 5;
}

enum Stage {
  STAGE_UNSPECIFIED = 0;
  STAGE_INIT = 1;
  STAGE_START = 2;
  STAGE_DRAIN = 3;
  STAGE_STOP = 4;
}

message StageRequest {
  Stage stage = 1;
  InitContext init_context = 2;
}

enum Readiness {
  READINESS_UNSPECIFIED = 0;
  READINESS_NOT_READY = 1;
  READINESS_READY = 2;
  READINESS_FAILED = 3;
}

message Status {
  Readiness readiness = 1;
  optional string detail = 2;
}

message EventEnvelope {
  string backend_id = 1;
  uint32 backend_major = 2;
  string event_type = 3;
  uint64 ingress_sequence = 4;
  google.protobuf.Timestamp received_at = 5;
  google.protobuf.Any payload = 6;
}
```

`InitContext`는 runtime이 `INIT`에서 한 번 전달합니다. 디렉터리는 컨테이너 안에서 보이는 경로이며 호스트 경로가 아닙니다. `StageRequest.init_context`는 `INIT`에서 필수이고 다른 stage에서는 없어야 합니다. 같은 인스턴스의 grants는 변경하지 않으며 정책 변경은 새 인스턴스로 재시작하여 적용합니다. 별도 `GetGrants` RPC는 두지 않습니다.

`Status`는 모듈의 readiness 보고입니다. 런타임의 `STARTING`, `INITIALIZING`, `READY`, `RUNNING`, `DRAINING`, `STOPPED`, `FAILED` 상태는 호스트가 소유하며 모듈이 `Status`로 지정할 수 없습니다. `detail`은 비밀이 없는 설명이며 제어 판단을 위한 자유 형식 상태 문자열로 사용하지 않습니다.

`ingress_sequence`는 모든 등록 백엔드에 공통인 런타임의 단일 admission 지점에서 1부터 증가하는 `uint64`입니다. 같은 런타임 수명 안에서 서로 다른 이벤트를 같은 값으로 표시하지 않으며 wrap 전에 실패합니다. 모듈은 필터된 증가 부분수열을 받으므로 값 사이의 공백은 정상입니다. 이는 Discord Gateway sequence, replay cursor 또는 영속 event ID가 아닙니다. `received_at`은 런타임이 수신한 UTC 시각이며 Discord 발생 시각을 대신하지 않습니다.

`payload.type_url`은 `(backend_id, backend_major, event_type)` registry와 정확히 일치해야 합니다. Discord 타입은 `type.googleapis.com/flexmodule.discord.v2.<EventMessage>`를 사용합니다. `Any`에는 raw JSON, SDK `Event`, caller 지정 임의 타입 또는 credential을 우회 전달하는 확장 필드를 넣을 수 없습니다.

#### 4.2.3 자격증명, 업로드와 정확한 JSON

```proto
message CredentialRef {
  string opaque_id = 1;
  string backend_id = 2;
  string purpose = 3;
  ResourceSelector resource_scope = 4;
  google.protobuf.Timestamp expires_at = 5;
}

message JSONValue {
  bytes json = 1;
}

message UploadMetadata {
  string file_name = 1;
  string content_type = 2;
  uint64 declared_size_bytes = 3;
}

message FileChunk {
  uint64 offset = 1;
  bytes data = 2;
}

message UploadChunk {
  oneof part {
    UploadMetadata metadata = 1;
    FileChunk chunk = 2;
  }
}

message UploadedFileRef {
  string opaque_id = 1;
  string file_name = 2;
  string content_type = 3;
  uint64 size_bytes = 4;
  google.protobuf.Timestamp expires_at = 5;
}

message DeleteUploadRequest {
  UploadedFileRef file = 1;
}

message NullableString {
  oneof kind {
    string value = 1;
    google.protobuf.NullValue null_value = 2;
  }
}

message NullableTimestamp {
  oneof kind {
    google.protobuf.Timestamp value = 1;
    google.protobuf.NullValue null_value = 2;
  }
}

message StringList {
  repeated string values = 1;
}

message StringMap {
  map<string, string> values = 1;
}
```

`CredentialRef`의 metadata는 설명용이며 서버 vault의 값을 수정하지 못합니다. vault는 실제 opaque ID를 인증된 `ModuleID`, `InstanceID`, backend, purpose, 리소스 및 만료에 묶습니다. 다른 인스턴스에 복사하거나 참조의 만료 시각과 scope를 고쳐도 권한이 생기지 않습니다. 모든 사용에서 저장된 binding과 실제 작업 대상을 다시 확인합니다.

등록하는 module-visible purpose는 `discord.interaction`, `discord.webhook`, `discord.user.guilds.join`입니다. 봇 토큰과 voice token은 모듈에 원문이나 범용 참조로 주지 않습니다. interaction 및 webhook의 짧은 수명은 원본 자격증명 유효기간을 넘지 않아야 하고, 운영자 제공 사용자 자격증명은 등록된 app, user, guild와 용도로 제한합니다. 원본에 확인 가능한 만료가 없으면 운영자가 유한한 만료를 설정해야 하며 숨은 무제한 기본값을 두지 않습니다. 일반 OAuth 발급, 갱신 또는 export 서비스는 추가하지 않습니다.

`JSONValue.json`은 정확히 하나의 완전한 UTF-8 JSON 값을 담습니다. 유효하지 않은 JSON, 여러 최상위 값, `NaN`과 infinity는 거부합니다. 바깥 message가 없으면 absent이고 `json`이 `null`이면 명시적 JSON null입니다. 감사 로그의 `OldValue`와 `NewValue` 등 실제로 동적인 값에만 사용하며 숫자 token을 `double`로 일괄 변환하지 않습니다. raw body 전체를 여기에 넣어 typed schema나 credential 정책을 우회할 수 없습니다.

파일은 인스턴스에 속한 임시 저장 참조입니다. 같은 인스턴스에서 여러 작업에 재사용할 수 있지만 다른 모듈이나 재시작 뒤에는 사용할 수 없습니다. 파일명과 content type은 metadata일 뿐이며 경로를 뜻하지 않습니다. 유효한 읽기 lease와 새 reader 생성은 런타임에서만 수행합니다.

#### 4.2.4 Patch 존재성과 도메인 값

Discord namespace에는 실제 element type별 list wrapper를 둡니다. 다음은 `MessageEdit` 전체 입력의 존재성 계약을 보이는 규범적 예입니다. 참조한 domain DTO는 4.3의 전체 필드 매핑 규칙을 따릅니다.

```proto
message FileAttachment {
  flexmodule.core.v2.UploadedFileRef uploaded_file_ref = 1;
  string filename = 2;
  optional string description = 3;
}

message FileAttachmentList { repeated FileAttachment values = 1; }
message EmbedList { repeated MessageEmbed values = 1; }
message ComponentList { repeated MessageComponent values = 1; }
message AttachmentEditEntry {
  oneof source {
    string existing_attachment_id = 1;
    uint32 new_file_index = 2;
  }
  optional string filename = 3;
  optional string description = 4;
}

message AttachmentEditList { repeated AttachmentEditEntry values = 1; }

message MessageEdit {
  string id = 1;
  string channel_id = 2;
  flexmodule.core.v2.NullableString content = 3;
  ComponentList components = 4;
  EmbedList embeds = 5;
  MessageAllowedMentions allowed_mentions = 6;
  optional uint64 flags = 7;
  FileAttachmentList files = 8;
  AttachmentEditList attachments = 9;
}

message AuditLogChange {
  optional string key = 1;
  flexmodule.core.v2.JSONValue old_value = 2;
  flexmodule.core.v2.JSONValue new_value = 3;
}
```

| wire 상태 | 의미 | Go 및 JSON 경계 규칙 |
|---|---|---|
| optional scalar 또는 wrapper 없음 | 해당 필드를 보내지 않거나 patch에서 유지합니다. | nil을 zero 또는 빈 목록으로 바꾸지 않습니다. |
| scalar가 있고 값이 `0`, `false`, `""` | 해당 값을 명시적으로 설정합니다. | `omitempty` 때문에 소실되는 SDK struct를 경유하지 않습니다. |
| list wrapper가 있고 `values=[]` | 빈 목록을 명시합니다. patch에서는 해당 목록을 지웁니다. | nil `*slice`와 빈 slice를 가리키는 포인터를 구분합니다. |
| nullable wrapper의 `null_value` | API가 허용하는 명시적 JSON null입니다. | 예를 들어 채널 이동의 null은 voice 연결 해제이고 timeout null은 timeout 해제입니다. |
| `JSONValue`가 있고 값이 `null` | 실제 동적 JSON 값이 null입니다. | audit field가 없는 상태와 구분합니다. |

`NullableString`이 존재하지만 oneof가 없으면 잘못된 요청입니다. nullable wrapper가 있다고 모든 endpoint에서 null을 허용하지는 않습니다. 예를 들어 메시지 내용 삭제는 `content.value=""`로 표현하며, 해당 API가 null을 허용하지 않는 필드에 `null_value`를 보내면 거부합니다. nullable list나 map이 필요한 필드에도 같은 원칙으로 `values`와 `null_value`의 oneof를 가진 concrete wrapper를 사용합니다.

`MessageEdit.files`는 새 파일의 순서 있는 목록이고, present `attachments`는 수정 후 남길 기존 파일과 추가할 새 파일을 모두 담은 완전한 목록입니다. `AttachmentEditEntry.source`는 `existing_attachment_id` 또는 `new_file_index` 중 정확히 하나여야 합니다. 선택된 `new_file_index=0`도 유효한 첫 파일 참조이며 oneof 미지정과 구분합니다. 기존 ID는 비어 있지 않은 snowflake여야 하고 중복할 수 없습니다. 새 index는 `files` 범위 안에 있어야 하며, 새 파일이 있으면 present 목록이 모든 index를 정확히 한 번씩 지정해야 합니다.

`attachments`가 없을 때 기존 첨부를 유지하는 의미는 새 파일이 없는 수정에만 허용합니다. present 빈 목록은 전체 첨부 제거이며 새 파일과 함께 보낼 수 없습니다. runtime이나 SDK가 유지 목록을 숨은 조회로 채우거나 기존 목록에 새 파일을 암묵적으로 덧붙이지 않습니다. 이 계약은 `MessageEdit`, `WebhookEdit`, `InteractionResponseData`의 메시지 갱신 응답 및 `ChannelMessageEditComplex`, `WebhookMessageEdit`, `InteractionRespond`의 갱신 경로에 동일하게 적용합니다. `InteractionResponseEdit`와 `FollowupMessageEdit` 등 편의 매핑도 같은 목록과 index를 그대로 보존합니다.

기존 ID 항목의 `filename`과 `description`은 해당 수정 endpoint가 허용하는 metadata 변경만 표현합니다. 미지정 값은 유지하고, 명시적 빈 `description`은 설명을 지우며, 빈 `filename`, 명시적 null 또는 endpoint가 지원하지 않는 변경은 거부합니다. 새 파일의 이름과 설명은 `files[index]`의 `FileAttachment.filename`과 선택적 `description`이 기준입니다. 새 index 항목에서 이 metadata를 생략하면 기준 값을 사용하고, 중복 지정하면 기준 값 및 description의 존재 여부와 일치해야 합니다. 서로 다른 값이나 description이 없는 파일에 entry만 설명을 지정한 요청은 거부합니다. `attachment://filename`과 적용 가능한 spoiler 이름은 이 최종 파일명에 대응해야 합니다.

`MessageAttachment`는 응답 및 이벤트의 완전한 도메인 snapshot DTO로 유지합니다. 크기, URL, content type, duration, waveform, flags 등 응답 필드를 attachment 수정 요청으로 되보내는 용도로 사용하지 않습니다. singular `File` 및 `Embed` 같은 기존 SDK 편의 필드는 새 wire에 중복 추가하지 않고 canonical plural 필드로 명시적으로 정규화합니다. 신구 필드가 서로 다른 값을 갖는 입력은 임의 우선순위로 숨기지 않고 거부합니다.

`MessageEdit.Flags`처럼 기존 SDK가 absent와 zero를 표현하지 못하는 필드는 typed wire 및 `MessageEditPatch` 같은 presence-aware Go view를 기준으로 처리합니다. SDK snapshot을 반환하더라도 원래 patch 존재성을 복원했다고 주장하지 않습니다. 런타임은 필요하면 typed 요청에서 바로 JSON을 작성합니다.

#### 4.2.5 숫자, enum과 polymorphic variant

| 종류 | v2 표현 및 유효성 |
|---|---|
| Discord snowflake | decimal string을 사용합니다. ID를 `double`이나 platform `int`로 바꾸지 않습니다. 숫자 ID는 unsigned 64-bit 범위를 검사하며 `@me`, `@original`처럼 특정 작업에 허용된 selector는 별도 작업 규칙으로 처리합니다. |
| Permissions, allow, deny, default member permissions | 비음수 10진 문자열 bitmask를 사용합니다. 일반 permission은 `"0"`이 유효하며 nullable permission의 absent/null/zero를 구분합니다. 현 SDK의 `int64`로 내릴 때 `0..2^63-1` 범위를 넘으면 오류이고 조용히 축소하지 않습니다. |
| MessageFlags, UserFlags 등 numeric flag | ordinal enum이 아닌 `uint64` mask입니다. source backing type의 표현 범위를 검사합니다. 현재 Linux amd64 `int` 기반 adapter에서는 signed 64-bit 양수 범위를 넘는 값을 오류로 처리합니다. |
| 실제 numeric enum | source의 실제 numeric 값을 사용합니다. 예를 들어 정상값 `0`이 있는 context 또는 integration type에 임의 1 offset을 더하지 않습니다. 구조가 필요 없는 알려지지 않은 표현 가능한 값은 source/API가 허용할 때 보존합니다. |
| 문자열 enum | `EmbedType`, `AllowedMentionType`, locale 등은 string 또는 정확한 문자열 대응표입니다. 숫자를 Unicode rune으로 바꾸는 cast를 사용하지 않습니다. |
| interaction INTEGER | wire는 `sint64`이고 Discord 범위는 `[-2^53+1, 2^53-1]`입니다. source 숫자가 유한한 정수인지 검사하며 SDK 재구성의 `Value`에는 `float64`를 넣습니다. 로컬 `IntValue()`가 `float64` type assertion을 하기 때문입니다. |
| interaction NUMBER | wire는 `double`이고 Discord 범위는 `[-2^53, 2^53]`입니다. 비유한 값을 거부하며 integer variant로 바꾸지 않습니다. |
| source `int`, 길이, offset, 합계 | 명시적인 폭의 wire 정수를 사용합니다. Go 변환 범위와 덧셈 overflow를 검사하며 음수 길이와 wrap을 허용하지 않습니다. |
| 시각 | UTC `Timestamp`와 명시적 presence를 사용합니다. zero time을 임의 현재 시각으로 바꾸지 않습니다. API가 Unix 단위 숫자를 요구하는 값은 원래 단위를 계약에 남깁니다. |

숫자 한계는 Discord 문서, protobuf 및 현재 Go adapter의 기술적 제약이며 사용자가 정한 운영 quota와 구분합니다.

```proto
message ApplicationCommandOptionValue {
  oneof value {
    string string_value = 1;
    sint64 integer_value = 2;
    bool boolean_value = 3;
    string user_id = 4;
    string channel_id = 5;
    string role_id = 6;
    string mentionable_id = 7;
    double number_value = 8;
    string attachment_id = 9;
  }
}

message ApplicationCommandInteractionDataOption {
  string name = 1;
  uint32 type = 2;
  ApplicationCommandOptionValue value = 3;
  repeated ApplicationCommandInteractionDataOption options = 4;
  optional bool focused = 5;
}

message ApplicationCommandChoiceValue {
  oneof value {
    string string_value = 1;
    sint64 integer_value = 2;
    double number_value = 3;
  }
}

message InteractionData {
  oneof data {
    ApplicationCommandInteractionData application_command = 1;
    MessageComponentInteractionData message_component = 2;
    ModalSubmitInteractionData modal_submit = 3;
  }
}

message MessageComponent {
  oneof component {
    ActionsRow actions_row = 1;
    Button button = 2;
    SelectMenu select_menu = 3;
    TextInput text_input = 4;
    Section section = 5;
    TextDisplay text_display = 6;
    Thumbnail thumbnail = 7;
    MediaGallery media_gallery = 8;
    FileComponent file = 9;
    Separator separator = 10;
    Container container = 11;
    Label label = 12;
    FileUpload file_upload = 13;
  }
}
```

옵션의 `type`과 oneof는 일치해야 합니다. subcommand와 subcommand group은 중첩 `options`를 사용하고 scalar value를 넣지 않습니다. user, channel, role, mentionable, attachment ID를 모두 구분하며 unknown option을 nil 성공으로 처리하지 않습니다. command choice의 string/integer/number variant는 부모 옵션 타입과 같아야 합니다. autocomplete는 application-command data를 사용하되 interaction의 원래 autocomplete 종류와 `focused`를 보존하며, 필수 옵션이 아직 없거나 문자열이 미완성인 입력을 최종 명령 실행처럼 검증하지 않습니다. 잘못된 숫자는 정상 숫자로 보정하지 않습니다.

`SelectMenu`의 source type 값은 3, 5, 6, 7, 8을 구분하여 유지합니다. protobuf oneof tag가 Discord component type 값이라는 뜻은 아닙니다. concrete component 13개를 모두 다루며 새 `Label`의 Discord type은 18, `FileUpload`는 19입니다. `Label.Component`의 재귀 variant와 modal의 Label 안에 배치하는 FileUpload를 보존합니다. unknown polymorphic variant는 `UNSUPPORTED_VARIANT` 오류이며 JSON bytes fallback은 없습니다.

resolved map은 입력 key가 권위 있는 식별자입니다. `members`의 key를 `Member.User.ID`에서 다시 만들지 않습니다. partial member의 `User`가 없으면 그대로 없고, 원본 map key 및 원본 nested ID를 각각 보존합니다. 모순되는 값이 작업의 리소스 권한에 영향을 주면 경계 검증에서 거부하되 converter가 임의로 고치지 않습니다.

resolved 타입의 현재 이름은 `ComponentInteractionDataResolved`입니다. 이전 `MessageComponentInteractionDataResolved` 이름을 호환 alias로 남기지 않으며 component 및 `ModalSubmitInteractionData.Resolved`의 `Attachments` map도 원래 key로 보존합니다. `SelectMenu.Values`, `FileUpload.Values`와 `MinValues`/`MaxValues`, `Required *bool`을 포함합니다. `TextInput.Required` 및 `SelectMenu.Required`도 `*bool`이므로 nil, false, true를 구분합니다.

### 4.3 Core Logic

#### 4.3.1 세 가지 완전성 기준과 타입 폐쇄 집합

전송 범위는 다음 세 집합의 합집합을 재귀적으로 확장한 결과입니다.

1. 기존 감사 schema의 메시지 154개와 enum 23개입니다. 역사적 기준선 전체를 분류하며 이 개수를 최종 ceiling으로 사용하지 않습니다.
2. 허용한 SDK-derived typed 작업 153개와 편의 매핑 43개의 input/output, source payload 및 nested field입니다. 현재 직접 API root는 59개, reachable named type은 204개입니다. 기존 root 57개/type 199개와 struct 131개, scalar/alias 66개, interface 2개 분류는 과거 감사 기록으로만 유지합니다. 현재 API/event closure 합집합 282개도 wire message 개수와 구분합니다.
3. 아래에서 모듈에 제공하기로 분류한 이벤트의 envelope와 nested domain 값입니다. 기존 base DTO가 존재하더라도 update/delete의 snapshot과 chunk metadata를 가진 event wrapper까지 존재한다는 뜻이 아닙니다.

각 source type 및 field는 `완전한 직접 변환`, `상위 DTO 내부의 완전한 변환`, `명시적 안전 wrapper 또는 참조 대체`, `런타임 전용 제거` 중 정확히 하나의 결과를 갖습니다. 제외한 API에 쓰이는 순수 DTO도 기존 선언형 도메인 집합에 포함되어 있으면 변환 범위에서 자동으로 빠지지 않습니다. `Gateway`와 `GatewayBot`의 안전한 읽기 전용 metadata는 포함하며 `GatewayBotResponse.SessionStartLimit`의 실제 로컬 타입인 `SessionInformation`도 완전히 변환합니다. 공개 base URL, shards, total, remaining, reset-after의 밀리초 단위, max concurrency를 보존하되 session/resume 정보나 연결 제어 권한을 추가하지 않습니다.

원칙은 다음과 같습니다.

- `Session`, `Identify`, 라이브 `State`의 mutex, handler, client, dialer, cache 제어, heartbeat 및 connection은 v2 DTO에서 제거합니다. 빈 `Session`을 되돌려 손실 없는 변환처럼 보이게 하지 않습니다. `IdentifyProperties`, `GatewayStatusUpdate`처럼 독립적인 안전 데이터는 별도로 완전히 변환합니다.
- `User.Token`, voice state의 `SessionID`, READY의 `SessionID`, `Shard`, `ResumeGatewayURL` 및 voice server credential은 모듈 snapshot에서 제거합니다. interaction과 webhook token, `GuildMemberAddParams.AccessToken`은 아래의 typed 경계로 바꿉니다.
- `Activity.Secrets`의 match/join/spectate 값은 앱의 도메인 데이터이지 봇 토큰과 같은 필드가 아닙니다. source 필드를 일괄 삭제하지 않되 런타임 인증정보가 채워지는 경로는 boundary policy로 분리하고 일반 string dump로 취급하지 않습니다.
- 파일 `Reader`는 `UploadedFileRef`로 대체합니다. `Reader=nil`인 파일을 정상 변환 결과로 반환하지 않습니다.
- application, monetization, moderation, poll, webhook, component 등 현재 비활성 변환 파일에 있는 family도 동일한 기준으로 처리합니다. 비활성 파일을 삭제해 변환 대상 수만 줄이는 것은 완료가 아닙니다.
- v1 schema가 빠뜨린 attachment duration/waveform/flags, application privacy proxy URL, channel edit의 forum/tag 관련 필드, API root에 새로 필요한 `GuildChannelCreateData`, `ChannelEdit`, `ChannelFollow`, `UpdateStatusData` 같은 타입도 포함합니다. 안전한 실제 source field를 임의로 축소하지 않습니다.
- source의 exported domain field를 선언 순서와 타입으로 추적하고 pointer, slice, map, embedded field, named alias, interface concrete variant를 재귀적으로 확장합니다. 직렬화 제외 표시가 있는 field도 검토에서 빠지지 않습니다. local `BeforeUpdate`와 `BeforeDelete`처럼 명시적 의미가 있으면 optional snapshot으로 전달합니다.

완료 조건은 목록의 모든 항목에 매핑 결과와 근거가 있고, 요청 및 응답의 모든 reachable field가 도달하며, 기존 선언 수만 채운 상태에서 API 또는 이벤트용 타입이 비는 일이 없는 것입니다. 세 목록은 통합 문서의 source inventory에서 별도로 추적합니다.

#### 4.3.2 Credential source-to-wire 경계

```text
runtime source value + authenticated destination principal
  -> operation/event registry and resource-scope check
  -> capture field presence and exact dynamic JSON before lossy SDK decoding
  -> project known domain fields recursively
  -> replace eligible credentials with purpose-bound CredentialRef
  -> remove explicitly runtime-only fields
  -> validate typed payload and sanitize diagnostics/credential-bearing URLs
  -> enqueue immutable serialized envelope or return typed response

module typed request + authenticated broker context
  -> validate shape, instance state, grants and trusted resource relationships
  -> validate CredentialRef against vault owner/purpose/resource/expiry
  -> acquire upload read leases when needed
  -> resolve secret only inside runtime adapter
  -> issue exactly the approved Discord operation using caller context
  -> project response through the same egress boundary
```

pure field converter는 credential을 발급하지 않으며 원문 token을 받은 채 빈 문자열로 버리고 성공하지 않습니다. credential-bearing source를 전달할 때는 runtime projection을 사용해야 합니다. 모듈 SDK는 `InteractionHandle`과 `WebhookHandle`에 안전한 SDK view와 별도 typed reference를 담으며 `Value.Token`은 항상 빈 문자열입니다. opaque ID를 `Token`에 넣어 실제 Discord token처럼 보이게 하지 않습니다.

projection은 event만 아니라 REST 응답, nested application/user/webhook 객체, credential-bearing URL, structured error까지 재귀적으로 적용합니다. untyped error body, request header, 원문 URL, raw event를 예외 경로로 반환하지 않습니다. 일반 사용자 콘텐츠와 자격증명 필드를 같은 이유로 일괄 삭제하는 대신 source field 정책을 명시합니다. 알려진 런타임 credential이 오류 및 진단 문자열에 포함되는 경로도 제거합니다.

운영자가 제공한 `guilds.join` 사용자 자격증명은 `GuildMemberAdd`의 별도 참조로만 사용합니다. 해당 봇 작업을 사용자 자격증명이 필요하다는 이유만으로 제외하지 않습니다. 반대로 bearer-only 사용자 권한 편집 같은 OAuth 전용 작업을 이 참조로 열어 주지 않습니다.

#### 4.3.3 이벤트 분류, 존재성과 순서

현재 event registry의 74개 이름 가운데 synthetic은 `__CONNECT__`, `__DISCONNECT__`, `__EVENT__`, `__RATE_LIMIT__`의 4개입니다. raw `__EVENT__`를 중복 집계하지 않습니다. 이 네 항목과 `RESUMED`, `VOICE_SERVER_UPDATE`, `PRESENCES_REPLACE`의 7개를 제외하고 67개 source row를 typed 모듈 이벤트에 연결합니다. 기존 73개/모듈 전달 66개는 감사 기준선이며 새 `MESSAGE_REACTION_REMOVE_EMOJI`를 포함한 현행 범위의 상한이 아닙니다.

| source 분류 | 모듈 전송 계약 |
|---|---|
| 일반 봇 도메인 이벤트 | source event wrapper의 전체 안전 field를 `<SourceType>Event`에 옮깁니다. embedded base object만 보내고 wrapper field를 버리지 않습니다. |
| `READY` | `ReadyEvent`에 실제로 있는 안전한 version, user, application, guild 및 private-channel snapshot만 전달합니다. `SessionID`, `Shard`, `ResumeGatewayURL`은 제거하며 런타임 readiness와 구분합니다. |
| `VOICE_STATE_UPDATE` | `VoiceStateUpdateEvent`에서 guild/channel/user/member, 안전한 상태 flag와 시각 및 실제 `BeforeUpdate`를 전달합니다. `SessionID`는 제거합니다. 상태 관찰 capability는 playback/receive capability와 별개입니다. |
| `VOICE_SERVER_UPDATE` | token과 endpoint handoff를 음성 adapter 내부에서만 처리합니다. 일반 envelope나 module-visible token ref로 제공하지 않습니다. |
| `RESUMED`와 synthetic 4개 | host connection 및 rate-limit 처리에만 사용합니다. rate-limit 결과는 필요한 RPC의 typed 오류로 전달합니다. |
| `PRESENCES_REPLACE` | 로컬에 남아 있는 legacy 이벤트로 분류합니다. 현재 공식 봇 Gateway 문서에 지원 이벤트로 제시되지 않으므로 모듈 지원을 약속하지 않습니다. 순수 `Presence` 변환은 유지합니다. |
| source의 `STAGE_INSTANCE_EVENT_CREATE/UPDATE/DELETE` | 공식 `STAGE_INSTANCE_CREATE/UPDATE/DELETE`로 바로잡습니다. 로컬 `StageInstanceEventCreate/Update/Delete`는 `StageInstanceCreateEvent/UpdateEvent/DeleteEvent`에 매핑하며 registration과 generator를 함께 수정합니다. 잘못된 이름의 호환 alias는 두지 않습니다. |

공식 Gateway 문서는 `PRESENCE_UPDATE`를 정의하고 미문서화 데이터의 앱 지원을 보장하지 않는다고 설명합니다. 같은 문서의 실제 stage 이름을 사용합니다. 이벤트 목록에 적힌 source intent 추정값을 그대로 권한표로 삼지 않습니다. 예를 들어 `WEBHOOKS_UPDATE`에는 `GUILD_WEBHOOKS` intent가 필요하며 `GUILDS`로 대체하지 않습니다. 필수 이벤트에 필요한 intent 및 privileged intent를 사용할 수 없으면 협상 실패로 처리하고 선택 이벤트만 거절한 경우 실제 grants에서 제외합니다. Discord가 제공하지 않은 message content를 복구하거나 만들어 내지 않습니다.

`MESSAGE_UPDATE` 같은 partial event는 SDK가 scalar zero value로 바꾸기 전에 런타임 내부 JSON decoder에서 field presence를 포착합니다. typed DTO만 경계를 통과하고 원문은 모듈에 전달하지 않습니다. before snapshot은 로컬 cache에 실제로 있을 때만 존재하며 fetch해서 원래 존재했던 것처럼 채우지 않습니다. resolved member의 user나 webhook author를 완전한 일반 사용자라고 가정하지 않습니다.

중요한 wrapper는 다음 의미를 유지합니다.

- create, update, delete는 서로 다른 event type이며 delete를 create hook으로 보내지 않습니다.
- before snapshot은 source wrapper가 실제 제공한 경우만 보존합니다. 새 `ChannelDelete.BeforeDelete`, `ThreadDelete.BeforeDelete`는 `*Channel`, `GuildMemberRemove.BeforeDelete`는 `*Member`, `GuildRoleUpdate.BeforeUpdate` 및 `GuildRoleDelete.BeforeDelete`는 `*Role`입니다. 이를 `*GuildRole` wrapper로 잘못 바꾸지 않으며 기존 message/guild/member/voice-state snapshot에도 같은 원칙을 적용합니다.
- `GuildMembersChunk`의 nonce, chunk index/count, not-found 및 presence 정보, `ThreadListSync`의 channel 범위와 member 목록, scheduled-event user add/remove 식별자를 유지합니다.
- interaction event는 command, autocomplete, component, modal의 typed variant와 credential reference를 보존합니다.
- `WEBHOOKS_UPDATE`는 guild/channel의 webhook 목록 변경 알림입니다. 존재하지 않는 webhook create/update/delete Gateway 객체 이벤트로 확장하지 않습니다.
- `MESSAGE_REACTION_REMOVE_EMOJI`는 특정 emoji의 모든 반응 제거 이벤트이며 사용자별 제거 또는 모든 emoji 제거와 구분합니다. guild에서는 `GUILD_MESSAGE_REACTIONS`, DM에서는 `DIRECT_MESSAGE_REACTIONS` intent를 적용하고 payload에 없는 user ID를 만들지 않습니다.

백엔드 ingress는 bounded 직렬 경로를 사용합니다. 현재 `SyncEvents=false`가 handler마다 goroutine을 만드는 동작에 기대어 도착 순서를 추정하지 않습니다. 정렬된 admission 지점에서 전송 가능한 불변 snapshot을 만들고 권한에 맞는 각 인스턴스 FIFO에 별도로 넣습니다. 모듈별 기본 한도는 256개 또는 직렬화된 총 32 MiB 중 먼저 도달하는 값이며 processing lane은 하나입니다. overflow는 해당 모듈을 실패 및 중단시키고 다른 모듈의 큐는 계속 처리합니다.

한 이벤트의 hook이 반환하기 전에 다음 이벤트를 실행하지 않습니다. timeout 또는 transport loss가 발생하면 이전 handler가 아직 실행 중일 수 있으므로 그 인스턴스를 중단한 뒤 처리 경로를 닫습니다. 다음 이벤트를 먼저 시작하여 단일 lane 약속을 깨지 않습니다. decode 실패나 알려진 variant의 누락을 정상 empty event로 바꾸지 않습니다. 새롭거나 미지원인 외부 이벤트를 지원한 것으로 기록하지 않으며 raw fallback도 제공하지 않습니다.

#### 4.3.4 수명주기와 drain context

```text
Launch(startup context, one global 30-second deadline):
  create fresh authenticated InstancePrincipal
  validate application handshake version 2
  obtain and validate ModuleManifest
  compute actual Grants from registered capabilities and operator approval
  reject unmet required backend/capability/scope/intent
  call OnStage(INIT, InitContext)
  wait for Status.readiness == READY within the same deadline
  if readiness FAILED or any hook errors: fail and clean up the instance
  call OnStage(START) within the same deadline
  mark host state RUNNING; only now admit module events

Stop(drain context, default application drain 10 seconds):
  atomically close new event admission and mark DRAINING
  invoke DRAIN without discarding already-admitted event callbacks
  drain accepted FIFO work within the remaining deadline
  cancel unfinished event/RPC work when the deadline expires
  invoke STOP when the transport is still usable, without making cleanup depend on it
  close owned streams, jobs, uploads, credential refs and broker connection
  terminate/delete the sandbox through bounded host cleanup
  retain the module persistent directory
```

INIT에서 허용한 설정 조회와 명령 준비 등 작업은 registry의 stage 정책을 따릅니다. `RUNNING`이 아니라고 모든 INIT helper를 막거나, 반대로 모든 helper를 허용하지 않습니다. 초기화 중 외부 부작용이 생겼다면 실패 시 완전한 transactional rollback이 보장된다고 설명하지 않습니다.

`START`가 진행 중인 `READY`에서 `RUNNING` 전용 helper가 도착하면 인증과 실제 grant 및 scope 검사를 먼저 통과한 요청만 작업 admission이나 부작용 전에 유한한 activation gate에서 기다립니다. 대기 요청은 P4의 `control_rpcs_per_instance` 상한을 점유하고 caller context 및 남은 전역 startup deadline을 따릅니다. `START` 성공 후 호스트가 `RUNNING`을 commit할 때만 대기를 풀고 권한, scope와 상태를 다시 검사하여 작업을 받아들입니다. `START` 실패, 취소 또는 drain 전환은 대기 요청을 실패시키며 다른 허용되지 않은 상태를 이 gate로 우회하지 않습니다. `START` callback은 worker를 시작한 뒤 반환해야 하고 그 worker의 `RUNNING` 전용 RPC 완료를 동기적으로 기다려서는 안 됩니다. 이를 위한 새 RPC나 자동 retry는 추가하지 않습니다.

이미 admission을 통과한 각 이벤트를 실제 전달할 때 runtime은 opaque work reference를 만들고 RPC metadata `flexmodule-work-ref`에 넣습니다. SDK는 그 callback context에서 호출하는 helper에만 이를 전달합니다. 서버는 인증된 `InstanceID`, 진행 중인 해당 delivery, global drain deadline, 작업별 drain-safe 정책을 확인하며 callback 반환 또는 취소 때 참조를 폐기합니다. queued-before-drain 이벤트에도 실제 전달 시 참조를 발급합니다.

work reference는 identity가 아니며 `module-id` metadata를 신뢰하는 장치가 아닙니다. 같은 불신 프로세스 안에서 실제 call-stack 인과관계를 증명하지 못하므로, 보장은 활성 callback에 연결된 유한 작업을 기존 grants 안에서 허용하는 것으로 한정합니다. DRAINING 중 새 upload, playback job, receive subscription과 장기 작업은 이 참조가 있어도 거부합니다. 기존 RPC는 deadline 안에서 마칠 수 있고 자신의 자원 정리 작업은 계속 허용합니다. 새 유한 helper는 활성 event work reference가 있어야 합니다.

#### 4.3.5 파일 전송과 소유권

첫 `UploadChunk`에는 metadata만 있고 뒤에는 offset이 증가하는 data chunk만 있습니다. declared size를 받으면 저장 공간과 동시 업로드 자리를 먼저 예약합니다. chunk 기본 최대 크기는 256 KiB이고 파일당 한도는 500 MiB입니다. declared size 0인 파일은 metadata 뒤 EOF로 완료할 수 있지만 일반 data chunk의 빈 payload로 진척을 가장하지 않습니다.

동시 업로드는 인스턴스당 2개, 호스트당 8개이며 저장 총량은 각각 2 GiB, 8 GiB입니다. 진행 중 예약과 아직 회수하지 못한 leased bytes를 모두 계산합니다. offset 불일치, 길이 초과, 조기 EOF, context 취소는 partial spool을 제거하고 예약을 반환합니다. 성공적으로 commit한 뒤에만 `UploadedFileRef`를 반환합니다. client가 최종 응답을 잃으면 서버가 이미 commit했을 수 있으며 같은 전송의 자동 재시도나 exactly-once commit을 약속하지 않습니다.

완료한 ref의 TTL은 완료 시점부터 30분입니다. 삭제와 만료는 새 lease를 금지하지만 사용 중인 reader가 닫히거나 취소되기 전에는 파일을 unlink하거나 바이트 사용량을 조기에 반환하지 않습니다. 인스턴스 종료는 새 작업을 막고 lease를 취소한 뒤 실제 정리를 수행합니다. 원래 `expires_at` 전에 삭제한 ref는 같은 인스턴스가 유효하고 현재 시각이 그 원래 만료 시각보다 이른 동안 반복 삭제에 성공해야 합니다. 파일 바이트를 이미 회수했어도 opaque ID, 소유 인스턴스와 원래 만료 시각을 담은 tombstone으로 이 멱등성을 유지합니다. 삭제나 반복 호출은 TTL을 연장하지 않습니다.

tombstone은 원래 `expires_at` 도달 또는 인스턴스 종료 중 먼저 발생한 때 제거합니다. 자연 만료에는 새 30분 tombstone 기간을 만들지 않으며, 아직 열린 lease의 내부 정리 레코드만 실제 회수까지 유지합니다. 보존 기간이 끝난 참조와 식별 불가능한 참조는 `NOT_FOUND`이고 삭제 성공 또는 새 읽기를 허용하지 않습니다. 다른 owner의 참조는 존재를 공개하지 않는 소유권 오류로 거부하며, tombstone도 새 lease를 허용하지 않습니다.

바이트 quota와 별도로 호스트 및 인스턴스별 업로드 레코드 개수 상한을 운영자가 유한한 값으로 반드시 설정합니다. 이는 무제한 metadata 누적을 막기 위한 기술 계약이며 사용자 확인 숫자 기본값은 없습니다. 업로드 시작 전에 바이트와 동시성 예약에 더해 레코드 자리 하나를 원자적으로 예약하고, `RECEIVING`, 완료/회수 대기 레코드 및 삭제 tombstone을 모두 같은 개수에 포함합니다. 파일 크기가 0이거나 바이트가 이미 회수되어도 tombstone이 남으면 개수를 반환하지 않습니다. 정리 레코드가 필요하면 만료 이후에도 실제 정리가 끝날 때까지 그 자리를 유지합니다. 유효한 tombstone을 새 업로드를 위해 조기 퇴거하지 않으며, 어느 개수 상한이든 도달하면 부분 예약 없이 `RESOURCE_EXHAUSTED`와 `QUOTA_EXCEEDED`로 거부합니다.

Discord attachment의 허용 크기 및 개수는 destination과 endpoint 규칙을 별도로 확인합니다. 500 MiB는 FlexModule 저장 한도이지 Discord 송신 보장이 아닙니다. P3는 현재 전체 multipart body를 `bytes.Buffer`에 담는 경로를 bounded streaming multipart로 바꾸고 context 취소가 HTTP와 writer, pipe, reader까지 닿도록 구현합니다. spool-backed reader만 사용한다고 메모리 한도가 해결된 것으로 보지 않습니다.

## 5. API Design

### 5-1. New / Modified

#### 공통 gRPC 서비스

다음은 코어 서비스의 전체 method set입니다. 실제 generated interface는 고정한 protobuf/grpc-go 도구로 생성하며, 애플리케이션 Go interface의 context와 오류 규칙을 바꾸지 않습니다. 현재 `proto/buf.gen.yaml`에 있는 `require_unimplemented_servers=false` 설정을 v2 생성에도 명시적으로 유지하여 모든 method 구현을 concrete server에 요구합니다.

```proto
service Hook {
  // Returns immutable artifact metadata and requested capabilities.
  // The runtime validates it before INIT; this call must not activate the module.
  rpc GetManifest(google.protobuf.Empty) returns (ModuleManifest);

  // Reports module readiness without asserting the authoritative host state.
  // The caller's deadline bounds startup and status collection.
  rpc GetStatus(google.protobuf.Empty) returns (Status);

  // Executes one lifecycle stage and returns its actual error.
  // INIT carries InitContext; other stages must omit it.
  rpc OnStage(StageRequest) returns (google.protobuf.Empty);

  // Handles exactly one admitted typed event in the instance's processing lane.
  // Success acknowledges callback completion, not durable or exactly-once processing.
  rpc OnEvent(EventEnvelope) returns (google.protobuf.Empty);
}

service Core {
  // Returns only this authenticated module's structured JSON configuration.
  // It must not expose host environment variables or runtime-owned credentials.
  rpc GetConfig(google.protobuf.Empty) returns (JSONValue);
}

service FileTransfer {
  // Accepts metadata followed by ordered chunks and returns only a committed ref.
  // Cancellation aborts an uncommitted upload; a lost final response may hide a commit.
  rpc Upload(stream UploadChunk) returns (UploadedFileRef);

  // Prevents new reads of an owned upload and reclaims it after active leases finish.
  // Repeated deletion succeeds only before the original expires_at while the
  // owning instance remains valid, even after bytes are reclaimed into a tombstone.
  // Deletion never extends that window; expired or unknown refs return NOT_FOUND.
  rpc DeleteUpload(DeleteUploadRequest) returns (google.protobuf.Empty);
}
```

config는 module-scoped JSON object이며 string map으로 평탄화하지 않습니다. 운영자가 구조화하여 제공한 일반 설정만 포함합니다. 자격증명 설정은 비밀이 아닌 binding 이름까지만 담을 수 있으며 실제 typed reference는 아래의 `GetProvisionedCredentialRef`로 얻습니다. config에 원문 secret을 넣거나 임의 JSON 문자열을 credential resolver 입력으로 해석하지 않습니다.

#### Go hook과 runtime 경계

```go
// ModuleHooks is the complete module-facing lifecycle and event contract.
// Implementations must honor cancellation and return failures to the runtime.
// Calls for one instance obey the lifecycle and single event-lane rules.
type ModuleHooks interface {
    GetManifest(ctx context.Context) (*corev2.ModuleManifest, error)
    GetStatus(ctx context.Context) (*corev2.Status, error)
    OnStage(ctx context.Context, req *corev2.StageRequest) error
    OnEvent(ctx context.Context, event *corev2.EventEnvelope) error
}

// CoreClient exposes instance-scoped runtime facilities to a module.
// It never accepts an identity parameter or returns host environment variables.
type CoreClient interface {
    GetConfig(ctx context.Context) (*corev2.JSONValue, error)
}

// InstancePrincipal is created by the trusted host from the authenticated broker.
// Neither field may be populated from caller-supplied RPC metadata or payloads.
type InstancePrincipal struct {
    ModuleID   ModuleID
    InstanceID InstanceID
}

// PrincipalFromContext returns the principal established for this connection.
// It fails when authentication is missing or the instance has been retired.
func PrincipalFromContext(ctx context.Context) (InstancePrincipal, error)

// Launch creates a fresh isolated instance and completes INIT, readiness and START.
// A returned instance is RUNNING; failure releases temporary resources only.
func Launch(ctx context.Context, spec ModuleSpec) (*Instance, error)

// Stop drains and terminates the installed module's current instance idempotently.
// It preserves the module's persistent directory and does not schedule a restart.
func Stop(ctx context.Context, id ModuleID) error
```

`ModuleSpec`, `Instance`와 host cleanup의 구체적인 구조는 P4의 호스트 API이며 wire에 직렬화하지 않습니다. 새 helper는 모두 첫 인자로 `context.Context`를 받습니다. module hook, generated RPC wrapper, backend adapter, Discord HTTP 및 streaming writer까지 같은 취소를 전달하고 중간에 `context.Background()`로 대체하지 않습니다. 모든 lifecycle 및 helper 오류를 호출자에게 전달합니다.

#### Credential projection과 안전한 SDK view

```go
// CredentialBinding describes one secret's permitted use and finite lifetime.
// The owner is obtained from ctx, never from these caller-visible fields.
type CredentialBinding struct {
    BackendID     string
    Purpose       string
    ResourceScope *corev2.ResourceSelector
    ExpiresAt     time.Time
}

// CredentialTarget contains only the purpose and exact source-derived resource
// selector. It never contains a raw token or caller-invented resource IDs.
type CredentialTarget struct {
    Purpose       string
    ResourceScope *corev2.ResourceSelector
}

// ProjectionPolicy is supplied by trusted P3 operation policy. It returns the
// authorized binding, including a future finite expiry, without receiving a secret.
type ProjectionPolicy func(context.Context, CredentialTarget) (CredentialBinding, error)

// CredentialUse describes the exact operation binding expected by an adapter.
// It is constructed by trusted operation policy, not copied from the ref.
type CredentialUse struct {
    BackendID     string
    Purpose       string
    ResourceScope *corev2.ResourceSelector
}

// CredentialEncoder exists only on the runtime side of the trust boundary.
// It stores the raw secret for the authenticated destination instance and returns
// a typed opaque ref; it must not log or embed the raw secret in diagnostics.
type CredentialEncoder interface {
    EncodeCredential(ctx context.Context, binding CredentialBinding, rawSecret string) (*corev2.CredentialRef, error)
}

// CredentialResolver is host-only and is never exposed as a module RPC.
// It checks owner, purpose, resource, expiry and instance state before returning
// a secret for the immediate trusted adapter call.
type CredentialResolver interface {
    ResolveCredential(ctx context.Context, ref *corev2.CredentialRef, expected CredentialUse) (string, error)
}

// InteractionHandle contains a safe SDK snapshot and a separate typed credential.
// Value.Token is always empty; Credential is not a Discord token string.
type InteractionHandle struct {
    Value      *discordgo.Interaction
    Credential *corev2.CredentialRef
}

// WebhookHandle preserves the same separation for webhook-returning operations.
// An absent credential does not authorize token-based webhook operations.
type WebhookHandle struct {
    Value      *discordgo.Webhook
    Credential *corev2.CredentialRef
}

// ProjectInteraction recursively sanitizes a runtime-owned source interaction.
// It derives the exact target from source IDs and obtains a binding from policy.
// Backend, purpose, scope and future finite expiry are validated before encoding.
// The unchanged authenticated ctx selects the destination; only encoder sees the secret.
func ProjectInteraction(ctx context.Context, src *discordgo.Interaction, encoder CredentialEncoder, policy ProjectionPolicy) (*discordv2.Interaction, error)

// ProjectWebhook applies the same policy and validation to source webhook IDs.
// A token-free source stays token-free; raw tokens and credential-bearing URLs
// never reach policy, module DTOs or diagnostics.
func ProjectWebhook(ctx context.Context, src *discordgo.Webhook, encoder CredentialEncoder, policy ProjectionPolicy) (*discordv2.Webhook, error)

// ProtoToInteractionHandle reconstructs a safe local convenience view.
// It never resolves a ref, populates Token, or invents missing partial members.
func ProtoToInteractionHandle(src *discordv2.Interaction) (*InteractionHandle, error)

// ProtoToWebhookHandle reconstructs a safe webhook view with Token left empty.
// The returned ref remains subject to runtime authorization on every use.
func ProtoToWebhookHandle(src *discordv2.Webhook) (*WebhookHandle, error)
```

wire `Interaction`과 `Webhook`에는 `token` 대신 `core.CredentialRef credential`을 둡니다. `GuildMemberAddParams`에는 `access_token` 대신 `core.CredentialRef access_credential`을 둡니다. 전체 안전 domain field는 기존 source와 reachable field 목록대로 보존합니다. 이 변경은 단순 token string rename이 아니라 별도 vault 및 resolver를 사용하는 신뢰 경계 변경입니다.

`ProjectionPolicy`는 P3의 신뢰된 작업 정책이 제공합니다. projection은 실제 source의 `interaction_id` 또는 `webhook_id`와 존재하는 `application_id`, `guild_id`, `channel_id`로 정확한 `CredentialTarget.ResourceScope`를 구성하며 사용자 입력으로 식별자를 만들어 보완하지 않습니다. policy에는 이 target과 변경하지 않은 인증 context만 전달하고 raw secret은 encoder에만 전달합니다. 반환 binding의 backend, purpose 및 정확한 selector가 target과 신뢰된 작업에 맞고 `ExpiresAt`이 유한한 미래 시각인지 검증한 뒤 발급합니다. 만료 시각을 converter 기본값이나 신뢰할 수 없는 ref metadata에서 가져오지 않습니다. 대상 인스턴스의 인증과 승인은 P4의 context 경계를 따르며 3인자 호환 overload나 alias는 두지 않습니다.

일반 pure converter는 `XToProto(src) (value, error)`와 `ProtoToX(src) (value, error)` 형태입니다. nil 입력은 nil 결과로 처리할 수 있지만 unknown variant, 범위 초과, credential projection 누락을 nil 성공으로 바꾸지 않습니다. interaction 및 webhook처럼 secret이 섞이는 값은 위 전용 projection/view를 사용합니다. Patch 및 audit log는 존재성이나 정확한 숫자를 표현할 수 있는 별도 view를 사용합니다.

#### 전체 Discord 작업 매핑 규칙

작업 계약의 기준은 현재 source signature 232개를 모두 분류한 catalog입니다. 기존 감사의 229개 목록만으로 닫지 않으며 API 이름만 나열하고 입력이나 반환 타입의 전송 가능 여부를 생략하지 않습니다. 새 `GuildRole`, `GuildRoleMemberCounts`, `UserVoiceState`와 pagination 응답으로 바뀐 `ChannelMessagesPinned`도 P3의 typed 계약으로 포함합니다.

| source 형태 | wire 및 Go 계약 |
|---|---|
| receiver `*Session` | 제거합니다. host의 승인된 Discord session만 사용합니다. |
| canonical `Method(a, b, data, options...)` | `MethodRequest`의 이름 있는 field로 변환합니다. source group parameter는 각각 분리하고 이름을 snake_case로 정규화합니다. domain payload는 nested DTO로 유지합니다. |
| scalar 또는 pointer argument | 명시적 폭 및 optional/nullable 규칙을 적용합니다. nil이 동작을 의미하는 alias는 단순 absent로 바꾸지 않습니다. |
| slice, map, variadic domain argument | source의 순서를 유지합니다. nil/empty가 의미를 가지면 present wrapper를 사용하며 list body를 개별 부수 호출로 임의 분해하지 않습니다. |
| `RequestOption` | `WithContext`는 RPC context로 전달하고, `WithAuditLogReason`과 `WithLocale`은 각각 `RequestOptions.audit_log_reason`과 `RequestOptions.locale`로 보존합니다. URL, 임의 header, authorization, client, bucket 및 caller retry injection은 허용하지 않습니다. |
| `(T, error)` | `MethodResponse`의 `value`와 gRPC status로 분리합니다. list 결과는 concrete list wrapper를 사용합니다. 여러 non-error 결과가 있으면 각 결과에 이름 있는 field를 둡니다. 예외적으로 `GatewayBot`은 공개 URL, shards와 전체 `SessionStartLimit`을 가진 기존 도메인 `GatewayBotResponse`를 직접 반환하며, 같은 이름의 wrapper나 재귀적인 `value` 필드를 만들지 않습니다. |
| `error`만 반환 | 성공은 `Empty`, 실패는 공통 typed status입니다. |
| error를 숨긴 source method | v2는 오류 반환 계약을 추가합니다. `GuildTemplateCreate`의 source 반환 형태 때문에 HTTP 실패를 nil 성공으로 재현하지 않습니다. |
| token parameter 또는 token이 든 interaction | typed `CredentialRef` 또는 `InteractionHandle` 인자로 바꿉니다. SDK 편의 함수도 raw token 문자열을 받지 않습니다. |
| `io.Reader` | 이미 완료한 `UploadedFileRef`로 바꿉니다. 별도 `Upload` 없이 helper 안에서 숨은 업로드를 시작하지 않습니다. |
| convenience alias | canonical request를 만드는 SDK 편의 매핑만 둡니다. 중복 RPC나 v1 호환 경로는 만들지 않습니다. |

신규 request field 번호는 canonical source 인자의 순서대로 1부터 부여하고 `RequestOptions`는 1000을 사용합니다. alias만 필요한 추가 canonical 옵션은 1001부터 문서에 고정합니다. 예를 들어 `GuildWithCounts`를 `Guild`에 매핑하기 위한 `with_counts`는 1001이며 원래 `Guild` 호출은 false를 사용합니다. 이미 명시한 특수 credential 및 file field는 위 경계 타입으로 치환합니다. 한 번 고정한 field 번호는 삭제 뒤 다른 의미로 재사용하지 않습니다.

canonical signature에 별도 `reason` 인자가 있으면 source 인자 순서에 따른 field 번호와 이름을 유지하고, `optional string reason`으로 미지정과 명시적 빈 값을 구분합니다. 이를 `RequestOptions.audit_log_reason`으로 옮기거나 뒤 인자의 번호를 당기지 않습니다. reason field와 `RequestOptions.audit_log_reason`이 동시에 지정되면 빈 문자열을 포함해 동일한 값이어야 합니다. 서로 다르면 `INVALID_ARGUMENT`으로 거부하고, 둘 중 하나만 지정되면 그 값을 사용합니다. reason 없는 `GuildBanCreate`와 `GuildMemberDelete` 편의 호출은 canonical reason field를 미지정으로 둡니다. audit option이 있으면 그 값을 적용하고, 둘 다 없을 때만 원본 SDK의 유효 기본값인 빈 문자열을 사용합니다. 따라서 `WithAuditLogReason("x")`를 전달한 reason 없는 편의 호출도 지원합니다. 지원하지 않는 작업에 audit reason을 지정해도 조용히 버리지 않고 거부합니다.

`RequestOptions`는 아래 두 field로 제한합니다. `locale`이 없으면 locale header를 지정하지 않고, 있으면 source의 문자열 기반 `Locale` 값을 보존하여 고정된 `X-Discord-Locale` header에만 적용합니다. 이는 기존 `WithLocale` 동작을 유지하는 typed REST metadata이며 임의 `WithHeader`를 허용하는 기능이 아닙니다.

```proto
message RequestOptions {
  optional string audit_log_reason = 1;
  optional string locale = 2;
}

message ChannelMessageSendComplexRequest {
  string channel_id = 1;
  MessageSend data = 2;
  RequestOptions options = 1000;
}

message ChannelMessageSendComplexResponse { Message value = 1; }

message GuildMemberAddRequest {
  string guild_id = 1;
  string user_id = 2;
  GuildMemberAddParams data = 3;
  RequestOptions options = 1000;
}

message WebhookThreadExecuteRequest {
  string webhook_id = 1;
  flexmodule.core.v2.CredentialRef credential = 2;
  bool wait = 3;
  string thread_id = 4;
  WebhookParams data = 5;
  RequestOptions options = 1000;
}

message WebhookThreadExecuteResponse { Message value = 1; }

message InteractionRespondRequest {
  Interaction interaction = 1;
  InteractionResponse resp = 2;
  RequestOptions options = 1000;
}
```

`wait=false`인 webhook 실행처럼 source/API가 결과 body 없이 성공하는 경우에만 응답 `value`의 absence가 정상입니다. 값이 필수인 조회의 누락 결과를 all-nil 성공으로 처리하지 않습니다. `InteractionRespondRequest.interaction`의 ID, app ID와 credential binding을 대조하며 token이 비어 있는 SDK view만으로 callback 권한을 만들지 않습니다.

```go
// ChannelMessageSendComplex submits one typed message operation after authorization.
// It may reference only committed uploads owned by this authenticated instance.
// Cancellation after dispatch does not prove that Discord rejected the message.
func (c *DiscordClient) ChannelMessageSendComplex(ctx context.Context, req *discordv2.ChannelMessageSendComplexRequest) (*discordv2.ChannelMessageSendComplexResponse, error)

// InteractionRespond sends the initial interaction callback using an opaque ref.
// It must not auto-defer, retry, or substitute a webhook credential with wider scope.
func (c *DiscordClient) InteractionRespond(ctx context.Context, req *discordv2.InteractionRespondRequest) error

// Upload explicitly transfers a local reader into instance-scoped temporary storage.
// src is consumed locally by the SDK and is never serialized as a remote object.
func (c *FileClient) Upload(ctx context.Context, meta *corev2.UploadMetadata, src io.Reader) (*corev2.UploadedFileRef, error)
```

대표 편의 매핑은 다음과 같으며 나머지 source alias도 같은 수준의 인자와 기본값 대응을 목록에 유지합니다.

| source 편의 메서드 | canonical 작업과 의미 |
|---|---|
| 메시지 Send/TTS/Embed/Embeds/Reply family | `ChannelMessageSendComplex`; 실제 content, TTS, embeds, reference를 채웁니다. reply의 nil reference는 오류입니다. |
| 메시지 Edit/Embed/Embeds family | `ChannelMessageEditComplex`; present content와 embeds를 만들며 빈 embeds는 clear입니다. |
| `ChannelFileSend`, `ChannelFileSendWithMessage` | `ChannelMessageSendComplex`; file 인자는 명시적으로 사전 업로드한 ref로 바꿉니다. |
| `GuildBanCreate`, `GuildMemberDelete` | 각각 `GuildBanCreateWithReason`, `GuildMemberDeleteWithReason`; canonical optional reason은 미지정으로 두고 audit option이 있으면 그 값을 사용합니다. 둘 다 없을 때만 유효 기본값이 `""`입니다. |
| member move/mute/deafen/timeout | `GuildMemberEdit`; false와 null을 present로 유지하고 나머지 field는 unset입니다. |
| `WebhookExecute` | `WebhookThreadExecute`; thread ID를 빈 값으로 둡니다. |
| InteractionResponse와 Followup family | `WebhookMessage`, `WebhookMessageEdit`, `WebhookMessageDelete`, `WebhookThreadExecute`; interaction-purpose ref와 app/message binding을 유지합니다. `@original`은 허용된 interaction 작업에만 사용합니다. |
| 상태 편의 메서드 | `UpdateStatusComplex`; source 기본값과 activity 종류를 보존합니다. shared bot presence를 여러 모듈이 last-write-wins로 덮어쓰지 못하도록 단일 승인 소유 정책을 적용합니다. |
| guild member Batch/BatchList | 각각 단일-guild canonical 메서드를 입력 순서로 호출합니다. 빈 목록은 호출하지 않고 실패/취소 뒤 나머지는 보내지 않습니다. 이미 요청한 guild의 결과를 rollback하거나 batch 전체를 자동 재시도하지 않습니다. |

3개 typed Gateway 작업은 `UpdateStatusComplex`, `RequestGuildMembers`, `RequestGuildMembersList`입니다. member 요청의 성공은 Gateway write 성공이며 모든 chunk 도착 완료를 뜻하지 않습니다. request correlation은 인스턴스와 guild에 묶고 외부 nonce가 다른 모듈 요청의 소유권을 바꾸지 못하게 합니다. `UserChannelPermissions`는 typed 상태 helper이며 사용자가 제공한 cache/권한 숫자를 신뢰하지 않습니다.

`Gateway`와 `GatewayBot`은 이 세 Gateway 송신 작업과 달리 읽기 전용 REST metadata 작업입니다. 공개 Gateway base URL과 안전한 shard/start-limit 정보의 조회를 허용하되, 조회 결과로 모듈이 host session의 `Open`, `Close`, `Identify`, resume 또는 voice transport를 제어할 수는 없습니다.

API 포함 및 제외의 중요한 경계는 다음과 같습니다.

- `oauth2.go`의 `Application`도 `appID="@me"`인 공식 bot application 조회로 포함합니다. 임의 앱 소유자 조회로 확장하지 않습니다. application role connection metadata 조회 및 갱신은 사용자 role connection과 달리 봇 작업으로 포함합니다.
- `GuildMemberAdd`는 runtime bot 인증과 운영자가 제공한 사용자 `guilds.join` 참조를 함께 쓰는 봇 작업으로 포함합니다.
- 사용자 connections, current-user guild member, user role connection, application 소유자 관리, invite accept와 bearer-only command permission 편집은 제외합니다. command permission 조회와 `DefaultMemberPermissions` 보존은 유지합니다.
- 앱 guild 생성과 template 생성, batch command permission 편집, 현재 공식 봇 경로가 아닌 legacy integration 생성/편집 및 channel별 active-thread 경로는 제외 이유와 공식 근거를 목록에 남깁니다. 더 넓은 범용 요청으로 우회하지 않습니다.
- `Request`, `RequestRaw`, bucket/locked-bucket 전송, `GatewayWriteStruct`는 제외합니다. `Open`, `Close`, handler 등록과 voice join의 라이브 객체는 host 내부 책임으로 분류합니다. 안전한 `Gateway`/`GatewayBot` metadata 조회는 이 제외에 포함하지 않습니다. voice join/leave의 기능은 P5의 소유 job과 subscription으로 제공하며 connection 객체를 되돌려 주지 않습니다.
- command create/edit/bulk overwrite를 제공해도 소유하지 않은 다른 모듈의 command를 삭제하거나 같은 `(application, guild/global scope, type, name)`을 조용히 덮어쓰지 않습니다. P3의 ownership 검증을 거치며 Discord end-user permission field를 제거하지 않습니다.

모든 canonical server method에는 실제 구현과 per-operation 검증이 있어야 합니다. concrete server는 전체 generated server interface를 compile-time assertion으로 만족해야 하며 `Unimplemented` 또는 `Unsafe` server embedding으로 구현 누락을 숨기지 않습니다. service descriptor의 모든 method를 열거한 검증과 작업별 behavioral probe가 실제 concrete handler dispatch를 확인해야 합니다. all-nil client로 인터페이스만 맞추는 것도 수용하지 않습니다.

#### 새 interaction routing 및 운영자 credential binding helper

이 세 helper는 SDK 메서드의 누락분을 대신하는 범용 호출이 아니라, 여러 모듈이 하나의 봇을 공유할 때 필요한 이름 있는 소유권 경계입니다. Discord namespace의 schema와 service method는 다음과 같습니다.

```proto
enum InteractionRouteKind {
  INTERACTION_ROUTE_KIND_UNSPECIFIED = 0;
  INTERACTION_ROUTE_KIND_MESSAGE_COMPONENT = 1;
  INTERACTION_ROUTE_KIND_MODAL_SUBMIT = 2;
}

message InteractionRouteClaim {
  string application_id = 1;
  InteractionRouteKind interaction_kind = 2;
  string custom_id = 3;
}

message RegisterInteractionRoutesRequest {
  repeated InteractionRouteClaim claims = 1;
}

message UnregisterInteractionRoutesRequest {
  repeated InteractionRouteClaim claims = 1;
}

message GetProvisionedCredentialRefRequest {
  string binding_name = 1;
}

service BackendOwnership {
  // Atomically claims exact component or modal-root routes for the stable ModuleID.
  // Repeating an identical owned claim succeeds; another owner's claim is rejected.
  rpc RegisterInteractionRoutes(RegisterInteractionRoutesRequest)
      returns (google.protobuf.Empty);

  // Atomically removes only routes owned by the authenticated stable ModuleID.
  // A route owned by another module must never be removed as a side effect.
  rpc UnregisterInteractionRoutes(UnregisterInteractionRoutesRequest)
      returns (google.protobuf.Empty);

  // Returns an instance-bound ref for an operator-provisioned named binding.
  // It neither accepts nor exports a raw secret and cannot reference the bot token.
  rpc GetProvisionedCredentialRef(GetProvisionedCredentialRefRequest)
      returns (flexmodule.core.v2.CredentialRef);
}
```

`InteractionRouteKind`는 routing용 enum이며 Discord `InteractionType` 숫자를 그대로 복제한 enum이 아닙니다. 정확한 `(application_id, interaction_kind, custom_id)`를 key로 사용합니다. 등록할 때 전체 batch의 형태, app scope, quota와 소유권을 먼저 확인하고 모두 성공한 경우에만 반영합니다. 같은 안정적 `ModuleID`가 같은 claim을 반복하면 성공하고 다른 모듈 소유 claim은 `ALREADY_EXISTS`입니다. wildcard, prefix routing 또는 custom ID 자동 변경은 하지 않습니다.

route metadata의 소유자는 안정적인 `ModuleID`이고 실제 전달 대상은 grants를 통과하여 활성화된 현재 인스턴스입니다. INIT에서 정적 route를 등록하고 RUNNING에서 동적 route를 추가할 수 있습니다. 모듈이 interactive button/select 또는 modal을 외부에 보내기 전에 해당 custom ID를 소유해야 합니다. modal root ID는 route이고 text-input child ID는 route가 아닙니다. 명령 및 autocomplete는 command 소유권, component와 modal은 이 route 소유권으로 수신자를 정한 뒤 해당 모듈용 credential ref를 발급합니다. 다른 모듈에게 interaction token의 사용 권한이 생기는 broadcast를 하지 않습니다.

route는 재시작에도 보존하며 기본 TTL을 두지 않습니다. 명시적 unregister 또는 uninstall로 해제하고 유한한 운영자 설정 quota를 적용합니다. unregister는 전체 claim에 대해 타 모듈 소유권 위반이 없는지 먼저 확인하며 이미 없는 claim은 추가 부작용 없이 처리합니다. DRAINING에서는 새 claim을 만들지 않으며 자신의 route 해제는 정리 작업으로 허용합니다.

`GetProvisionedCredentialRef`는 운영자가 해당 `ModuleID`에 미리 연결한 binding만 찾습니다. `discord.credentials.reference`와 binding scope를 검사한 뒤 INIT 또는 RUNNING에서 현재 `InstanceID`에 새 ref를 발급합니다. 임의 app/user/guild를 입력하여 binding을 바꾸거나 원문 사용자 token을 업로드하는 API가 아닙니다. DRAINING에서 새 ref 발급을 거부하고, 같은 binding을 재시작 뒤 쓰려면 새 인스턴스에서 다시 발급받아야 합니다. purpose, 실제 리소스와 만료 검사 후 사용하는 원칙은 모든 credential과 같습니다.

```go
// RegisterInteractionRoutes claims the complete batch or changes nothing.
// The stable owner comes from the authenticated context, never from req.
func (c *DiscordClient) RegisterInteractionRoutes(ctx context.Context, req *discordv2.RegisterInteractionRoutesRequest) error

// UnregisterInteractionRoutes releases only owned routes and never rewrites IDs.
// It remains available as an owned cleanup operation during drain.
func (c *DiscordClient) UnregisterInteractionRoutes(ctx context.Context, req *discordv2.UnregisterInteractionRoutesRequest) error

// GetProvisionedCredentialRef obtains a typed reference to an approved binding.
// The runtime alone resolves the secret; the result expires with its instance.
func (c *DiscordClient) GetProvisionedCredentialRef(ctx context.Context, req *discordv2.GetProvisionedCredentialRefRequest) (*corev2.CredentialRef, error)
```

### 5-2. Error Handling

FlexModule은 REST 서버가 아니므로 별도의 HTTP 400/500 계약을 만들지 않습니다. 외부 오류는 gRPC status와 typed detail로 전달하며 Discord의 실제 HTTP status는 원본 의미를 가진 backend detail에만 보존합니다.

```proto
enum ErrorOrigin {
  ERROR_ORIGIN_UNSPECIFIED = 0;
  ERROR_ORIGIN_FLEXMODULE = 1;
  ERROR_ORIGIN_BACKEND = 2;
  ERROR_ORIGIN_TRANSPORT = 3;
}

enum RemoteOutcome {
  REMOTE_OUTCOME_UNSPECIFIED = 0;
  REMOTE_OUTCOME_NOT_SENT = 1;
  REMOTE_OUTCOME_RESPONSE_RECEIVED = 2;
  REMOTE_OUTCOME_UNKNOWN = 3;
}

message FieldViolation {
  string field_path = 1;
  string reason = 2;
  string description = 3;
}

message ErrorDetail {
  ErrorOrigin origin = 1;
  string reason = 2;
  string operation = 3;
  string backend_id = 4;
  RemoteOutcome remote_outcome = 5;
  repeated FieldViolation violations = 6;
  google.protobuf.Any backend_detail = 7;
}
```

Discord 전용 detail은 Discord namespace에 둡니다.

```proto
message DiscordErrorDetail {
  int32 http_status = 1;
  optional int64 api_code = 2;
  google.protobuf.Duration retry_after = 3;
  repeated flexmodule.core.v2.FieldViolation field_errors = 4;
}
```

| gRPC status | 대표 reason 및 의미 |
|---|---|
| `UNAUTHENTICATED` | 인증된 broker principal이 없거나 사용할 수 없는 연결입니다. payload ID로 보완하지 않습니다. |
| `PERMISSION_DENIED` | capability, scope 또는 credential 소유권이 맞지 않습니다. foreign ref의 존재와 owner를 공개하지 않습니다. FlexModule 거절은 `origin=FLEXMODULE`이고 Discord 403은 `origin=BACKEND`, `backend_id=discord`로 구분합니다. |
| `FAILED_PRECONDITION` | `PROTOCOL_MISMATCH`, `REQUIRED_CAPABILITY_DENIED`, `INSTANCE_NOT_RUNNING`, 잘못된 stage, 다른 채널을 사용 중인 voice 예약 등 현재 상태가 허용하지 않습니다. |
| `INVALID_ARGUMENT` | malformed JSON, presence 및 oneof 모순, `UNSUPPORTED_VARIANT`, `UPLOAD_SEQUENCE_INVALID`, `UPLOAD_SIZE_MISMATCH`, 허용하지 않은 옵션 또는 null입니다. |
| `OUT_OF_RANGE` | `NUMERIC_OUT_OF_RANGE`, `UPLOAD_TOO_LARGE`, `CHUNK_TOO_LARGE` 등 표현 및 고정 한도를 넘었습니다. |
| `RESOURCE_EXHAUSTED` | `QUOTA_EXCEEDED`, 이벤트 큐 overflow 또는 유한 buffer 한도입니다. Discord 429는 backend origin과 실제 retry-after를 보존합니다. |
| `NOT_FOUND` | 자신의 `REF_NOT_FOUND`, 이미 만료되어 새 사용이 불가능한 `REF_EXPIRED`, 또는 Discord 404입니다. backend와 core 원인을 구분합니다. |
| `ALREADY_EXISTS` | 이미 다른 소유자가 가진 command key 등 충돌입니다. 조용한 overwrite로 해결하지 않습니다. |
| `CANCELLED`, `DEADLINE_EXCEEDED` | caller 취소 또는 deadline 만료입니다. 이미 Discord에 보냈다면 `remote_outcome=UNKNOWN`일 수 있습니다. |
| `UNAVAILABLE` | upstream 또는 broker transport를 사용할 수 없습니다. 자동 재시도 허가를 뜻하지 않습니다. |
| `INTERNAL` | 반환값 불변조건 위반, 알려진 이벤트의 안전한 encode 실패, 예상하지 못한 구현 오류입니다. 비밀 없는 설명으로 전달하며 해당 lifecycle 정책에 따라 실패시킵니다. |

`remote_outcome=RESPONSE_RECEIVED`는 응답을 받았다는 뜻일 뿐 원격 변경이 rollback되었거나 재전송이 안전하다는 뜻이 아닙니다. HTTP 5xx나 연결 단절 시 부작용이 없었다고 단정하지 않습니다. permission 오류와 일반 사용자 콘텐츠를 포함하는 상세 오류는 알려진 안전 field만 정규화하며 raw JSON body, 헤더, 토큰 포함 URL을 전달하지 않습니다.

Discord HTTP 400 계열의 잘못된 요청, 403 권한, 404 자원, 429 rate limit 등은 실제 status와 API code를 보존하여 P3의 작업별 mapping으로 연결합니다. 사용자 인증이 아닌 runtime bot credential 자체의 401 문제는 모듈 재인증을 요구하는 것으로 바꾸지 않고 backend credential 실패로 보고합니다. RPC가 반환할 상태와 downstream outcome을 분리합니다.

retry-after는 caller가 상황을 판단하기 위한 정보입니다. helper가 자동 defer, upload, 재시도 또는 비멱등 쓰기 재전송을 추가하지 않습니다. 기존 DiscordGo의 내부 retry도 P3에서 명시적으로 비활성화하거나 승인된 유한 동작으로 제어하며 file stream을 재사용할 때는 매 시도마다 새 lease/reader를 획득해야 합니다. 이 제안은 추가 retry subsystem이나 idempotency key 보장을 도입하지 않습니다.

startup 30초, drain 10초, 이벤트 256개/32 MiB, 업로드의 확인된 기본값 외에 필요한 RPC 최대 bytes, JSON nesting/bytes, decoder recursion, backend ingress와 voice buffer 한도는 운영자가 유한값을 설정해야 합니다. 설정이 빠지면 무제한으로 실행하지 않습니다. 새로운 숫자를 사용자 승인 기본값처럼 제시하지 않습니다.

## 6. Implementation Plan

### 6-1. Milestones

각 단계는 입력, 산출물과 수용 기준을 자체적으로 설명합니다. 아래 단계들은 실제로 선후 의존성이 있으므로 독립 배포 가능하다고 주장하지 않습니다.

| Phase | Task | Estimated Duration | Owner |
|---------|------|--------------------|-------|
| Phase 1 | P0의 고정 source와 현재 inventory를 입력으로 역사적 메시지 154개 및 enum 23개를 보존하고, Session 메서드 232개, event row 74개와 전체 field closure를 확정합니다. API root 59개, API closure 204개, event closure 228개와 합집합 282개의 처리 결론이 모두 있어야 합니다. | 미정 | 미정 |
| Phase 2 | Phase 1 목록과 이 문서의 필드 규칙을 입력으로 core-v2 및 discord-v2 schema, capability registry, event type URL과 공통 오류를 고정합니다. 산출물에는 모든 request/response, 67개 event projection, presence wrapper, credential/file 경계 및 실제 numeric 값이 포함됩니다. unknown variant raw fallback과 런타임 객체 field가 없어야 합니다. | 미정 | 미정 |
| Phase 3 | 고정된 schema를 입력으로 생성 코드를 만들고 context-aware hook/helper 및 안전 Go view를 작성합니다. P2가 전체 변환을, P4가 principal과 lifecycle을 같은 계약에 맞춰 구현합니다. 모든 public wrapper가 context와 오류를 전달하고 credential view의 Token이 비어 있으며 patch/JSON 존재성 손실이 없어야 합니다. | 미정 | 미정 |
| Phase 4 | P2/P4의 경계 구현을 입력으로 P3가 SDK-derived typed 작업 153개, 편의 매핑 43개, 새 ownership helper 3개, 이벤트 admission과 core upload를 연결합니다. P5는 동일한 grants와 소유권 위에 optional voice를 연결합니다. 작업마다 실제 handler가 있고 공식 stage event 이름이 등록되며 token/Reader/live session이 경계를 통과하지 않아야 합니다. | 미정 | 미정 |
| Phase 5 | 통합 구현을 입력으로 모든 모듈, 예제와 검증을 v2로 함께 이동하고 v1 경로를 제거합니다. P6에서 아래 수용 조건을 실행하며 구형 모듈을 확실히 거절해야 합니다. 일반 봇 검증이 media 도구 없이 실행 가능하고 부분 stub 빌드를 배포 가능 상태로 설명하지 않아야 합니다. | 미정 | 미정 |

#### 통합 수용 조건

- 기존 선언형 도메인 목록, API source 목록 및 event source 목록을 각각 닫습니다. 모든 항목에 source 근거와 최종 처리가 있고 API root에서 모든 nested type과 variant까지 도달해야 합니다.
- v1 handshake, v1 package import, void `OnStage`, metadata identity, 빈 helper 구현과 누락 `Unimplemented` handler가 최종 실행 경로에 없어야 합니다.
- 필수 backend/capability/scope/intent 거절, 선택 기능 거절, 실제 grants 확인, 동일 artifact 이름의 다른 ModuleID 사칭 방지 및 재시작 뒤 ref 거절을 확인합니다.
- `nil`, empty string, zero, false, 빈 list/map, 명시적 null과 no-op patch를 구분합니다. member move/timeout의 null, attachment 목록의 absent/clear/완전한 수정 후 집합, 기존 ID와 새 index `0`, 모든 새 파일의 정확한 한 번 대응, 누락/중복 index와 metadata 충돌 거부를 각 수정 및 interaction 갱신 경로에서 확인합니다.
- permission composite mask, unknown representable enum, 모든 문자열 enum, 숫자 경계, option integer의 `float64` 재구성, 13개 component와 3개 interaction data variant를 확인합니다. 새 `User.PrimaryGuild`와 `UserPrimaryGuild` 전체 필드, pin pagination, modal resolved attachment 및 Required pointer의 presence도 포함합니다.
- audit JSON의 큰 정수와 null/absence가 손실 없이 전달되고, partial resolved map의 key와 nil member user, 실제로 없는 before snapshot이 보존되는지 확인합니다.
- event 생성, 변경, 삭제가 구분되고 typed ordering, byte accounting, overflow 격리, callback timeout 뒤 중단 및 drain work reference의 제한이 일치해야 합니다. restart replay는 없어야 합니다.
- 실제 source credential을 넣은 event, REST 응답, URL, nested value와 오류에서 원문 token이 새지 않아야 합니다. opaque ref를 SDK `Token`으로 넣는 경로가 없어야 합니다.
- upload의 metadata-first, offset, chunk/file/quota 제한, 취소, 결과 손실, TTL, 삭제, 동시 lease와 instance-stop cleanup을 확인합니다. 바이트 회수 직후 반복 삭제의 성공, 원래 만료 및 인스턴스 종료 뒤 실패, tombstone의 owner 검사, zero-byte 업로드를 포함한 host/instance 레코드 상한과 유효 tombstone 조기 퇴거 금지를 확인합니다. 저장 500 MiB와 Discord 허용 크기의 차이를 검증합니다.
- 모든 canonical 작업의 요청 field, response, scope, 오류 및 context 전파와 모든 alias의 인자/기본값 대응을 확인합니다. non-idempotent 쓰기의 timeout을 자동 재전송으로 숨기지 않습니다.
- 읽기 전용 `Gateway`/`GatewayBot` metadata가 원문 session/resume credential이나 연결 제어 권한을 노출하지 않아야 합니다. route batch의 원자성, 동일 소유자 반복, 타 모듈 충돌, 재시작 뒤 소유권, component/modal 수신자 결정과 운영자 binding의 typed ref 재발급을 확인합니다.

이 항목들은 구현 수용을 위한 요구사항입니다. 이 제안서 작성 과정에서 새 build, test, lint, live Discord 또는 gVisor 실행을 수행했다는 뜻이 아닙니다.

### 6-2. Dependencies

- [P0 빌드 및 의존성 기준선](flexmodule-0-build-and-dependency-baseline.md): 최소 Go 언어 버전, 실제 컴파일러, protobuf 도구와 모든 모듈의 `github.com/bwmarrin/discordgo` 요구 버전 및 준비된 SDK 대체를 일치시킵니다. 루트 `go.mod`의 upstream pin과 유지 관리하는 패치로 얻은 같은 소스를 최종 계약의 기준으로 삼습니다.
- [P2 전체 도메인 변환](flexmodule-2-domain-conversions.md): 이 문서에서 고정한 DTO 및 경계 결과의 실제 양방향 변환을 구현합니다.
- [P3 Discord 백엔드](flexmodule-3-discord-backend.md): 전체 typed 작업, 소유 command, 안전한 Gateway ingress, 자격증명 projection과 core upload/streaming multipart 구현을 담당합니다.
- [P4 런타임과 격리](flexmodule-4-runtime-and-isolation.md): 인증된 broker principal, backend registry, 실제 grants, lifecycle, sandbox와 자원 회수를 제공합니다. P3 helper가 이 작업 전에 인증을 완료했다고 주장할 수 없습니다.
- [P5 음성 job](flexmodule-5-voice-jobs.md): 선택적 playback/receive capability와 owned resource, 우선순위 constraints, 취소를 같은 core 계약에 맞춥니다.
- [P6 예제와 검증](flexmodule-6-examples-and-validation.md): 일반 봇과 optional voice의 실제 호출 및 실패 경로를 통합하여 확인합니다.
- hashicorp/go-plugin, grpc-go, protobuf well-known type, P0의 upstream과 패치로 준비한 DiscordGo 모듈이 기술 의존성입니다. API 범위는 upstream 최신 SDK라는 이름으로 임의 확장하지 않습니다.
- 현재 공식 Discord API 및 Gateway 문서는 인증, 폐지 경로, event 이름, intent와 숫자 제약 확인의 근거입니다. source 선언만으로 현재 원격 API의 성공 가능성을 보장하지 않습니다.

## 7. References

### 기존 감사의 저장소 근거

아래 v1 및 로컬 SDK 경로는 2026-10-29 제안의 역사적 근거입니다. 현재 계약은 [proto/core-v2](../../proto/core-v2/), [shared/core-v2](../../shared/core-v2/), [shared/discord-v2](../../shared/discord-v2/)와 [유지 관리하는 SDK 패치 계약](../../third_party/discordgo/README.md)을 따릅니다.

- [기존 core manifest, status, stage schema](../../proto/core-v1/hook.proto)
- [기존 core config schema](../../proto/core-v1/helper.proto)
- [기존 core Go hook interface](../../shared/core-v1/interface.go)
- [기존 core runtime client](../../shared/core-v1/runtime/client.go)
- [현재 go-plugin handshake](../../shared/shared.go)
- [기존 Discord domain schema](../../proto/discord-v1/discordgo.proto)
- [기존 Discord helper schema](../../proto/discord-v1/helper.proto)
- [기존 Discord hook schema](../../proto/discord-v1/hook.proto)
- [현재 Discord helper interface](../../shared/discord-v1/interface.go)
- [현재 로컬 REST 메서드](../../discordgo/restapi.go), [OAuth 메서드](../../discordgo/oauth2.go), [Gateway 메서드](../../discordgo/wsapi.go)
- [현재 event 등록 이름과 handler](../../discordgo/eventhandlers.go), [event wrapper 선언](../../discordgo/events.go), [handler dispatch](../../discordgo/event.go)
- [메시지 및 patch 선언](../../discordgo/message.go), [interaction 옵션 및 accessor](../../discordgo/interactions.go), [component variant](../../discordgo/components.go), [webhook credential 및 payload](../../discordgo/webhook.go)
- [일반 domain 선언](../../discordgo/structs.go), [user 선언](../../discordgo/user.go)

### 공식 계약 및 분류 근거

- [Protocol Buffers field presence](https://protobuf.dev/programming-guides/field_presence/)
- [Protocol Buffers enum semantics](https://protobuf.dev/programming-guides/proto3/#enum)
- [gRPC status codes](https://grpc.io/docs/guides/status-codes/), [cancellation](https://grpc.io/docs/guides/cancellation/), [deadlines](https://grpc.io/docs/guides/deadlines/)
- [Discord Gateway events](https://docs.discord.com/developers/events/gateway-events): 공식 stage 이름, receive-event 목록, READY 및 presence 경계를 확인합니다.
- [Discord Gateway intents](https://docs.discord.com/developers/events/gateway#gateway-intents)
- [Discord application command option types와 autocomplete](https://docs.discord.com/developers/interactions/application-commands): INTEGER 및 NUMBER 범위와 partial autocomplete를 확인합니다.
- [Discord interaction 구조](https://docs.discord.com/developers/interactions/receiving-and-responding#interaction-object-interaction-structure)
- [Discord file upload](https://docs.discord.com/developers/reference#uploading-files), [message create](https://docs.discord.com/developers/resources/message#create-message), [message edit](https://docs.discord.com/developers/resources/message#edit-message)
- [Get Current Bot Application Information](https://docs.discord.com/developers/topics/oauth2#get-current-bot-application-information)
- [Add Guild Member](https://docs.discord.com/developers/resources/guild#add-guild-member)
- [Application Role Connection Metadata](https://docs.discord.com/developers/resources/application-role-connection-metadata)
- [Command permission edit](https://docs.discord.com/developers/interactions/application-commands#edit-application-command-permissions), [비활성화된 batch edit](https://docs.discord.com/developers/interactions/application-commands#batch-edit-application-command-permissions)
- [앱 guild 생성 폐지](https://docs.discord.com/developers/change-log#deprecating-guild-creation-by-apps), [guild-create endpoint 제거](https://docs.discord.com/developers/change-log#guild-create-deprecation)
- [Discord rate limits](https://docs.discord.com/developers/topics/rate-limits)
- [go-plugin v1.8.0 client 및 application handshake](https://github.com/hashicorp/go-plugin/blob/v1.8.0/client.go)
- [원래 모듈 구조 논의](https://github.com/thirdscam/chatanium/issues/10)
