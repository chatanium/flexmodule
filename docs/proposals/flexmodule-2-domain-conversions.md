# Discord 도메인 변환 완전성 및 경계 정비 - Spec Proposal

| Item       | Detail                           |
|------------|----------------------------------|
| Author     | Seonguk Moon                     |
| Created    | 2026-10-29                       |
| Status     | **Draft** / In Review / Approved |
| Reviewers  |                                  |

---

## 1. Summary

기존 감사의 Discord 도메인 메시지 154개와 enum 23개를 보존하고, 최신 고정 SDK의 API 및 이벤트에서 도달하는 전체 타입까지 분류합니다. 전송 가능한 데이터는 양방향 변환에서 필드와 값의 의미를 보존하며, 자격 증명, 파일 Reader 및 실행 중인 SDK 객체는 typed reference, 업로드 파일 참조 또는 명시적으로 제한한 데이터 경계로 교체합니다. 역사적 선언 수로 최종 전송 범위를 제한하지 않습니다.

## 2. Background & Motivation

### 기존 감사 기준선

아래 표는 업그레이드 전 v1 `discordgo.proto`의 메시지 154개와 enum 23개에 대한 감사 기록입니다. `ChannelEdit`은 당시 이 집계 밖의 `helper.proto`에 있었으므로 별도로 포함합니다. 함수 유무와 결함 관찰은 역사적 근거이며 현재 전송 범위나 구현 완료율이 아닙니다.

| 기존 감사 분류 | 메시지 수 | 이 제안에서의 처리 |
|---|---:|---|
| 활성 named 변환이 양방향에 존재 | 72 | 모든 필드와 의미를 검토합니다. 실행 객체는 명시적 경계 교체 대상으로 분리합니다. |
| 활성 변환이 한 방향에만 존재 | 2 | `File` 및 `WebhookParams`의 반대 방향과 경계 계약을 완성합니다. |
| `Message` 안에서 양방향 inline 변환 | 2 | `MessageActivity`, `MessageApplication`의 전체 필드와 enum을 보존합니다. |
| 비활성 파일에만 양방향 변환 존재 | 39 | 중복을 통합하고 전체 동작을 수정한 뒤 정식 변환 경로에 포함합니다. |
| 변환 없음 | 28 | 전체 양방향 변환을 추가합니다. |
| 스칼라 wrapper로 별도 변환 함수 없음 | 11 | underlying 값, 비트마스크 및 presence 계약을 명시합니다. |
| 합계 | 154 | 미분류 메시지를 남기지 않습니다. |

활성 변환 함수는 방향별 77개이고 비활성 함수는 방향별 43개입니다. 이는 최상위 메시지의 수나 정확한 변환의 수와 같지 않습니다. 예를 들어 활성 변환기가 있는 `Message`에서도 11개 필드가 빠지고, `File` 역변환은 `Reader`를 nil로 돌려주며, 비활성 `AuditLogChange`는 `OldValue`와 `NewValue`를 실제로 변환하지 않습니다.

현재 루트 빌드의 네 컴파일 오류는 [P0](flexmodule-0-build-and-dependency-baseline.md)에서 수정합니다. 비활성 파일 여덟 개는 중복 심벌을 제외한 격리 빌드에서 이미 컴파일되었습니다. 따라서 비활성 코드를 활성화하면 반드시 새로운 타입 오류가 발생한다고 가정하지 않습니다. 반대로 컴파일된다는 사실을 필드 보존의 근거로 삼지도 않습니다.

기존 24개 격리 probe에서는 enum 문자열의 제어문자 변환, 기본 embed 타입 panic, nil 중첩 객체 panic, patch 초기화 손실, integer option accessor panic, resolved map key 변경 및 여러 필드 누락이 관찰되었습니다. 해당 probe는 컴파일을 막는 일부 response 함수를 제외한 격리 환경에서 실행되었으므로 전체 저장소의 성공 또는 실패를 대신하지 않습니다. 이 제안서를 작성하면서 빌드나 probe를 다시 실행하지 않았습니다.

### 현재 source delta와 처리 범위

현재 입력은 P0에 고정한 공개 `github.com/chatanium/discordgo v0.30.0`입니다. upstream base는 `f43dd94faaacd5b163e9e783f14b5bd8be639fc9`이며 승인된 Gateway 및 native DAVE 변경을 포함합니다. [source summary](../../tools/protocol/inventory/source-refresh-summary.json), [API catalog](../../tools/protocol/inventory/source-api.json), [event catalog](../../tools/protocol/inventory/source-events.json) 및 [type closure](../../tools/protocol/inventory/source-type-closure.json)에 따라 API root 59개, API named-type closure 204개, event closure 228개, 두 closure의 합집합 282개를 처리합니다. source 메서드 232개에서 SDK-derived Helper 작업 153개를 노출하고 event row 74개 중 67개를 모듈에 전달합니다. 과거의 229개 메서드, 73개 이벤트, root 57개/type 199개와 메시지 154개/enum 23개는 최종 ceiling이 아닙니다.

새 필드와 타입의 변환 계약은 다음과 같습니다.

- `Label`(Discord type 18)과 `FileUpload`(19)를 포함한 component 13개를 재귀적으로 변환합니다. Label의 `Component`, FileUpload의 ID/custom ID, `MinValues *int`, `MaxValues`, `Required *bool`, `Values`를 보존하며 modal FileUpload의 Label 배치 규칙을 검증합니다.
- resolved 타입을 `MessageComponentInteractionDataResolved`에서 `ComponentInteractionDataResolved`로 단절적으로 이관합니다. component 및 modal의 `Resolved.Attachments` map key, partial member와 첨부 DTO 전체를 유지합니다. `SelectMenu.Values`, `TextInput.Required *bool`, `SelectMenu.Required *bool`의 nil/false/true와 `TextInput.Label`의 omitempty 의미를 보존합니다.
- `User.PrimaryGuild`의 `UserPrimaryGuild{IdentityGuildID, IdentityEnabled, Tag, Badge}`와 `WebhookEdit.Flags`를 포함합니다. 기본값으로 덮어쓰거나 flags patch 존재성을 잃지 않습니다.
- `ChannelMessagesPinned`는 `before *time.Time` 및 `limit`을 받으며 응답은 `ChannelMessagePinsList{Items, HasMore}`입니다. 각 `MessagePin`의 `PinnedAt`과 `Message`, 목록 순서와 pagination을 보존하고 이전 `[]*Message` 결과로 축소하지 않습니다.
- 새 `MESSAGE_REACTION_REMOVE_EMOJI`는 사용자별 제거와 구분합니다. `ChannelDelete.BeforeDelete`/`ThreadDelete.BeforeDelete`의 `*Channel`, `GuildMemberRemove.BeforeDelete`의 `*Member`, `GuildRoleUpdate.BeforeUpdate`/`GuildRoleDelete.BeforeDelete`의 `*Role`을 실제 존재할 때만 전달합니다. READY는 `SessionID`, `Shard`, `ResumeGatewayURL`을 제거하며 `UserVoiceState` REST 결과에서도 `VoiceState.SessionID`를 제거합니다.

### 확정된 요구사항

관리, 모더레이션, 명령, 일반 메시지 및 선택적 음성 모듈은 같은 수준으로 지원합니다. 현재 예제가 사용하는 필드만 고치거나 음악 기능에 필요한 타입으로 범위를 줄이지 않습니다. 런타임과 모든 모듈을 함께 재빌드하는 v2 전환을 전제로 하며, 잘못된 v1 표현을 유지하기 위한 shim이나 alias는 남기지 않습니다.

## 3. Goals & Non-Goals

### 3.1 Goals

- [ ] 기존 154개 메시지, 23개 enum, 별도 `ChannelEdit` 및 P1/P3의 신규 API와 이벤트 DTO를 빠짐없이 분류합니다.
- [ ] 모든 안전한 도메인 필드의 양방향 변환, 중첩 변환, map key, 목록 순서 및 표현 가능한 미지의 값을 보존합니다.
- [ ] wire DTO와 presence-aware view에서 미설정, 0, false, 빈 문자열, 빈 목록 및 API가 지원하는 명시적 null을 구분합니다.
- [ ] 자격 증명을 P1의 typed `CredentialRef`와 sanitized SDK wrapper로 전달하고 일반 Token 문자열로 위장하지 않습니다.
- [ ] 파일 내용을 nil Reader로 바꾸는 성공 경로를 제거하고 P3의 업로드 참조 및 읽기 lease 경계와 연결합니다.
- [ ] 변환 실패를 구조화한 오류로 반환하고 panic, 로그만 출력한 값 누락 및 부분 성공을 제거합니다.
- [ ] 모듈에서 실제 사용하는 SDK accessor와 요청 직렬화까지 소비자 관점의 회귀 기준으로 검증합니다.

### 3.2 Non-Goals

- [ ] `Session`, `State`, `Identify`, HTTP client, dialer, mutex, ratelimiter 또는 `VoiceConnection`을 원격에서 재구성 가능한 실행 객체로 제공하지 않습니다.
- [ ] runtime-owned bot token, interaction token, webhook token, voice token 또는 세션 연결 정보를 모듈에 공개하지 않습니다.
- [ ] 변환기에서 자동 업로드, 네트워크 요청, 권한 승인, 캐시 조회, retry 또는 숨겨진 기본 응답을 실행하지 않습니다.
- [ ] 문자열 URL이나 raw JSON을 검사 없이 통과시키는 범용 프록시를 추가하지 않습니다.
- [ ] SDK가 이미 소실한 JSON presence나 float 정밀도를 추측으로 복구하지 않습니다.
- [ ] 공식 bot API 범위 밖의 endpoint를 도메인 타입 변환이 존재한다는 이유만으로 노출하지 않습니다. endpoint 분류와 구현은 [P3](flexmodule-3-discord-backend.md)의 계약을 따릅니다.

## 4. Technical Design

### 4.1 Architecture Overview

v2의 변환 경계는 세 부분으로 나눕니다.

1. **순수 데이터 변환:** 안전한 Go 데이터 또는 명시적인 SDK wrapper와 `discord-v2` DTO 사이를 변환합니다. 네트워크, 파일 열기, 인증 주체 판별 및 credential 발급을 하지 않습니다.
2. **런타임 경계 projection:** 신뢰된 Discord 응답 및 이벤트에서 presence를 확보하고, 런타임이 보유한 자격 증명을 P1의 typed reference로 바꾼 뒤 안전한 DTO만 모듈에 전달합니다. 원본 SDK 값에서 필요한 존재 여부가 사라지는 경우에는 trusted ingress에서 원래 필드의 존재를 먼저 기록합니다.
3. **실행 adapter:** P3/P4가 인증된 호출 인스턴스, 실제 grant, credential 용도 및 업로드 참조 소유권을 확인합니다. 승인된 요청만 로컬 SDK 또는 presence-aware HTTP 인코더에 전달합니다. 변환기가 wire 참조를 복사했다고 사용 권한까지 승인된 것은 아닙니다.

`core-v2`는 backend-neutral identity, lifecycle 및 reference 계약을 소유하며 Discord DTO를 import하지 않습니다. Discord 변환기는 Discord backend 패키지 안에 둡니다. 전환 완료 시 기존 `discord-v1` 변환기와 임시 비활성 파일, 이전 이름의 alias 및 중복 함수를 제거하고 모든 hook, helper, 예제 및 테스트 호출부를 함께 옮깁니다.

P1의 이벤트 envelope에는 backend ID, major version, event type, ingress sequence, received timestamp 및 typed `Any`가 들어갑니다. P2는 등록된 각 payload의 안전한 변환을 완성하며 raw gateway 바이트를 우회 경로로 제공하지 않습니다. 타입 식별자와 payload의 일치, 현재 SDK의 이벤트 범위 및 canonical 이벤트 이름은 P1/P3의 allowlist를 그대로 사용합니다.

### 4.2 Data Model Changes

DB 스키마 변경은 없습니다. wire DTO와 Go 편의 타입은 다음과 같이 바뀝니다. 이 표는 새로운 v2 계약이며 현재 파일에 이미 구현되어 있다는 뜻이 아닙니다.

| 대상 | v2 데이터 계약 |
|---|---|
| patch 스칼라 | 의미가 있는 필드에는 `optional` 또는 명시적 값 wrapper를 사용합니다. 존재 여부를 버리고 getter 기본값만 읽지 않습니다. |
| patch 목록 및 map | 부모 wrapper의 부재와 값이 존재하는 빈 collection을 구분합니다. repeated 필드만으로 clear와 omitted를 표현하지 않습니다. |
| 명시적 null | endpoint가 null을 지원하는 해당 필드에만 absent/value/null 표현을 둡니다. 모든 필드의 빈 값 또는 0을 null로 해석하지 않습니다. |
| 권한 | `Guild.Permissions`, `Role.Permissions`, `Member.Permissions`, overwrite의 allow/deny, `AppPermissions`, `DefaultMemberPermissions`, install parameters 등 모든 권한 필드를 비음수 10진 문자열로 전달합니다. 선택적 권한은 presence도 보존합니다. |
| 비트마스크 | `MessageFlags`, `UserFlags`, `RoleFlags`, `MemberFlags`, `SKUFlags` 등은 ordinal enum이 아닌 충분한 폭의 숫자 mask로 전달합니다. SDK로 내릴 때 실제 목적 타입의 범위를 검사합니다. |
| 문자열 기반 값 | embed type, allowed mention type 및 select default-value type은 원래 문자열이나 명시적인 대응표를 사용합니다. 숫자를 rune으로 바꾸지 않습니다. |
| 동적 JSON | `AuditLogChange.OldValue` 및 `NewValue` 등은 presence wrapper 안의 정확한 JSON bytes로 보존합니다. 값 없음과 JSON `null`은 다릅니다. |
| credential | P1의 `core-v2.CredentialRef`를 사용합니다. `opaque_id`, `backend_id`, `purpose`, `resource_scope`, `expires_at`은 typed 필드이며 실제 Discord token이 아닙니다. |
| 업로드 파일 | P1의 wire `UploadedFileRef{opaque_id,file_name,content_type,size_bytes,expires_at}`와 `FileAttachment{uploaded_file_ref,filename,description?}`를 사용합니다. 공용 Go view는 `FileAttachment{Ref,Filename,Description}`이며 `Ref`를 wire `uploaded_file_ref`로 매핑합니다. `io.Reader`나 호스트 파일 경로는 wire에 넣지 않습니다. |
| 부분 이벤트 | trusted ingress에서 수집한 필드 presence를 typed 이벤트 payload에 반영합니다. Go 편의 객체의 0 값만 보고 absent와 explicit zero를 복원하지 않습니다. |

#### 기존 스키마에 없는 필드

다음 항목은 변환 함수 수정만으로 해결되지 않습니다. P1의 schema 변경과 P2 변환을 함께 반영합니다.

| 현재 누락 | 처리 |
|---|---|
| `MessageAttachment.DurationSecs`, `Waveform`, `Flags` | 도메인 snapshot 필드로 추가하고 양방향 보존합니다. numeric 타입과 mask를 실제 SDK 선언에 맞춥니다. |
| `Application.PrivacyProxyURL` | 안전한 URL 데이터 필드로 추가합니다. credential이 포함된 URL을 그대로 통과시키는 예외는 아닙니다. |
| `ChannelEdit.AvailableTags`, `DefaultReactionEmoji`, `DefaultSortOrder`, `DefaultForumLayout` | 추가하고 endpoint별 patch presence 및 null 지원 여부를 그대로 표현합니다. |
| `File.Reader`의 내용 | 스칼라나 unary bytes 필드로 추가하지 않습니다. 업로드 참조로 명시적으로 교체합니다. |
| `State.TrackStickers` | live `State` 전체가 모듈 경계에서 제거되는 항목으로 기록합니다. 지원하지 않는 필드를 조용히 누락시킨 변환기로 남기지 않습니다. |

#### 실행 객체와 자격 증명의 명시적 교체

| 현재 타입 또는 필드 | 전환 후 경계 및 대체 경로 |
|---|---|
| `Session` | v2 전송 타입과 `ProtoToSession` 재구성을 제거합니다. Client, Dialer, Ratelimiter, VoiceConnections, 연결 준비 플래그, heartbeat 시각, 재연결/retry 설정, shard 실행 설정 및 SDK 로깅 설정은 호스트 내부 상태입니다. 모듈은 승인된 API와 실제 전달된 도메인 이벤트를 사용하며, 새 범용 `SessionSnapshot` API를 추가하지 않습니다. |
| `Identify` | gateway 인증 및 identify 실행 설정은 호스트 소유이므로 전송 타입을 제거합니다. bot token을 opaque reference로 일반 공개하는 대체 경로도 만들지 않습니다. |
| `State` | live cache, `Ready`, 내부 map 및 tracking 설정을 포함한 실행 객체를 제거합니다. 필요한 domain snapshot은 실제 API 결과와 P1/P3의 typed 이벤트에서 제공합니다. 역변환으로 잠금이나 동작 가능한 SDK 캐시를 만들지 않습니다. |
| `IdentifyProperties`, `GatewayStatusUpdate` | 자체가 안전한 순수 데이터인 범위에서 독립 변환을 유지합니다. `Identify` 실행 권한을 함께 부여하지 않습니다. |
| `Channel.Messages`, `Channel.Members` | 실제 입력에 존재하는 안전한 snapshot을 변환합니다. 누락된 캐시를 조회해 채우거나 빈 결과를 완전한 캐시로 표시하지 않습니다. |
| `User.Token`, `Session.Token`, `Identify.Token` | 모듈 DTO에서 제거합니다. 순수 변환기에 raw token이 들어오면 경계 오류로 거절하며 trusted runtime projection이 해당 정책을 먼저 적용합니다. |
| `Interaction.Token`, `Webhook.Token` | runtime projection에서 용도와 자원 범위가 제한된 `CredentialRef`로 교체합니다. 모듈에는 Token이 비어 있는 SDK 객체와 별도 typed reference를 가진 handle을 제공합니다. |
| `VoiceState.SessionID`, READY의 `SessionID`/`Shard`/`ResumeGatewayURL`, voice server credential | 런타임 연결 내부 정보로 제거합니다. 사용자, 채널, mute/deaf 등 허용된 voice 도메인 필드는 REST `UserVoiceState`와 이벤트에 같은 정책으로 보존합니다. raw 이벤트나 URL을 통한 재노출도 금지합니다. |
| `GuildMemberAddParams`의 사용자 승인 credential | bot endpoint에 필요한 operator-provisioned 사용자 credential을 P1의 목적 제한 reference로 전달합니다. OAuth 로그인 흐름이나 secret export 기능을 새로 만들지 않습니다. |
| `File` | 이름과 content type만 남기고 Reader를 nil로 만드는 변환은 삭제합니다. 모듈은 명시적으로 업로드한 `FileAttachment`를 전달하고 호스트가 P3의 read lease로 실제 파일을 엽니다. |

이 교체는 “모든 실행 객체의 무손실 직렬화”를 약속하지 않는 의도적인 경계 변경입니다. 각 제거 필드의 이유와 사용할 도메인 경로를 기록해야 154개 분류의 완료로 인정합니다. `ReadState`, `SessionInformation`처럼 이름이 비슷하더라도 실제로 순수 데이터인 타입까지 이름만으로 실행 객체에 포함시키지 않습니다.

### 4.3 Core Logic

#### 기본 변환 규칙

- nil 최상위 포인터는 `(nil, nil)`로 반환합니다. 선택적인 nested 포인터도 존재 여부를 보존하며, `Activity.Party`, `Assets`, `Secrets`, timestamp, `Presence` 및 partial `Channel`을 무조건 역참조하지 않습니다.
- 요청용 SDK value-only 필드는 필드별로 검토한 `omitempty` 계약을 따릅니다. `MessageSend`, `WebhookParams`와 component 계열의 선택적 문자열 17개는 빈 source 값을 생략합니다. 숫자는 `Poll.Duration`, `Poll.LayoutType`, `SelectMenu.MaxValues`, `FileUpload.MaxValues`, `TextInput.MaxLength` 5개에만 source 0 생략을 적용합니다. 대상과 근거는 [필드별 매핑](../../tools/protocol/domain-mapping.json)의 `source_json_omit_empty` 및 `source_json_omit_zero`와 [SDK 설명](../../shared/README.md)에 기록합니다.
- 위 생략 규칙은 SDK 입력을 wire로 옮길 때만 적용합니다. wire에 명시한 빈 문자열과 0을 HTTP 인코더에서 제거하지 않으며, 다른 0/false 값, 포인터, nullable 및 collection 존재성도 바꾸지 않습니다. 전체 SDK JSON과의 일괄 동등성이나 수신 snapshot의 존재성을 SDK 기본값에서 복원한다고 약속하지 않습니다.
- protobuf가 표현할 수 없는 nil collection 원소는 인덱스가 포함된 명시적 오류로 거절합니다. 원소를 건너뛰어 목록 길이나 순서를 바꾸지 않습니다. API가 null 원소를 허용하는 경우에는 별도 nullable wire 표현을 사용합니다.
- 일반 snapshot에서 nil과 빈 collection이 같은 의미라면 같은 빈 결과로 정규화할 수 있습니다. patch에서 같은 정규화를 적용하지 않습니다. 이 의미 구분을 필드별 분류에 기록합니다.
- map은 입력 key를 그대로 복사합니다. 값 안의 `ID` 또는 `Member.User.ID`로 key를 다시 만들지 않습니다. resolved member에 `User`가 없는 정상 partial object를 허용하고 사용자를 만들어 넣지 않습니다.
- 상위 변환기는 첫 하위 오류를 field path와 함께 반환하며 부분 결과는 nil로 반환합니다. 변환 중 출력하거나 전송한 부작용이 없어야 합니다.
- 입력 객체를 변경하지 않습니다. 출력은 별도 snapshot이며 호출자가 입력을 바꿔도 출력의 collection 값이 변하지 않습니다. 필요한 출력 크기에 맞게 할당하고 JSON을 거치는 범용 왕복이나 불필요한 중간 복사를 사용하지 않습니다.
- 재귀 데이터는 선언된 필드를 따라 변환합니다. SDK 포인터 그래프에 실제 cycle이 있으면 명시적 오류로 종료하며 무한 재귀나 조용한 잘림을 허용하지 않습니다. 동일한 객체를 두 필드가 참조한다는 이유만으로 cycle로 오인하지 않습니다.
- timestamp는 유효한 seconds/nanos 범위를 확인합니다. 없는 timestamp를 Unix epoch로 만들지 않습니다. 원래 optional이면 nil을 유지하고 SDK value 필드가 zero time으로만 표현할 수 있으면 authoritative typed DTO의 presence를 함께 유지합니다.

#### 주요 활성 필드 손실의 수정

다음은 우선 확인된 결함이며, 이 표만 고치고 나머지 필드 검토를 생략하는 목록은 아닙니다.

| 타입 또는 경로 | 반드시 보존하거나 교체할 내용 |
|---|---|
| `Message` | `Flags`, `Interaction`, `InteractionMetadata`, `Mentions`, `MessageReference`, `MessageSnapshots`, `Poll`, `Reactions`, `ReferencedMessage`, `StickerItems`, `Thread`의 11개 누락을 양방향 수정합니다. |
| `MessageSend` | `AllowedMentions`, `Components`, `Flags`, `Poll`, `Reference`를 보존합니다. 파일 내용은 업로드 참조로 전달하고 기존 단일 `File`/`Embed` convenience 필드는 SDK의 canonical 요청으로 명시적으로 정규화하거나 새 typed 호출로 마이그레이션합니다. |
| `MessageEdit` | content, components, embeds, attachments의 omitted와 explicit empty를 구분합니다. `AllowedMentions` 및 flags를 보존하며 SDK value-only flags를 거쳐 presence를 잃지 않습니다. |
| `User` | `GlobalName`, `AccentColor`, `Banner`, `MFAEnabled`, `PremiumType`, `System`, `PrimaryGuild`와 `UserPrimaryGuild`의 모든 안전한 필드를 보존합니다. Token 누락은 보완 대상이 아니라 위의 금지 경계로 처리합니다. |
| `ApplicationCommand` | `Contexts`, `IntegrationTypes`, `DefaultPermission`, `DMPermission` 및 `DefaultMemberPermissions`의 값과 존재 여부를 보존합니다. nil과 명시적 0 권한을 혼동하거나 helper에서 권한을 강제로 nil로 만들지 않습니다. |
| `ApplicationCommandOption` 및 choice | 역방향 `Autocomplete`, choice의 `NameLocalizations`, min/max 및 length의 presence를 보존합니다. 선택 값의 실제 종류와 parent option 타입을 맞춥니다. |
| interaction option | string, integer, boolean, user, channel, role, mentionable, number, attachment 및 nested subcommand/group을 모두 처리합니다. role/mentionable/attachment ID를 버리지 않습니다. |
| `Interaction` | `Context`, `AuthorizingIntegrationOwners`, `Entitlements`를 hardcoded zero/nil 대신 실제 값으로 변환합니다. interaction 종류별 typed data와 credential handle을 함께 보존합니다. |
| resolved 객체 | Users, Members, Roles, Channels, Messages, Attachments 및 component resolved map의 원래 key를 유지합니다. partial member나 channel이 완전한 객체라는 가정을 하지 않습니다. |
| `InteractionResponseData` | `Files`, `Attachments`, `Choices`, `CustomID`, `Title`, `Poll` 및 기존 content/components/embeds/mentions/flags를 모두 변환합니다. autocomplete, modal 및 메시지 응답을 같은 일부 필드로 축소하지 않습니다. |
| `WebhookParams` | 누락된 역방향을 구현하고 `Components`, `Files`, `Flags` 및 모든 기존 필드를 보존합니다. 파일은 참조 경계를 사용합니다. |
| `WebhookEdit` | 기존 감사에서 content만 변환하던 경로를 교체하여 `AllowedMentions`, `Attachments`, `Components`, `Embeds`, `Files`, 새 `Flags`와 patch presence를 보존합니다. |
| `ChannelEdit` | `PermissionOverwrites`, `Flags`, `DefaultThreadRateLimitPerUser`, `Archived`, `AutoArchiveDuration`, `Locked`, `Invitable`, `AppliedTags`를 보완하고 4.2의 스키마 누락도 함께 해결합니다. |
| `Presence.Activities` | 별도 축약 inline 변환을 없애거나 완전한 `Activity` 변환과 같은 결과가 되게 하여 `Party`, `Assets`, `Secrets`, `Instance`, `Flags`를 보존합니다. |
| `GatewayBotResponse` | 공개 base URL, shard 권고 및 `SessionStartLimit`을 읽기 전용 데이터로 보존합니다. limit 필드의 실제 SDK 타입은 `SessionInformation`이며 `Total`, `Remaining`, `ResetAfter`의 밀리초 단위, `MaxConcurrency`를 모두 변환합니다. gateway 연결 또는 세션 재개 권한을 함께 부여하지 않습니다. |
| `VoiceRegion` | `Optimal`, `Deprecated`, `Custom`을 보존합니다. |
| embed 및 emoji | `MessageEmbedThumbnail.Width`를 보존하고 `Emoji.Available`은 `Animated`가 아닌 해당 필드를 읽습니다. 기본/빈 embed type을 panic으로 처리하지 않습니다. |

#### 비활성 39개와 중복 구현

현재 비활성 전용 타입 39개는 다음과 같습니다. 각 그룹은 사용 빈도와 관계없이 모두 양방향 변환 또는 명시된 경계 매핑의 대상입니다.

| 계열 | 타입 |
|---|---|
| 애플리케이션 및 팀, 7개 | `ApplicationIntegrationTypeConfig`, `ApplicationInstallParams`, `Application`, `ApplicationRoleConnectionMetadata`, `ApplicationRoleConnection`, `TeamMember`, `Team` |
| 사용자 연결 및 integration, 3개 | `UserConnection`, `Integration`, `IntegrationAccount` |
| 초대, thread 및 보조 데이터, 11개 | `Invite`, `ThreadStart`, `ThreadsList`, `AddedThreadMember`, `EmojiParams`, `TooManyRequests`, `ReadState`, `GuildRole`, `GuildBan`, `APIErrorMessage`, `SessionInformation` |
| 모더레이션 및 audit, 8개 | `AutoModerationRule`, `AutoModerationTriggerMetadata`, `AutoModerationActionMetadata`, `AutoModerationAction`, `GuildAuditLog`, `AuditLogEntry`, `AuditLogChange`, `AuditLogOptions` |
| poll 및 monetization, 10개 | `PollMedia`, `PollAnswer`, `PollAnswerCount`, `PollResults`, `Poll`, `SKU`, `Subscription`, `Entitlement`, `EntitlementTest`, `EntitlementFilterOptions` |

활성화 대상 파일은 양방향 `application.go.disabled`, `misc.go.disabled`, `moderation_audit.go.disabled`, `monetization.go.disabled`입니다. 현재 중복은 `GatewayBotResponse`, `VoiceRegion`, `ComponentEmoji`의 양방향 및 `File`의 역방향입니다. 각 심벌에 완전한 구현 하나만 남기고 기존 minimal 구현과 비활성 중복을 통합합니다. 단순히 확장자를 바꾸거나 중복을 피하려고 두 이름을 유지하지 않습니다.

`AuditLogChange`의 현재 no-op는 별도로 제거합니다. `Key`의 presence와 Old/New의 absent, JSON null 및 실제 값을 구분하며, 빈 bytes를 모든 값의 성공적인 변환으로 돌려주지 않습니다. `GuildAuditLog`가 이 변환을 포함하므로 하위 값 오류는 전체 audit 결과 변환의 오류로 전달합니다.

#### 변환이 없는 28개

| 계열 | 타입 |
|---|---|
| 메시지 관계 및 reaction, 5개 | `MessageSnapshot`, `MessageReference`, `MessageInteraction`, `MessageInteractionMetadata`, `MessageReaction` |
| sticker, 3개 | `Sticker`, `StickerItem`, `StickerPack` |
| guild 및 멤버 요청, 7개 | `GuildPreview`, `UserGuild`, `GuildParams`, `RoleParams`, `GuildEmbed`, `GuildMemberParams`, `GuildMemberAddParams` |
| 예약 이벤트, 4개 | `GuildScheduledEvent`, `GuildScheduledEventParams`, `GuildScheduledEventEntityMetadata`, `GuildScheduledEventUser` |
| onboarding, 3개 | `GuildOnboarding`, `GuildOnboardingPrompt`, `GuildOnboardingPromptOption` |
| guild template, 2개 | `GuildTemplate`, `GuildTemplateParams` |
| stage, 1개 | `StageInstanceParams` |
| 명령 권한, 3개 | `ApplicationCommandPermissions`, `ApplicationCommandPermissionsList`, `GuildApplicationCommandPermissions` |

중첩 객체부터 외부 요청까지 같은 규칙을 적용합니다. 예를 들어 `GuildMemberAddParams`를 구현하면서 사용자 승인 credential을 원래 문자열로 전송하지 않습니다. 삭제된 공식 endpoint나 OAuth 전용 endpoint의 요청 타입이 목록에 있더라도, 도메인 변환의 완성과 endpoint 공개 여부는 별도로 분류합니다.

#### enum 23개와 스칼라 wrapper 11개

| 분류 | 기존 enum 이름 | 변환 규칙 |
|---|---|---|
| 비트마스크, 2개 | `MessageFlags`, `UserFlags` | 원래 bit 위치와 조합을 보존하는 숫자 저장소로 교체합니다. 예를 들어 SDK ephemeral 값 64를 기존 proto ordinal 6과 같은 값으로 간주하지 않습니다. |
| 문자열 기반, 3개 | `AllowedMentionType`, `EmbedType`, `SelectMenuDefaultValueType` | 실제 문자열을 보존하거나 명시적 대응표를 사용합니다. 빈 값이 SDK에서 유효한 기본 표현이면 그대로 보존합니다. 허용되지 않는 값은 오류이며 rune cast로 생성하지 않습니다. |
| 수치 discriminator, 18개 | `MessageType`, `MessageActivityType`, `MessageReferenceType`, `UserPremiumType`, `MembershipState`, `WebhookType`, `ChannelType`, `ComponentType`, `ButtonStyle`, `TextInputStyle`, `SeparatorSpacingSize`, `ApplicationCommandType`, `InteractionContextType`, `ApplicationIntegrationType`, `ApplicationCommandOptionType`, `ApplicationCommandPermissionType`, `InteractionType`, `InteractionResponseType` | 실제 SDK 및 Discord 숫자 의미와 맞춥니다. 기존 enum의 선언 순서가 아니라 상수별 값으로 대응하며, 목적 타입 범위 안의 미지 값을 허용하는 API에서는 그대로 보존합니다. |

`MessageActivityType`, `ChannelType`, `InteractionContextType`, `ApplicationIntegrationType`은 기존 숫자 계약 차이를 특히 확인합니다. 실제 의미가 0인 타입에 임의 offset을 주지 않습니다. composite mask는 알려진 상수의 단일 enum 값으로 축소하지 않습니다. wire에는 표현되지만 로컬 SDK의 `uint8`, `int`, `int64` 등에 들어가지 않는 값은 명시적 range 오류로 반환합니다.

스칼라 wrapper 11개는 `GuildOnboardingMode`, `GuildOnboardingPromptType`, `RoleFlags`, `MemberFlags`, `AutoModerationActionType`, `PollLayoutType`, `SKUType`, `SKUFlags`, `SubscriptionStatus`, `EntitlementType`, `EntitlementOwnerType`입니다. mask인 세 flags wrapper는 bit 조합을, 나머지는 wire에 존재하는 실제 underlying numeric value를 보존합니다. wrapper 부재와 존재하는 0이 구분되는 필드는 그 차이도 보존합니다. SDK source의 검토된 생략 규칙은 위의 기본 변환 규칙을 따릅니다. 별도 함수가 없다는 이유만으로 변환 완료로 계산하지 않습니다.

#### interaction 값과 동적 JSON

정수 option은 wire의 `sint64`, number는 `double`, 문자열과 ID는 string, boolean은 bool로 구분합니다. subcommand와 group은 중첩 options를 가지며 값이 없는 것이 정상입니다. value discriminator가 option 종류와 맞지 않거나 알 수 없는 payload를 처리할 수 없으면 오류로 반환합니다.

공식 command 계약에 따라 INTEGER는 `[-2^53+1, 2^53-1]`, NUMBER는 `[-2^53, 2^53]` 범위로 제한합니다. 정수에 소수값, non-finite 수 또는 범위 밖 값을 허용하지 않습니다. 로컬 SDK의 `IntValue()` 및 `UintValue()`가 `Value.(float64)`를 사용하므로, 역변환에서 integer를 `int64`로 넣지 않고 범위를 확인한 `float64`로 재구성합니다. `UintValue()`에는 음수를 적합한 unsigned 입력인 것처럼 보장하지 않습니다. number는 `float64`, user/channel/role/mentionable/attachment는 ID string으로 재구성하며 option의 원래 타입을 바꾸지 않습니다.

choice는 typed string/integer/number 값을 보존하고 parent option 타입과 일치해야 합니다. localizations와 선택 값의 범위를 함께 검사합니다. 모든 동적 값을 `google.protobuf.Value`의 double로 일괄 변환하지 않습니다.

Audit JSON은 trusted Discord 수신 경계에서 원본 필드 존재 여부와 정확한 JSON bytes를 확보합니다. JSON 전체가 한 값으로 유효한지 검사하며, 모듈의 편의 decoding이 필요한 경우에는 `UseNumber`에 준하는 방식으로 정수를 보존합니다. 이미 일반 `interface{}`의 float64로 읽어 정밀도나 absent/null 구분을 잃은 SDK 객체만 전달되면 완전한 역복원을 약속하지 않고 presence-aware view가 필요한 오류를 반환합니다. JSON이 런타임 credential을 포함하면 raw 통과를 허용하지 않으며, P1에 명시적인 typed replacement가 없는 경우 해당 projection을 거절합니다. 안전한 도메인 JSON의 정확한 보존과 secret export는 다른 요구사항입니다.

wire JSON wrapper의 필드 이름은 P1의 `JSONValue.json`이며, UTF-8로 인코딩한 JSON 값 하나를 담습니다. 아래 Go 편의 타입의 `Raw`는 이 bytes 필드와 직접 대응합니다. gateway와 audit의 raw 입력은 typed 필드 allowlist를 통과한 값만 투영하며, credential이 포함될 수 있는 미지의 raw map은 그대로 내보내지 않습니다.

`Activity.Secrets`의 join/match/spectate 값은 애플리케이션의 도메인 값이며 runtime bot token과 동일시하지 않습니다. 선언된 안전한 도메인 값은 필드별 정책에 따라 변환하지만, 그 경로에 runtime-owned credential이 들어오면 approved typed replacement를 적용하거나 projection을 거절합니다. 이름만 보고 모든 값을 제거하거나 모든 문자열을 secret 검토 없이 내보내지 않습니다.

#### component, patch 및 이벤트 closure

현재 component oneof의 13가지 variant는 `ActionsRow`, `Button`, `SelectMenu`, `TextInput`, `Section`, `TextDisplay`, `Thumbnail`, `MediaGallery`, `FileComponent`, `Separator`, `Container`, `Label`, `FileUpload`입니다. 각각의 필드와 중첩 component를 보존합니다. typed nil interface, 빈 oneof, 알려지지 않은 variant 및 허용되지 않는 type/payload 조합은 nil 성공이나 원소 삭제로 처리하지 않습니다.

`MessageEdit`, `WebhookEdit`, `ChannelEdit` 및 다른 수정 요청의 authoritative 표현은 P1/P3의 typed request DTO입니다. SDK 구조체가 `omitempty`나 value-only 필드 때문에 명시적 zero/empty/null을 잃으면 해당 구조체를 왕복 경로로 사용하지 않습니다. P3의 presence-aware encoder가 typed DTO를 직접 요청 body로 만들며, P2의 편의 patch view는 같은 존재 여부를 유지합니다. attachment 수정 요청에는 응답 snapshot인 `MessageAttachment` 대신 P1의 `AttachmentEditEntry`와 present `AttachmentEditList`를 사용합니다. present 목록은 유지할 기존 ID와 추가할 새 파일 index를 모두 담은 수정 후 전체 집합입니다. 목록 미지정은 새 파일이 없을 때만 기존 첨부 유지이고, present 빈 목록은 전체 첨부 제거입니다.

새 파일이 있으면 목록이 반드시 존재하며 `files`의 모든 index를 정확히 한 번씩 지정해야 합니다. index `0`을 oneof 부재로 바꾸지 않으며, 중복 기존 ID, 중복/범위 밖/누락 index, 빈 oneof와 잘못된 metadata 조합을 거부합니다. 숨은 GET, 보존 목록 자동 생성 또는 새 파일의 암묵적 append는 하지 않습니다. 이 규칙은 `MessageEdit`, `WebhookEdit`, `InteractionResponseData`의 메시지 갱신용 안전 view 및 `InteractionResponseEdit`와 `FollowupMessageEdit` 편의 매핑에 공통으로 적용합니다. 각 view의 수정 첨부 필드는 `Attachments *[]AttachmentEditEntry`이고 wire의 `AttachmentEditList`와 양방향 대응합니다. 응답과 이벤트의 `MessageAttachment`는 전체 도메인 DTO로 별도 보존합니다.

P1/P3의 신규 요청 및 응답 DTO는 전부 같은 분류표에 추가합니다. tuple 인자는 이름 있는 필드, variadic 목록은 의미에 맞는 collection, error-only 결과는 Empty로 매핑하며, 각 reachable nested 타입까지 변환을 닫습니다. 새 이벤트는 `<SourceType>Event` 기반 typed payload와 실제 필드 presence를 사용합니다. stage 이벤트의 canonical 이름은 `STAGE_INSTANCE_CREATE`, `STAGE_INSTANCE_DELETE`, `STAGE_INSTANCE_UPDATE`이며 기존 로컬 event 이름의 오기를 호환 alias로 남기지 않습니다. `PRESENCES_REPLACE`처럼 전달 대상에서 제외한 legacy event가 있어도 순수 `Presence` 변환을 삭제하지 않습니다.

기존 공식 bot API 감사의 root 57개와 named type 199개(struct 131개, scalar/alias 66개, interface 2개)는 역사적 기록입니다. 현재 API root 59개에서 도달하는 named type은 204개이며 API/event closure 합집합은 282개입니다. 이는 신규 Protobuf message 개수가 아니며 기존 메시지 154개나 이벤트 전용 타입의 수를 대체하지 않습니다. `ChannelFollow`, `GuildChannelCreateData`, `UpdateStatusData`, `ChannelMessagePinsList` 및 polymorphic variant도 포함합니다. `Session` receiver, 임의 `RequestOption`, HTTP/Gateway transport 및 `VoiceConnection`은 domain root에서 제외하고 명시적 경계로 분류합니다. 전체 source signature와 root 목록은 P3의 현재 API catalog에 연결하며 타입 하나라도 처리 결론이 없으면 closure 완료로 보지 않습니다.

`BeforeUpdate` 및 `BeforeDelete`는 실제 cache snapshot이 존재할 때만 제공합니다. 삭제 이벤트를 생성 이벤트로 재해석하거나 누락된 author/member를 만들어 넣지 않습니다. 초기 154개를 모두 처리했더라도 새 API나 이벤트에서 도달하는 DTO가 미분류이면 P2의 통합 완료로 인정하지 않습니다.

## 5. API Design

### 5-1. New / Modified

REST endpoint를 새로 정의하지 않습니다. 다음은 `shared/discord-v2`에서 사용할 제안 시그니처입니다. P1의 wire 타입, credential 인터페이스 및 P3의 파일 lease API를 재정의하는 별도 규약으로 사용하지 않습니다.

#### 순수 변환 및 오류 타입

일반 데이터 타입 `X`는 `XToProto(*discordgo.X) (*discordv2.X, error)`와 `ProtoToX(*discordv2.X) (*discordgo.X, error)` 한 쌍을 제공합니다. 모든 retained domain 타입에 동일한 반환 계약을 적용합니다. interface 기반 component, parent 타입이 필요한 choice, credential handle, 파일, presence-aware patch 및 동적 JSON을 포함하는 외곽 객체는 아래의 구체적인 safe view 입력을 사용합니다. 제거한 실행 객체에는 같은 이름의 빈 변환 함수를 남기지 않습니다.

```go
// ConversionError identifies a conversion failure without exposing source
// values. Path uses field names, collection indices, and map keys only when
// the key is non-secret. A failed conversion returns no partial output.
// Code is one of InvalidValue, OutOfRange, UnsupportedVariant,
// InvalidPresence, InvalidJSON, CredentialBoundaryRequired,
// UploadedFileRefRequired, or RuntimeObjectNotTransportable.
// Direction is StructToProto or ProtoToStruct; Message names the DTO.
// Reason is a stable, sanitized explanation, not raw JSON or a token.
type ConversionError struct {
    Code      string
    Direction string
    Message   string
    Path      string
    Reason    string
}

// Error returns a sanitized diagnostic suitable for an RPC error summary.
// It must not format the original object, raw credentials, or arbitrary JSON.
func (e *ConversionError) Error() string

// MessageToProto converts all permitted message fields and nested values.
// Nil input returns (nil, nil). Raw credentials anywhere in the source graph
// require runtime projection first. The source is never modified.
func MessageToProto(src *discordgo.Message) (*discordv2.Message, error)

// ProtoToMessage reconstructs a token-free SDK message snapshot. It preserves
// source map keys and accepted partial objects, and never fabricates members
// or cache data. Event field presence remains in the typed event payload.
func ProtoToMessage(src *discordv2.Message) (*discordgo.Message, error)

// MessageComponentToProto converts exactly one supported component variant.
// Unknown variants and unrepresentable nil collection elements are errors;
// they must not be silently removed from the parent component list.
func MessageComponentToProto(src discordgo.MessageComponent) (*discordv2.MessageComponent, error)

// ProtoToMessageComponent checks the oneof and reconstructs the corresponding
// local SDK variant. It returns an error rather than a nil success for an
// unsupported non-empty oneof or a discriminator mismatch.
func ProtoToMessageComponent(src *discordv2.MessageComponent) (discordgo.MessageComponent, error)

// ApplicationCommandInteractionDataOptionToProto preserves the option kind,
// ID or typed scalar value, focus flag, and nested options. Integer values
// must satisfy the Discord integer range and must not contain fractions.
func ApplicationCommandInteractionDataOptionToProto(src *discordgo.ApplicationCommandInteractionDataOption) (*discordv2.ApplicationCommandInteractionDataOption, error)

// ProtoToApplicationCommandInteractionDataOption reconstructs integer Value
// as float64 because the local SDK IntValue accessor requires that dynamic
// type. Conversion rejects out-of-range values instead of rounding them.
func ProtoToApplicationCommandInteractionDataOption(src *discordv2.ApplicationCommandInteractionDataOption) (*discordgo.ApplicationCommandInteractionDataOption, error)

// ApplicationCommandOptionChoiceToProto validates the choice against its
// parent option type and preserves localized names and the typed value.
// Callers must provide the actual parent type rather than guess from JSON.
func ApplicationCommandOptionChoiceToProto(src *discordgo.ApplicationCommandOptionChoice, optionType discordgo.ApplicationCommandOptionType) (*discordv2.ApplicationCommandOptionChoice, error)

// ProtoToApplicationCommandOptionChoice applies the same parent-type checks
// and reconstructs a local SDK value compatible with that option's encoder.
func ProtoToApplicationCommandOptionChoice(src *discordv2.ApplicationCommandOptionChoice, optionType discordgo.ApplicationCommandOptionType) (*discordgo.ApplicationCommandOptionChoice, error)
```

#### presence-aware patch 및 동적 값

다음 `MessageEditPatch`는 기존 SDK가 잃는 flags presence를 포함하는 편의 입력입니다. P1/P3에서 endpoint에 null을 허용한다고 정한 다른 필드는 해당 typed nullable view를 사용합니다. 이 구조체에 존재하지 않는 null 의미를 nil 포인터로 추가 해석하지 않습니다.

```go
// AttachmentEditSource is a closed choice, not two nullable scalar fields.
// Converters accept exactly one of the concrete value variants below.
type AttachmentEditSource interface {
    isAttachmentEditSource()
}

// ExistingAttachmentID retains one attachment already on the target message.
// Empty or malformed snowflakes are invalid; duplicate IDs fail the parent patch.
type ExistingAttachmentID string
func (ExistingAttachmentID) isAttachmentEditSource() {}

// NewFileIndex selects one element of Files. Zero is the valid first index,
// not an omitted choice. The parent patch checks bounds and exact-once use.
type NewFileIndex uint32
func (NewFileIndex) isAttachmentEditSource() {}

// AttachmentEditEntry is the shared request view for P1's oneof source.
// Nil, typed-nil, pointer variants, and unsupported Source types are rejected.
// Existing metadata updates follow the endpoint contract. New-file metadata,
// when repeated here, must agree with the selected FileAttachment.
type AttachmentEditEntry struct {
    Source      AttachmentEditSource
    Filename    *string
    Description *string
}

// AttachmentEditEntryToProto preserves the selected source even at index zero.
// It validates the choice and metadata shape without fetching any attachment.
func AttachmentEditEntryToProto(src *AttachmentEditEntry) (*discordv2.AttachmentEditEntry, error)

// ProtoToAttachmentEditEntry returns the exact concrete value variant.
// An absent or unsupported oneof is an error, not a zero-index fallback.
func ProtoToAttachmentEditEntry(src *discordv2.AttachmentEditEntry) (*AttachmentEditEntry, error)

// MessageEditPatch retains the distinction between an omitted field and an
// explicit empty or zero value. Files contains new uploaded attachments;
// a present Attachments list is the complete post-edit set, with every new
// index exactly once. Nil Attachments is valid only when Files is empty.
// ID and ChannelID identify the target, not the calling instance.
type MessageEditPatch struct {
    ID              string
    ChannelID       string
    Content         *string
    Components      *[]discordgo.MessageComponent
    Embeds          *[]*discordgo.MessageEmbed
    AllowedMentions *discordgo.MessageAllowedMentions
    Flags           *uint64
    Files           []FileAttachment
    Attachments     *[]AttachmentEditEntry
}

// MessageEditToProto preserves every field's presence without routing through
// discordgo.MessageEdit fields that cannot express the same distinction.
// It checks the complete attachment set and exact-once file-index coverage.
// It performs no upload, request dispatch, implicit append, or hidden fetch.
func MessageEditToProto(src *MessageEditPatch) (*discordv2.MessageEdit, error)

// ProtoToMessageEdit reconstructs the presence-aware SDK view, not a lossy
// legacy MessageEdit value. Empty present collections remain present.
func ProtoToMessageEdit(src *discordv2.MessageEdit) (*MessageEditPatch, error)

// JSONValue holds exactly one valid JSON value. A nil *JSONValue means the
// parent field is absent; Raw containing "null" means an explicit JSON null.
// Construction from already rounded float64 data cannot restore lost digits.
type JSONValue struct {
    Raw json.RawMessage
}

// AuditLogChangeValue is the safe view used for audit-log changes. Its key
// and old/new values retain presence captured at the trusted JSON boundary.
// It is not reconstructed by guessing whether a legacy interface{} was nil.
type AuditLogChangeValue struct {
    Key      *discordgo.AuditLogChangeKey
    OldValue *JSONValue
    NewValue *JSONValue
}

// AuditLogChangeToProto validates the JSON values and preserves their exact
// bytes and presence. Credential-bearing values require an approved runtime
// projection and are never exported through this generic JSON container.
func AuditLogChangeToProto(src *AuditLogChangeValue) (*discordv2.AuditLogChange, error)

// ProtoToAuditLogChange preserves absent, null, and present values without a
// blanket conversion to protobuf double or an untyped float64 JSON tree.
func ProtoToAuditLogChange(src *discordv2.AuditLogChange) (*AuditLogChangeValue, error)

// AuditLogEntryView replaces the fixed SDK Changes field with presence-aware
// values. It is a distinct type, not a discordgo.AuditLogEntry with a different
// slice element type. All other current local SDK fields remain represented.
type AuditLogEntryView struct {
    TargetID   string
    Changes    []*AuditLogChangeValue
    UserID     string
    ID         string
    ActionType *discordgo.AuditLogAction
    Options    *discordgo.AuditLogOptions
    Reason     string
}

// GuildAuditLogView retains the complete current local audit-log result.
// Entries use the safe view above, and webhooks use token-free handles.
// Users and nested integration data must already satisfy credential policy.
type GuildAuditLogView struct {
    Webhooks        []*WebhookHandle
    Users           []*discordgo.User
    AuditLogEntries []*AuditLogEntryView
    Integrations    []*discordgo.Integration
}

// AuditLogEntryToProto preserves all entry fields and every change value's
// exact JSON bytes and presence. A nested error fails the entire conversion.
func AuditLogEntryToProto(src *AuditLogEntryView) (*discordv2.AuditLogEntry, error)

// ProtoToAuditLogEntry returns the explicit safe view rather than an SDK
// pointer whose fixed Changes field cannot retain AuditLogChangeValue.
func ProtoToAuditLogEntry(src *discordv2.AuditLogEntry) (*AuditLogEntryView, error)

// GuildAuditLogToProto converts the complete safe result without minting
// webhook credentials or inferring lost JSON presence from SDK values.
func GuildAuditLogToProto(src *GuildAuditLogView) (*discordv2.GuildAuditLog, error)

// ProtoToGuildAuditLog preserves safe entries and typed webhook handles
// throughout the outer result; it does not return discordgo.GuildAuditLog.
func ProtoToGuildAuditLog(src *discordv2.GuildAuditLog) (*GuildAuditLogView, error)
```

`GuildAuditLogView`와 `AuditLogEntryView`는 기존 SDK 구조체와 별개의 편의 타입입니다. SDK의 `GuildAuditLog.AuditLogEntries` 및 `AuditLogEntry.Changes` 필드 타입을 그대로 둔 채 다른 타입의 값을 넣는 방식은 사용하지 않습니다. 따라서 audit 변환 호출부는 위의 정확한 시그니처로 마이그레이션하고, 직접 typed RPC를 사용하는 모듈은 wire DTO를 그대로 소비할 수 있습니다. 같은 원칙으로 `WebhookEdit`과 SDK로 표현할 수 없는 `ChannelEdit` 필드는 typed patch view를 사용합니다. `WebhookEdit` 및 메시지 갱신용 `InteractionResponseData`의 첨부 필드도 위의 단일 `AttachmentEditEntry` view를 사용하며, 별도의 retained-only 목록이나 nullable ID/index 쌍을 정의하지 않습니다.

#### credential projection 및 모듈 handle

아래 `CredentialEncoder`, `CredentialBinding`, `CredentialTarget`, `ProjectionPolicy`, `CredentialUse` 및 `CredentialResolver`는 P1 소유 타입입니다. `CredentialTarget`은 `Purpose string`과 `ResourceScope *corev2.ResourceSelector`만 담고, `ProjectionPolicy`의 정확한 함수 타입은 `func(context.Context, CredentialTarget) (CredentialBinding, error)`입니다. P3의 신뢰된 policy가 작업에 승인된 backend, purpose, 정확한 scope 및 유한한 미래 expiry를 제공합니다. converter가 기본 TTL을 정하거나 모듈 입력의 owner ID와 ref metadata를 신뢰하지 않으며 resolver는 런타임에만 존재합니다.

```go
// ProjectInteraction is a runtime-only projection using P1-owned policy types.
// It derives interaction_id and present application_id/guild_id/channel_id from
// the source, never from invented caller IDs, and passes no raw token to policy.
// It validates the binding's backend, purpose, exact scope and future finite expiry.
// The authenticated ctx is passed unchanged; only encoder receives the raw secret.
func ProjectInteraction(ctx context.Context, src *discordgo.Interaction, encoder CredentialEncoder, policy ProjectionPolicy) (*discordv2.Interaction, error)

// ProjectWebhook derives webhook_id and other present source resource IDs,
// obtains the trusted policy binding and applies the same validation.
// A token-free source stays token-free; secrets and token-bearing URLs never
// reach policy, module DTOs or diagnostics.
func ProjectWebhook(ctx context.Context, src *discordgo.Webhook, encoder CredentialEncoder, policy ProjectionPolicy) (*discordv2.Webhook, error)

// InteractionHandle keeps the SDK convenience value separate from its opaque
// credential. Value.Token is always empty; Credential is not a Discord token.
type InteractionHandle struct {
    Value      *discordgo.Interaction
    Credential *corev2.CredentialRef
}

// WebhookHandle follows the same separation and never reconstructs raw Token.
type WebhookHandle struct {
    Value      *discordgo.Webhook
    Credential *corev2.CredentialRef
}

// ProtoToInteractionHandle preserves typed interaction data and the reference
// while leaving every raw token field empty. It does not resolve credentials.
func ProtoToInteractionHandle(src *discordv2.Interaction) (*InteractionHandle, error)

// InteractionHandleToProto accepts only a sanitized value and an existing
// typed reference. It never mints a credential from an arbitrary string.
func InteractionHandleToProto(src *InteractionHandle) (*discordv2.Interaction, error)

// ProtoToWebhookHandle reconstructs a sanitized SDK webhook plus its typed
// reference, without inventing a token usable outside the runtime boundary.
func ProtoToWebhookHandle(src *discordv2.Webhook) (*WebhookHandle, error)

// WebhookHandleToProto preserves that explicit separation and rejects any
// non-empty raw token supplied in Value.
func WebhookHandleToProto(src *WebhookHandle) (*discordv2.Webhook, error)
```

두 projection은 실제 source의 `interaction_id` 또는 `webhook_id` 및 존재하는 `application_id`, `guild_id`, `channel_id`로 `CredentialTarget`을 구성합니다. policy가 반환한 binding의 backend와 purpose는 신뢰된 P3 작업에, selector는 source target에 정확히 일치해야 하며 expiry는 유한한 미래 시각이어야 합니다. raw secret은 policy에 주지 않고 검증된 binding과 함께 encoder에만 전달합니다. context를 바꾸지 않으며 대상 인스턴스의 인증과 승인은 P4가 담당합니다. expiry를 converter 기본값이나 untrusted ref metadata로 대신하지 않고 모든 호출자는 명시적인 네 번째 `policy` 인자로 이관하며 3인자 호환 alias를 남기지 않습니다.

다른 순수 객체의 중첩 `User.Token`도 같은 금지 규칙을 따릅니다. runtime projection은 전체 객체 그래프, 알려진 credential URL 필드, raw 및 오류 표현을 정제한 뒤 모듈 경계로 내보냅니다. 변환기는 token을 발견했을 때 무조건 삭제하고 성공하는 대신 trusted projection을 요구하여, accidental 호출도 감지할 수 있게 합니다. 모듈 측 handle로 들어온 credential reference의 유효성, 소유권, 만료 및 목적은 호출 실행 시 P3/P4에서 확인합니다.

#### 업로드 파일의 편의 변환

```go
// FileAttachment refers to a completed P1 upload owned by the calling module
// instance. Filename and Description are request metadata, not host paths.
// Ref metadata is descriptive; the runtime upload store remains authoritative.
type FileAttachment struct {
    Ref         *corev2.UploadedFileRef
    Filename    string
    Description *string
}

// FileAttachmentToProto maps Ref to uploaded_file_ref and retains metadata.
// It never reads files, uploads content, or converts io.Reader to nil.
func FileAttachmentToProto(src *FileAttachment) (*discordv2.FileAttachment, error)

// ProtoToFileAttachment returns a reference-bearing value rather than a fake
// discordgo.File with an unusable Reader. Runtime read leases belong to P3.
func ProtoToFileAttachment(src *discordv2.FileAttachment) (*FileAttachment, error)
```

`MessageSend`, `MessageEdit`, `WebhookParams`, `WebhookEdit` 및 `InteractionResponseData`의 파일 필드는 모두 이 표현을 사용합니다. 나머지 안전한 SDK 필드와 file-reference 목록을 묶은 각 요청 전용 view를 제공하고, 오래된 `Files []*discordgo.File`을 채워 반환하는 가짜 역변환은 제거합니다. 기존 local Reader를 사용하는 호출부는 먼저 명시적인 업로드를 수행한 뒤 reference를 요청에 넣도록 마이그레이션합니다. P3의 `AcquireReadLease(ctx, principal, ref)` 및 lease의 `Open(ctx)`가 실제 읽기를 소유하며, 각 요청 또는 허용된 실행 시도는 필요한 새 reader를 엽니다. 업로드 성공, TTL, 인스턴스 소유권, 삭제 및 multipart streaming은 P3의 계약을 따릅니다.

기존 attachment entry의 선택적 filename/description은 P1에 따라 endpoint가 허용하는 변경만 보존합니다. 미지정은 기존 값을 유지하고 빈 description은 설명 제거이며, 빈 filename이나 지원하지 않는 null은 오류입니다. 새 index 항목의 filename/description은 생략할 수 있지만 지정하면 `FileAttachment.Filename` 및 `Description`의 값과 존재 여부에 일치해야 합니다. 새 파일 metadata의 기준을 entry로 몰래 옮기거나 서로 다른 값을 덮어쓰지 않습니다. `AttachmentEditSource`는 한 interface 값에 하나의 concrete value만 담으며, converter와 P3 admission이 nil 및 허용하지 않은 variant를 거부하여 정확히 하나의 선택을 강제합니다.

#### 공통 의사코드

```text
CONVERT_TYPED_OBJECT(source):
    if source is absent: return absent, success
    allocate the destination snapshot for declared fields
    for every declared field in the field inventory:
        if field is credential/runtime-only/file-reader:
            require its explicitly declared safe view or boundary projection
        if converting an SDK source field with a reviewed omission policy:
            if its empty string or numeric zero matches that field's policy:
                leave the wire field absent and continue
        if field has meaningful presence:
            preserve absent, present-value, present-empty and allowed null separately
        if field is a map:
            convert each value using the original input key
        if field is a collection:
            convert in input order; do not drop failed or nil elements
        if field is a numeric/string discriminator or mask:
            apply the declared mapping and destination range check
        if field contains dynamic JSON:
            validate exact JSON and presence without converting numbers to double
        on any error:
            return no partial output and a sanitized error with field path
    return destination, success

PROJECT_RUNTIME_OBJECT(context, trusted_source):
    obtain the authenticated instance and approved projection scope
    capture required JSON presence before SDK decoding loses it
    replace approved credential fields through P1 CredentialEncoder
    reject prohibited credentials, session internals and unsafe raw representations
    convert every permitted domain field into its typed DTO
    return the complete sanitized DTO or an error

DECODE_INTEGER_OPTION(wire):
    require integer discriminator and an integral, allowed-range value
    create SDK option with its original integer Type
    assign Value = float64(exact integer)
    preserve focus and nested options
    return option that the local IntValue accessor can consume

APPLY_TYPED_PATCH(wire_request):
    preserve absent fields by excluding their JSON keys
    preserve present empty values and collections by emitting their keys
    if the operation edits existing message attachments:
        require a present complete list whenever new Files is nonempty
        validate one source per entry and each new index exactly once, including zero
        preserve absent versus empty; never fetch retained IDs or append implicitly
        validate entry metadata against the selected FileAttachment
    emit null only where the endpoint contract explicitly permits it
    use direct typed request encoding if the SDK representation loses presence
    resolve credentials and file readers only at the authenticated P3/P4 boundary
```

### 5-2. Error Handling

| 오류 | 발생 조건 및 처리 |
|---|---|
| `InvalidValue` | field 값과 선언된 타입이 다르거나 timestamp가 잘못되었거나 실제 객체 cycle이 있습니다. 위치를 포함한 오류를 반환합니다. |
| `OutOfRange` | 권한, mask, enum underlying 값 또는 option 숫자를 목적 SDK 타입에 안전하게 넣을 수 없습니다. narrowing, wraparound 또는 반올림하지 않습니다. |
| `UnsupportedVariant` | 알려지지 않은 component oneof나 현재 계약으로 표현할 수 없는 interaction data variant입니다. nil 성공, 기본 variant 대입 또는 로그 후 누락을 하지 않습니다. |
| `InvalidPresence` | legacy SDK 입력만으로 필수 presence를 알 수 없거나 해당 endpoint가 지원하지 않는 null/presence 조합입니다. attachment source 미지정, 새 파일이 있는데 목록이 없거나 index가 누락된 경우도 포함합니다. typed view와 완전한 목록을 요구합니다. |
| `InvalidJSON` | audit old/new 등 값이 하나의 유효한 JSON이 아니거나 정확한 JSON 값으로 직렬화할 수 없습니다. 빈 bytes로 대체하지 않습니다. |
| `CredentialBoundaryRequired` | 순수 변환에 raw token, runtime credential을 포함한 URL/JSON 또는 승인된 typed replacement가 없는 credential 값이 들어왔습니다. 데이터와 secret을 오류 메시지에 포함하지 않습니다. |
| `UploadedFileRefRequired` | 원본 Reader나 host path만 전달되었거나 필수 업로드 참조가 없습니다. 자동 업로드하거나 `Reader=nil` 파일을 만들지 않습니다. |
| `RuntimeObjectNotTransportable` | 제거된 live Session/Identify/State 등의 실행 객체를 wire로 사용하려는 호출입니다. 지원되는 typed 도메인 경로를 명시합니다. |

오류의 문자열에는 secret, 원본 요청 body 또는 임의 JSON을 넣지 않습니다. nested 오류는 안정적인 field path로 추적하며 모듈을 식별하는 권한 판단을 변환 오류에서 수행하지 않습니다.

이 함수들은 REST API가 아니므로 HTTP 상태 코드를 정의하지 않습니다. P3/P4는 호출자 입력에서 발생한 오류와 trusted Discord 응답의 변환 실패를 구분하여 공통 gRPC 오류 계약에 매핑합니다. trusted 응답이 잘못되었다고 호출자에게 입력 오류를 돌려주지 않으며, 이벤트 인코딩 실패를 정상 전달 또는 조용한 누락으로 처리하지 않습니다. credential 만료와 업로드 lease 실패는 실행 경계의 실제 오류이며 변환기의 fake 성공으로 덮지 않습니다.

## 6. Implementation Plan

### 6-1. Milestones

각 단계는 산출물과 완료 기준을 독립적으로 설명하지만 실제 통합 순서는 P0, P1, P2/P4, P3, P5/P6 의존성을 따릅니다. 변환기만 완성한 중간 결과를 권한과 격리가 구현된 런타임으로 배포하지 않습니다.

| Phase | Task | Estimated Duration | Owner |
|-------|------|--------------------|-------|
| Phase 1 | 154개 메시지와 23개 enum 및 현재 로컬 SDK 필드를 입력으로 field-level 분류를 완성합니다. P1과 optional, numeric, credential, JSON, runtime-only 경계를 고정합니다. 완료 조건은 모든 기존 필드가 완전 변환, inline, wrapper 또는 명시적 replacement/removal 중 하나에 배정되고 미분류 필드가 없는 것입니다. | 미정 | 미정 |
| Phase 2 | 고정된 v2 계약에 따라 공통 오류, nil/partial 처리, permission/mask 및 scalar 매핑을 구현하고 활성 72개와 inline 2개의 전체 필드를 수정합니다. 완료 조건은 상위 객체에서 하위 오류가 전파되고 4.3의 활성 필드 손실 및 소비자 accessor 결함을 재현하는 회귀 조건이 충족되는 것입니다. | 미정 | 미정 |
| Phase 3 | 비활성 39개, 미구현 28개, wrapper 11개를 완성하고 방향별 중복을 제거합니다. 완료 조건은 audit JSON의 실제 값과 presence, 모든 component variant 및 중첩 데이터가 양방향으로 보존되며 비활성 또는 no-op 경로가 남지 않는 것입니다. | 미정 | 미정 |
| Phase 4 | `File` 및 `WebhookParams`의 단방향 경로, credential handle, patch view 및 신뢰된 raw presence capture를 P1/P3/P4 경계에 연결합니다. 완료 조건은 secret 불전송, 명시적 zero/clear/null 요청, 실제 파일 reference 전달 및 정제된 이벤트 payload가 소비자까지 유지되는 것입니다. | 미정 | 미정 |
| Phase 5 | P3의 전체 bot API 및 P1/P3의 신규 이벤트 DTO를 대상으로 reachable type closure를 완성하고 모든 caller/test를 v2로 옮깁니다. 완료 조건은 신규 DTO도 필드별 분류가 있고 오래된 v1 변환, duplicate, shim 및 fake Reader 경로가 제거되는 것입니다. | 미정 | 미정 |
| Phase 6 | 변환 규칙, 경계 교체 목록, error 계약 및 마이그레이션 설명을 저장소 문서와 변경 기록에 반영하고 P6의 통합 회귀에 인계합니다. 완료 조건은 아래 소비자 관찰 항목이 별도 기대값으로 검증되며 실제 실행 여부와 결과가 구분되어 기록되는 것입니다. | 미정 | 미정 |

#### 소비자에게 보이는 회귀 완료 기준

다음은 구현 뒤 수행할 검증 항목이며 이 문서를 작성하며 통과했다고 주장하는 결과가 아닙니다.

| 관찰 대상 | 완료 조건 |
|---|---|
| 전체 타입 및 필드 | 기존 154개와 추가된 DTO의 분류, 양방향 field mapping 및 경계 교체가 일치합니다. 변환 함수가 있다는 사실만 세지 않습니다. |
| 일반 메시지 및 관리 명령 | mentions, reply/reference, reaction, poll, sticker, thread 및 권한 값이 실제 wire 왕복과 소비자 객체에서 보존됩니다. 미디어 도구를 요구하지 않습니다. |
| 명령 권한과 context | `DefaultMemberPermissions` nil과 0, context 및 integration type 목록, localizations와 autocomplete가 유지됩니다. Discord 최종 사용자 권한을 FlexModule capability로 대체하지 않습니다. |
| interaction 종류 | command, component, modal 및 autocomplete payload마다 맞는 SDK 접근 방식을 사용합니다. 일반 이벤트 처리에서 command 전용 accessor를 강제로 호출하지 않습니다. |
| option accessor | integer와 number의 0, 허용 경계, 잘못된 동적 타입, role/mentionable/attachment ID, nested options 및 focus가 맞게 처리됩니다. 알려진 `IntValue()` panic이 재발하지 않습니다. |
| patch | 미설정은 키를 보내지 않고, 명시적 빈 content/components/embeds/attachments 및 false/0은 실제 요청 JSON에 남습니다. 허용된 null은 빈 문자열과 구분됩니다. 새 파일 없는 attachment 미지정, 명시적 전체 제거, 기존 ID와 새 index `0` 혼합을 보존하고, 새 파일의 누락/중복 index, 빈 source 및 metadata 충돌을 모든 메시지 수정/응답 갱신 경로에서 거부합니다. |
| partial 및 nil | 부분 member/channel, 빠진 nested activity, nil timestamp, nil root 및 typed nil component에 대해 규정된 값 또는 오류를 반환하고 panic하지 않습니다. |
| resolved map | map key와 embedded ID가 달라도 원래 key를 유지하며 `Member.User=nil`인 상태로 조회할 수 있습니다. |
| enum 및 mask | 알려진 모든 상수, 비트 조합, 표현 가능한 미지 값, 범위 밖 값 및 빈 문자열의 기대 결과가 명시됩니다. encode/decode가 같은 잘못된 표를 공유하는 왕복 테스트만으로 통과시키지 않습니다. |
| audit 및 동적 JSON | absent, null, false, 0, 빈 문자열, 빈 collection, 중첩 값 및 double 정밀도를 넘는 정수의 exact JSON 값이 유지됩니다. no-op bytes 결과가 없습니다. |
| 파일 | source bytes가 metadata-only 성공으로 사라지지 않습니다. 공용 `FileAttachment.Ref`가 wire `uploaded_file_ref`에 대응하고 모든 파일 수용 API에서 유지됩니다. 첨부 수정의 request-specific 목록과 응답 `MessageAttachment`를 혼용하지 않으며, 실제 업로드/lease/stream 검증은 P3/P6 결과와 연결됩니다. |
| credential | interaction/webhook/user credential ref가 실제 raw Token 필드로 복원되지 않습니다. nested user, URL, raw 또는 오류 경로를 포함하여 runtime-owned secret이 모듈 경계를 넘지 않습니다. |
| snapshot 정확성 | 서로 다른 Width/Height 및 Available/Animated 값을 넣어 잘못된 필드 대입을 잡고, 입력 객체의 변경이 출력 snapshot을 바꾸지 않습니다. |
| 이벤트 presence | update의 omitted와 explicit zero가 모듈 typed payload에서 구분됩니다. 없는 BeforeUpdate/BeforeDelete와 cache 객체를 만들어 내지 않습니다. |
| 호출부 전환 | helper, hook, 일반 예제, 선택적 음성 예제 및 모든 관련 테스트가 오류를 처리하며 이전 no-error 변환이나 raw-token 시그니처를 사용하지 않습니다. |

fixture는 각 SDK 선언, canonical wire 값 및 실제 요청 JSON의 별도 기대값에서 작성합니다. property/round-trip 검사만으로 숫자 offset이나 대칭적인 필드 누락을 놓치지 않도록 golden 기대값과 malformed 입력 검사를 함께 둡니다. 라이브 Discord 검증은 P6의 승인된 환경에서 별도 수행하며, 이 제안은 라이브 실행이나 gVisor 검증 결과를 포함하지 않습니다.

### 6-2. Dependencies

- [P0](flexmodule-0-build-and-dependency-baseline.md)의 로컬 DiscordGo 일관성, 컴파일 복구 및 고정 생성 도구가 선행 입력입니다.
- [P1](flexmodule-1-protocol-contracts.md)의 v2 스키마, presence, numeric mapping, credential reference 및 이벤트 allowlist가 선행 계약입니다. 컴파일을 맞추기 위해 이 계약을 임의로 좁히지 않습니다.
- [P3](flexmodule-3-discord-backend.md)는 전체 공식 bot API 분류, typed 요청/응답, 파일 저장소, read lease, streaming multipart 및 직접 patch 인코딩을 제공합니다. 기존 154개 밖의 DTO도 P2와 함께 완성합니다.
- [P4](flexmodule-4-runtime-and-isolation.md)는 authenticated principal, 인스턴스 수명 및 자원/credential 소유권을 제공합니다. 순수 변환기 테스트로 이 권한 경계까지 검증했다고 간주하지 않습니다.
- [P5](flexmodule-5-voice-jobs.md)는 선택적 voice job과 receive 기능을 사용하며 세션 credential이나 live voice connection 변환에 의존하지 않습니다.
- [P6](flexmodule-6-examples-and-validation.md)는 일반 봇 및 선택적 음성 예제의 소비자 수준 회귀와 실제 통합 검증을 소유합니다.
- 외부 타입 기준은 현재 저장소의 로컬 DiscordGo입니다. 새로운 upstream API를 가져오기 위한 광범위한 SDK 업그레이드는 포함하지 않습니다.

## 7. References

현재 변환 구현은 [shared/discord-v2](../../shared/discord-v2/), wire 계약은 [proto/discord-v2](../../proto/discord-v2/)에 있습니다. 아래 v1 경로와 부록의 source/line 참조는 2026-10-29 감사의 역사적 근거이며 현재 파일 위치가 아닙니다. 현행 inventory의 `discordgo/*.go`는 논리적 source label이며 SDK source는 [tools/sdk.py](../../tools/sdk.py)로 해석합니다.

- [현재 Discord 도메인 스키마](../../proto/discord-v1/discordgo.proto), [별도 ChannelEdit 및 helper 스키마](../../proto/discord-v1/helper.proto), [현재 hook 스키마](../../proto/discord-v1/hook.proto)
- [현재 정방향 message 변환](../../shared/discord-v1/convert/struct2buf/message.go), [현재 역방향 message 변환](../../shared/discord-v1/convert/buf2struct/message.go)
- [현재 정방향 interaction 변환](../../shared/discord-v1/convert/struct2buf/interactions.go), [현재 역방향 interaction 변환](../../shared/discord-v1/convert/buf2struct/interactions.go)
- [현재 정방향 component 변환](../../shared/discord-v1/convert/struct2buf/components.go), [현재 역방향 component 변환](../../shared/discord-v1/convert/buf2struct/components.go)
- [현재 정방향 activity 변환](../../shared/discord-v1/convert/struct2buf/activity.go), [현재 역방향 activity 변환](../../shared/discord-v1/convert/buf2struct/activity.go)
- [현재 정방향 도메인 구조 변환](../../shared/discord-v1/convert/struct2buf/structs.go), [현재 역방향 도메인 구조 변환](../../shared/discord-v1/convert/buf2struct/structs.go)
- [현재 비활성 application 변환](../../shared/discord-v1/convert/struct2buf/application.go.disabled), [현재 비활성 misc 변환](../../shared/discord-v1/convert/struct2buf/misc.go.disabled), [현재 비활성 audit 변환](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled), [현재 비활성 monetization 변환](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled)
- [현재 역방향 비활성 application 변환](../../shared/discord-v1/convert/buf2struct/application.go.disabled), [현재 역방향 비활성 misc 변환](../../shared/discord-v1/convert/buf2struct/misc.go.disabled), [현재 역방향 비활성 audit 변환](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled), [현재 역방향 비활성 monetization 변환](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled)
- [로컬 DiscordGo message 타입](../../discordgo/message.go), [interaction 타입 및 accessor](../../discordgo/interactions.go), [component 타입](../../discordgo/components.go), [domain 타입](../../discordgo/structs.go), [user 타입](../../discordgo/user.go), [webhook 타입](../../discordgo/webhook.go), [live state 구현](../../discordgo/state.go)
- [Protobuf field presence](https://protobuf.dev/programming-guides/field_presence/), [Protobuf enum 표현](https://protobuf.dev/programming-guides/proto3/#enum)
- [Discord application command 데이터 및 숫자 범위](https://docs.discord.com/developers/interactions/application-commands.md)
- [Discord 파일 업로드](https://docs.discord.com/developers/reference#uploading-files), [메시지 수정](https://docs.discord.com/developers/resources/message#edit-message)
- [P0 빌드 기준선](flexmodule-0-build-and-dependency-baseline.md), [P1 프로토콜 계약](flexmodule-1-protocol-contracts.md), [P3 Discord backend](flexmodule-3-discord-backend.md), [P4 런타임 및 격리](flexmodule-4-runtime-and-isolation.md), [P5 voice job](flexmodule-5-voice-jobs.md), [P6 예제 및 검증](flexmodule-6-examples-and-validation.md)

### 7.1 활성 named 72개 및 나머지 특수 분류의 원본 범위

현재 활성 named 양방향 72개는 다음과 같습니다. 이 목록은 현황 분류이며 모든 항목을 v2의 동일 이름 실행 객체로 유지한다는 뜻이 아닙니다. `Session`, `Identify`, `State`는 4.2의 명시적 제거 계약을 적용합니다.

`Message`, `MessageSend`, `MessageEdit`, `MessageAllowedMentions`, `MessageAttachment`, `MessageEmbedFooter`, `MessageEmbedImage`, `MessageEmbedThumbnail`, `MessageEmbedVideo`, `MessageEmbedProvider`, `MessageEmbedAuthor`, `MessageEmbedField`, `MessageEmbed`, `MessageReactions`, `Session`, `Identify`, `IdentifyProperties`, `GatewayStatusUpdate`, `Activity`, `TimeStamps`, `Assets`, `Party`, `Secrets`, `VoiceRegion`, `Channel`, `PermissionOverwrite`, `ThreadMetadata`, `ThreadMember`, `ForumDefaultReaction`, `ForumTag`, `Emoji`, `Guild`, `Role`, `VoiceState`, `Presence`, `Member`, `ClientStatus`, `GatewayBotResponse`, `StageInstance`, `State`, `User`, `Webhook`, `WebhookEdit`, `ComponentEmoji`, `MessageComponent`, `SelectMenuOption`, `SelectMenuDefaultValue`, `UnfurledMediaItem`, `MediaGalleryItem`, `ActionsRow`, `Button`, `SelectMenu`, `TextInput`, `Section`, `TextDisplay`, `Thumbnail`, `MediaGallery`, `FileComponent`, `Separator`, `Container`, `ApplicationCommand`, `ApplicationCommandOption`, `ApplicationCommandOptionChoice`, `Interaction`, `ApplicationCommandInteractionData`, `ApplicationCommandInteractionDataResolved`, `MessageComponentInteractionData`, `MessageComponentInteractionDataResolved`, `ModalSubmitInteractionData`, `ApplicationCommandInteractionDataOption`, `InteractionResponse`, `InteractionResponseData`.

단방향 두 개는 역방향만 활성인 `File`과 정방향만 활성인 `WebhookParams`입니다. inline 두 개는 `MessageActivity`와 `MessageApplication`입니다. 비활성 39개, 미구현 28개, wrapper 11개 및 enum 23개의 전체 이름과 처리 규칙은 4.3에 기록했습니다. 이 분류와 추가된 API/event DTO의 field-level inventory를 함께 완료 기준으로 사용합니다.


### 부록 A. 기존 154개 메시지의 전체 전환 목록

다음은 업그레이드 전 v1 스키마와 변환 소스의 역사적 감사 기준선입니다. 메시지 154개 및 enum 23개는 현재 범위의 상한이 아닙니다. 원본 source 링크와 필드 후보를 감사 근거로 보존하며 현재 signature와 field는 본문의 delta 및 source catalog를 따릅니다. 활성 함수 존재나 `추가 후보 없음`은 정확성 검증 통과를 뜻하지 않습니다. inline 처리와 의도적인 경계 제거는 본문 규칙으로 판정하며 Session, Identify, State의 실행 객체도 명시적 제거와 안전한 대체 대상으로 남깁니다.

| 원본 메시지와 스키마 | 현재 분류 | 정방향 근거 및 미대입 후보 | 역방향 근거 및 미사용 후보 | 추가 Go 필드 후보 / 전환 경계 |
|---|---|---|---|---|
| `Message`<br>[proto/discord-v1/discordgo.proto:34](../../proto/discord-v1/discordgo.proto#L34) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:12](../../shared/discord-v1/convert/struct2buf/message.go#L12)<br>`Flags`, `Interaction`, `InteractionMetadata`, `Mentions`, `MessageReference`, `MessageSnapshots`, `Poll`, `Reactions`, `ReferencedMessage`, `StickerItems`, `Thread` | [shared/discord-v1/convert/buf2struct/message.go:9](../../shared/discord-v1/convert/buf2struct/message.go#L9)<br>`Flags`, `Interaction`, `InteractionMetadata`, `Mentions`, `MessageReference`, `MessageSnapshots`, `Poll`, `Reactions`, `ReferencedMessage`, `StickerItems`, `Thread` | Go 미사용: `Flags`, `Interaction`, `InteractionMetadata`, `Mentions`, `MessageReference`, `MessageSnapshots`, `Poll`, `Reactions`, `ReferencedMessage`, `StickerItems`, `Thread` |
| `File`<br>[proto/discord-v1/discordgo.proto:85](../../proto/discord-v1/discordgo.proto#L85) | 한 방향만 활성 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:99](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L99) (disabled) | [shared/discord-v1/convert/buf2struct/interactions.go:411](../../shared/discord-v1/convert/buf2struct/interactions.go#L411)<br>추가 후보 없음 | Reader는 UploadedFileRef/FileAttachment로 교체합니다. fake Reader 역변환을 제거합니다. |
| `MessageSend`<br>[proto/discord-v1/discordgo.proto:93](../../proto/discord-v1/discordgo.proto#L93) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:242](../../shared/discord-v1/convert/struct2buf/message.go#L242)<br>`AllowedMentions`, `Components`, `Flags`, `Poll`, `Reference` | [shared/discord-v1/convert/buf2struct/message.go:223](../../shared/discord-v1/convert/buf2struct/message.go#L223)<br>`AllowedMentions`, `Components`, `Flags`, `Poll`, `Reference` | Go 미사용: `AllowedMentions`, `Components`, `Embed`, `File`, `Flags`, `Poll`, `Reference` |
| `MessageEdit`<br>[proto/discord-v1/discordgo.proto:107](../../proto/discord-v1/discordgo.proto#L107) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:273](../../shared/discord-v1/convert/struct2buf/message.go#L273)<br>`AllowedMentions`, `Components`, `Flags` | [shared/discord-v1/convert/buf2struct/message.go:254](../../shared/discord-v1/convert/buf2struct/message.go#L254)<br>추가 후보 없음 | Go 미사용: `AllowedMentions`, `Components`, `Embed`, `Flags`<br>patch와 첨부파일은 존재성이 있는 요청 전용 view를 사용합니다. |
| `MessageAllowedMentions`<br>[proto/discord-v1/discordgo.proto:130](../../proto/discord-v1/discordgo.proto#L130) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:315](../../shared/discord-v1/convert/struct2buf/message.go#L315)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:316](../../shared/discord-v1/convert/buf2struct/message.go#L316)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageAttachment`<br>[proto/discord-v1/discordgo.proto:138](../../proto/discord-v1/discordgo.proto#L138) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:82](../../shared/discord-v1/convert/struct2buf/message.go#L82)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:80](../../shared/discord-v1/convert/buf2struct/message.go#L80)<br>추가 후보 없음 | Go 미사용: `DurationSecs`, `Flags`, `Waveform` |
| `MessageEmbedFooter`<br>[proto/discord-v1/discordgo.proto:151](../../proto/discord-v1/discordgo.proto#L151) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:157](../../shared/discord-v1/convert/struct2buf/message.go#L157)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:137](../../shared/discord-v1/convert/buf2struct/message.go#L137)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageEmbedImage`<br>[proto/discord-v1/discordgo.proto:158](../../proto/discord-v1/discordgo.proto#L158) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:169](../../shared/discord-v1/convert/struct2buf/message.go#L169)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:149](../../shared/discord-v1/convert/buf2struct/message.go#L149)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageEmbedThumbnail`<br>[proto/discord-v1/discordgo.proto:166](../../proto/discord-v1/discordgo.proto#L166) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:182](../../shared/discord-v1/convert/struct2buf/message.go#L182)<br>`Width` | [shared/discord-v1/convert/buf2struct/message.go:162](../../shared/discord-v1/convert/buf2struct/message.go#L162)<br>추가 후보 없음 | Go 미사용: `Width` |
| `MessageEmbedVideo`<br>[proto/discord-v1/discordgo.proto:174](../../proto/discord-v1/discordgo.proto#L174) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:194](../../shared/discord-v1/convert/struct2buf/message.go#L194)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:175](../../shared/discord-v1/convert/buf2struct/message.go#L175)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageEmbedProvider`<br>[proto/discord-v1/discordgo.proto:181](../../proto/discord-v1/discordgo.proto#L181) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:206](../../shared/discord-v1/convert/struct2buf/message.go#L206)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:187](../../shared/discord-v1/convert/buf2struct/message.go#L187)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageEmbedAuthor`<br>[proto/discord-v1/discordgo.proto:187](../../proto/discord-v1/discordgo.proto#L187) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:217](../../shared/discord-v1/convert/struct2buf/message.go#L217)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:198](../../shared/discord-v1/convert/buf2struct/message.go#L198)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageEmbedField`<br>[proto/discord-v1/discordgo.proto:195](../../proto/discord-v1/discordgo.proto#L195) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:145](../../shared/discord-v1/convert/struct2buf/message.go#L145)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:125](../../shared/discord-v1/convert/buf2struct/message.go#L125)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageEmbed`<br>[proto/discord-v1/discordgo.proto:202](../../proto/discord-v1/discordgo.proto#L202) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:100](../../shared/discord-v1/convert/struct2buf/message.go#L100)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:98](../../shared/discord-v1/convert/buf2struct/message.go#L98)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageReactions`<br>[proto/discord-v1/discordgo.proto:230](../../proto/discord-v1/discordgo.proto#L230) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/message.go:230](../../shared/discord-v1/convert/struct2buf/message.go#L230)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/message.go:211](../../shared/discord-v1/convert/buf2struct/message.go#L211)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageActivity`<br>[proto/discord-v1/discordgo.proto:237](../../proto/discord-v1/discordgo.proto#L237) | Message 내부 inline | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageApplication`<br>[proto/discord-v1/discordgo.proto:251](../../proto/discord-v1/discordgo.proto#L251) | Message 내부 inline | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageSnapshot`<br>[proto/discord-v1/discordgo.proto:261](../../proto/discord-v1/discordgo.proto#L261) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageReference`<br>[proto/discord-v1/discordgo.proto:272](../../proto/discord-v1/discordgo.proto#L272) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageInteraction`<br>[proto/discord-v1/discordgo.proto:281](../../proto/discord-v1/discordgo.proto#L281) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageInteractionMetadata`<br>[proto/discord-v1/discordgo.proto:292](../../proto/discord-v1/discordgo.proto#L292) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Session`<br>[proto/discord-v1/discordgo.proto:306](../../proto/discord-v1/discordgo.proto#L306) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:141](../../shared/discord-v1/convert/struct2buf/activity.go#L141)<br>`Status` | [shared/discord-v1/convert/buf2struct/activity.go:150](../../shared/discord-v1/convert/buf2struct/activity.go#L150)<br>`Status` | Go 미사용: `Client`, `Dialer`, `Ratelimiter`, `VoiceConnections`<br>live runtime 객체를 wire에서 제거합니다. 필요한 도메인 값은 명시적인 snapshot으로 분리합니다. |
| `Identify`<br>[proto/discord-v1/discordgo.proto:334](../../proto/discord-v1/discordgo.proto#L334) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:104](../../shared/discord-v1/convert/struct2buf/activity.go#L104)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/activity.go:108](../../shared/discord-v1/convert/buf2struct/activity.go#L108)<br>추가 후보 없음 | live runtime 객체를 wire에서 제거합니다. 필요한 도메인 값은 명시적인 snapshot으로 분리합니다. |
| `IdentifyProperties`<br>[proto/discord-v1/discordgo.proto:344](../../proto/discord-v1/discordgo.proto#L344) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:126](../../shared/discord-v1/convert/struct2buf/activity.go#L126)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/activity.go:135](../../shared/discord-v1/convert/buf2struct/activity.go#L135)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GatewayStatusUpdate`<br>[proto/discord-v1/discordgo.proto:352](../../proto/discord-v1/discordgo.proto#L352) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:90](../../shared/discord-v1/convert/struct2buf/activity.go#L90)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/activity.go:89](../../shared/discord-v1/convert/buf2struct/activity.go#L89)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Activity`<br>[proto/discord-v1/discordgo.proto:359](../../proto/discord-v1/discordgo.proto#L359) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:10](../../shared/discord-v1/convert/struct2buf/activity.go#L10)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/activity.go:9](../../shared/discord-v1/convert/buf2struct/activity.go#L9)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `TimeStamps`<br>[proto/discord-v1/discordgo.proto:376](../../proto/discord-v1/discordgo.proto#L376) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:34](../../shared/discord-v1/convert/struct2buf/activity.go#L34)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/activity.go:33](../../shared/discord-v1/convert/buf2struct/activity.go#L33)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Assets`<br>[proto/discord-v1/discordgo.proto:381](../../proto/discord-v1/discordgo.proto#L381) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:46](../../shared/discord-v1/convert/struct2buf/activity.go#L46)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/activity.go:45](../../shared/discord-v1/convert/buf2struct/activity.go#L45)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Party`<br>[proto/discord-v1/discordgo.proto:388](../../proto/discord-v1/discordgo.proto#L388) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:60](../../shared/discord-v1/convert/struct2buf/activity.go#L60)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/activity.go:59](../../shared/discord-v1/convert/buf2struct/activity.go#L59)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Secrets`<br>[proto/discord-v1/discordgo.proto:393](../../proto/discord-v1/discordgo.proto#L393) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:77](../../shared/discord-v1/convert/struct2buf/activity.go#L77)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/activity.go:76](../../shared/discord-v1/convert/buf2struct/activity.go#L76)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationIntegrationTypeConfig`<br>[proto/discord-v1/discordgo.proto:400](../../proto/discord-v1/discordgo.proto#L400) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:43](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L43) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:43](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L43) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationInstallParams`<br>[proto/discord-v1/discordgo.proto:404](../../proto/discord-v1/discordgo.proto#L404) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:54](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L54) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:54](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L54) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `Application`<br>[proto/discord-v1/discordgo.proto:409](../../proto/discord-v1/discordgo.proto#L409) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:9](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L9) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:9](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L9) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationRoleConnectionMetadata`<br>[proto/discord-v1/discordgo.proto:431](../../proto/discord-v1/discordgo.proto#L431) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:66](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L66) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:66](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L66) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationRoleConnection`<br>[proto/discord-v1/discordgo.proto:440](../../proto/discord-v1/discordgo.proto#L440) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:92](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L92) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:92](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L92) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `UserConnection`<br>[proto/discord-v1/discordgo.proto:446](../../proto/discord-v1/discordgo.proto#L446) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:140](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L140) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:140](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L140) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `Integration`<br>[proto/discord-v1/discordgo.proto:454](../../proto/discord-v1/discordgo.proto#L454) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:160](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L160) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:160](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L160) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `IntegrationAccount`<br>[proto/discord-v1/discordgo.proto:469](../../proto/discord-v1/discordgo.proto#L469) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:182](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L182) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:182](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L182) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `VoiceRegion`<br>[proto/discord-v1/discordgo.proto:474](../../proto/discord-v1/discordgo.proto#L474) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/minimal.go:10](../../shared/discord-v1/convert/struct2buf/minimal.go#L10)<br>`Custom`, `Deprecated`, `Optimal` | [shared/discord-v1/convert/buf2struct/minimal.go:10](../../shared/discord-v1/convert/buf2struct/minimal.go#L10)<br>`Custom`, `Deprecated`, `Optimal` | Go 미사용: `Custom`, `Deprecated`, `Optimal` |
| `Invite`<br>[proto/discord-v1/discordgo.proto:482](../../proto/discord-v1/discordgo.proto#L482) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:177](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L177) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:176](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L176) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `Channel`<br>[proto/discord-v1/discordgo.proto:502](../../proto/discord-v1/discordgo.proto#L502) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:57](../../shared/discord-v1/convert/struct2buf/structs.go#L57)<br>`Members`, `Messages` | [shared/discord-v1/convert/buf2struct/structs.go:56](../../shared/discord-v1/convert/buf2struct/structs.go#L56)<br>`Members`, `Messages` | Go 미사용: `Members`, `Messages` |
| `PermissionOverwrite`<br>[proto/discord-v1/discordgo.proto:536](../../proto/discord-v1/discordgo.proto#L536) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:126](../../shared/discord-v1/convert/struct2buf/structs.go#L126)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:124](../../shared/discord-v1/convert/buf2struct/structs.go#L124)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ThreadStart`<br>[proto/discord-v1/discordgo.proto:543](../../proto/discord-v1/discordgo.proto#L543) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:125](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L125) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:124](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L124) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `ThreadMetadata`<br>[proto/discord-v1/discordgo.proto:552](../../proto/discord-v1/discordgo.proto#L552) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:139](../../shared/discord-v1/convert/struct2buf/structs.go#L139)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:137](../../shared/discord-v1/convert/buf2struct/structs.go#L137)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ThreadMember`<br>[proto/discord-v1/discordgo.proto:560](../../proto/discord-v1/discordgo.proto#L560) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:153](../../shared/discord-v1/convert/struct2buf/structs.go#L153)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:151](../../shared/discord-v1/convert/buf2struct/structs.go#L151)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ThreadsList`<br>[proto/discord-v1/discordgo.proto:568](../../proto/discord-v1/discordgo.proto#L568) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:141](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L141) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:140](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L140) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `AddedThreadMember`<br>[proto/discord-v1/discordgo.proto:574](../../proto/discord-v1/discordgo.proto#L574) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:164](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L164) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:163](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L163) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `ForumDefaultReaction`<br>[proto/discord-v1/discordgo.proto:580](../../proto/discord-v1/discordgo.proto#L580) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:115](../../shared/discord-v1/convert/struct2buf/structs.go#L115)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:113](../../shared/discord-v1/convert/buf2struct/structs.go#L113)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ForumTag`<br>[proto/discord-v1/discordgo.proto:585](../../proto/discord-v1/discordgo.proto#L585) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:167](../../shared/discord-v1/convert/struct2buf/structs.go#L167)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:165](../../shared/discord-v1/convert/buf2struct/structs.go#L165)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Emoji`<br>[proto/discord-v1/discordgo.proto:594](../../proto/discord-v1/discordgo.proto#L594) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:181](../../shared/discord-v1/convert/struct2buf/structs.go#L181)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:179](../../shared/discord-v1/convert/buf2struct/structs.go#L179)<br>`Available` | 본문의 전체 필드·presence 규칙 적용 |
| `EmojiParams`<br>[proto/discord-v1/discordgo.proto:605](../../proto/discord-v1/discordgo.proto#L605) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:219](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L219) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:218](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L218) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `Sticker`<br>[proto/discord-v1/discordgo.proto:611](../../proto/discord-v1/discordgo.proto#L611) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `StickerItem`<br>[proto/discord-v1/discordgo.proto:625](../../proto/discord-v1/discordgo.proto#L625) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `StickerPack`<br>[proto/discord-v1/discordgo.proto:631](../../proto/discord-v1/discordgo.proto#L631) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Guild`<br>[proto/discord-v1/discordgo.proto:641](../../proto/discord-v1/discordgo.proto#L641) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:199](../../shared/discord-v1/convert/struct2buf/structs.go#L199)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:343](../../shared/discord-v1/convert/buf2struct/structs.go#L343)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildPreview`<br>[proto/discord-v1/discordgo.proto:692](../../proto/discord-v1/discordgo.proto#L692) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildScheduledEvent`<br>[proto/discord-v1/discordgo.proto:705](../../proto/discord-v1/discordgo.proto#L705) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildScheduledEventParams`<br>[proto/discord-v1/discordgo.proto:724](../../proto/discord-v1/discordgo.proto#L724) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildScheduledEventEntityMetadata`<br>[proto/discord-v1/discordgo.proto:737](../../proto/discord-v1/discordgo.proto#L737) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildScheduledEventUser`<br>[proto/discord-v1/discordgo.proto:741](../../proto/discord-v1/discordgo.proto#L741) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildOnboardingMode`<br>[proto/discord-v1/discordgo.proto:747](../../proto/discord-v1/discordgo.proto#L747) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `GuildOnboarding`<br>[proto/discord-v1/discordgo.proto:752](../../proto/discord-v1/discordgo.proto#L752) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildOnboardingPromptType`<br>[proto/discord-v1/discordgo.proto:760](../../proto/discord-v1/discordgo.proto#L760) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `GuildOnboardingPrompt`<br>[proto/discord-v1/discordgo.proto:765](../../proto/discord-v1/discordgo.proto#L765) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildOnboardingPromptOption`<br>[proto/discord-v1/discordgo.proto:775](../../proto/discord-v1/discordgo.proto#L775) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildTemplate`<br>[proto/discord-v1/discordgo.proto:787](../../proto/discord-v1/discordgo.proto#L787) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildTemplateParams`<br>[proto/discord-v1/discordgo.proto:801](../../proto/discord-v1/discordgo.proto#L801) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `UserGuild`<br>[proto/discord-v1/discordgo.proto:806](../../proto/discord-v1/discordgo.proto#L806) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildParams`<br>[proto/discord-v1/discordgo.proto:817](../../proto/discord-v1/discordgo.proto#L817) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Role`<br>[proto/discord-v1/discordgo.proto:840](../../proto/discord-v1/discordgo.proto#L840) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:319](../../shared/discord-v1/convert/struct2buf/structs.go#L319)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:196](../../shared/discord-v1/convert/buf2struct/structs.go#L196)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `RoleFlags`<br>[proto/discord-v1/discordgo.proto:854](../../proto/discord-v1/discordgo.proto#L854) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `RoleParams`<br>[proto/discord-v1/discordgo.proto:860](../../proto/discord-v1/discordgo.proto#L860) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `VoiceState`<br>[proto/discord-v1/discordgo.proto:870](../../proto/discord-v1/discordgo.proto#L870) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:431](../../shared/discord-v1/convert/struct2buf/structs.go#L431)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:304](../../shared/discord-v1/convert/buf2struct/structs.go#L304)<br>추가 후보 없음 | SessionID는 host transport 내부에 남깁니다. |
| `Presence`<br>[proto/discord-v1/discordgo.proto:886](../../proto/discord-v1/discordgo.proto#L886) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:380](../../shared/discord-v1/convert/struct2buf/structs.go#L380)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:254](../../shared/discord-v1/convert/buf2struct/structs.go#L254)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MemberFlags`<br>[proto/discord-v1/discordgo.proto:894](../../proto/discord-v1/discordgo.proto#L894) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `Member`<br>[proto/discord-v1/discordgo.proto:899](../../proto/discord-v1/discordgo.proto#L899) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:29](../../shared/discord-v1/convert/struct2buf/structs.go#L29)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:28](../../shared/discord-v1/convert/buf2struct/structs.go#L28)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ClientStatus`<br>[proto/discord-v1/discordgo.proto:916](../../proto/discord-v1/discordgo.proto#L916) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:418](../../shared/discord-v1/convert/struct2buf/structs.go#L418)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:292](../../shared/discord-v1/convert/buf2struct/structs.go#L292)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `TooManyRequests`<br>[proto/discord-v1/discordgo.proto:922](../../proto/discord-v1/discordgo.proto#L922) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:49](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L49) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:48](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L48) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `ReadState`<br>[proto/discord-v1/discordgo.proto:928](../../proto/discord-v1/discordgo.proto#L928) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:62](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L62) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:61](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L61) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `GuildRole`<br>[proto/discord-v1/discordgo.proto:934](../../proto/discord-v1/discordgo.proto#L934) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:75](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L75) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:74](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L74) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `GuildBan`<br>[proto/discord-v1/discordgo.proto:939](../../proto/discord-v1/discordgo.proto#L939) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:87](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L87) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:86](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L86) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `AutoModerationRule`<br>[proto/discord-v1/discordgo.proto:944](../../proto/discord-v1/discordgo.proto#L944) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled:9](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled#L9) (disabled) | [shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled:9](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled#L9) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `AutoModerationTriggerMetadata`<br>[proto/discord-v1/discordgo.proto:958](../../proto/discord-v1/discordgo.proto#L958) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled:50](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled#L50) (disabled) | [shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled:39](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled#L39) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `AutoModerationActionType`<br>[proto/discord-v1/discordgo.proto:966](../../proto/discord-v1/discordgo.proto#L966) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `AutoModerationActionMetadata`<br>[proto/discord-v1/discordgo.proto:971](../../proto/discord-v1/discordgo.proto#L971) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled:87](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled#L87) (disabled) | [shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled:76](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled#L76) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `AutoModerationAction`<br>[proto/discord-v1/discordgo.proto:977](../../proto/discord-v1/discordgo.proto#L977) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled:75](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled#L75) (disabled) | [shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled:64](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled#L64) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `GuildEmbed`<br>[proto/discord-v1/discordgo.proto:982](../../proto/discord-v1/discordgo.proto#L982) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildAuditLog`<br>[proto/discord-v1/discordgo.proto:987](../../proto/discord-v1/discordgo.proto#L987) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled:100](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled#L100) (disabled) | [shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled:89](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled#L89) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `AuditLogEntry`<br>[proto/discord-v1/discordgo.proto:994](../../proto/discord-v1/discordgo.proto#L994) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled:134](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled#L134) (disabled) | [shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled:123](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled#L123) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `AuditLogChange`<br>[proto/discord-v1/discordgo.proto:1004](../../proto/discord-v1/discordgo.proto#L1004) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled:161](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled#L161) (disabled) | [shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled:147](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled#L147) (disabled) | old/new의 absence·null 및 exact JSON을 보존합니다. |
| `AuditLogOptions`<br>[proto/discord-v1/discordgo.proto:1013](../../proto/discord-v1/discordgo.proto#L1013) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled:186](../../shared/discord-v1/convert/struct2buf/moderation_audit.go.disabled#L186) (disabled) | [shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled:167](../../shared/discord-v1/convert/buf2struct/moderation_audit.go.disabled#L167) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `GuildMemberParams`<br>[proto/discord-v1/discordgo.proto:1028](../../proto/discord-v1/discordgo.proto#L1028) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildMemberAddParams`<br>[proto/discord-v1/discordgo.proto:1037](../../proto/discord-v1/discordgo.proto#L1037) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `APIErrorMessage`<br>[proto/discord-v1/discordgo.proto:1045](../../proto/discord-v1/discordgo.proto#L1045) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:37](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L37) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:36](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L36) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `MessageReaction`<br>[proto/discord-v1/discordgo.proto:1050](../../proto/discord-v1/discordgo.proto#L1050) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GatewayBotResponse`<br>[proto/discord-v1/discordgo.proto:1058](../../proto/discord-v1/discordgo.proto#L1058) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/minimal.go:22](../../shared/discord-v1/convert/struct2buf/minimal.go#L22)<br>`SessionStartLimit` | [shared/discord-v1/convert/buf2struct/minimal.go:22](../../shared/discord-v1/convert/buf2struct/minimal.go#L22)<br>`SessionStartLimit` | Go 미사용: `SessionStartLimit` |
| `SessionInformation`<br>[proto/discord-v1/discordgo.proto:1064](../../proto/discord-v1/discordgo.proto#L1064) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/misc.go.disabled:23](../../shared/discord-v1/convert/struct2buf/misc.go.disabled#L23) (disabled) | [shared/discord-v1/convert/buf2struct/misc.go.disabled:22](../../shared/discord-v1/convert/buf2struct/misc.go.disabled#L22) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `StageInstance`<br>[proto/discord-v1/discordgo.proto:1071](../../proto/discord-v1/discordgo.proto#L1071) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:454](../../shared/discord-v1/convert/struct2buf/structs.go#L454)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:326](../../shared/discord-v1/convert/buf2struct/structs.go#L326)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `StageInstanceParams`<br>[proto/discord-v1/discordgo.proto:1081](../../proto/discord-v1/discordgo.proto#L1081) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `PollLayoutType`<br>[proto/discord-v1/discordgo.proto:1088](../../proto/discord-v1/discordgo.proto#L1088) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `PollMedia`<br>[proto/discord-v1/discordgo.proto:1093](../../proto/discord-v1/discordgo.proto#L1093) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:31](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L31) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:31](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L31) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `PollAnswer`<br>[proto/discord-v1/discordgo.proto:1098](../../proto/discord-v1/discordgo.proto#L1098) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:43](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L43) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:43](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L43) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `PollAnswerCount`<br>[proto/discord-v1/discordgo.proto:1103](../../proto/discord-v1/discordgo.proto#L1103) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:55](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L55) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:55](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L55) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `PollResults`<br>[proto/discord-v1/discordgo.proto:1109](../../proto/discord-v1/discordgo.proto#L1109) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:68](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L68) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:68](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L68) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `Poll`<br>[proto/discord-v1/discordgo.proto:1114](../../proto/discord-v1/discordgo.proto#L1114) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:9](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L9) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:9](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L9) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `SKUType`<br>[proto/discord-v1/discordgo.proto:1124](../../proto/discord-v1/discordgo.proto#L1124) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `SKUFlags`<br>[proto/discord-v1/discordgo.proto:1129](../../proto/discord-v1/discordgo.proto#L1129) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `SKU`<br>[proto/discord-v1/discordgo.proto:1134](../../proto/discord-v1/discordgo.proto#L1134) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:154](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L154) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:151](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L151) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `SubscriptionStatus`<br>[proto/discord-v1/discordgo.proto:1143](../../proto/discord-v1/discordgo.proto#L1143) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `Subscription`<br>[proto/discord-v1/discordgo.proto:1148](../../proto/discord-v1/discordgo.proto#L1148) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:170](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L170) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:167](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L167) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `EntitlementType`<br>[proto/discord-v1/discordgo.proto:1161](../../proto/discord-v1/discordgo.proto#L1161) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `Entitlement`<br>[proto/discord-v1/discordgo.proto:1166](../../proto/discord-v1/discordgo.proto#L1166) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:98](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L98) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:98](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L98) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `EntitlementOwnerType`<br>[proto/discord-v1/discordgo.proto:1180](../../proto/discord-v1/discordgo.proto#L1180) | scalar wrapper | 전용 함수 없음 | 전용 함수 없음 | 원본 scalar 의미로 직접 매핑하며 불필요한 빈 변환 함수를 만들지 않습니다. |
| `EntitlementTest`<br>[proto/discord-v1/discordgo.proto:1185](../../proto/discord-v1/discordgo.proto#L1185) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:124](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L124) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:121](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L121) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `EntitlementFilterOptions`<br>[proto/discord-v1/discordgo.proto:1191](../../proto/discord-v1/discordgo.proto#L1191) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/monetization.go.disabled:137](../../shared/discord-v1/convert/struct2buf/monetization.go.disabled#L137) (disabled) | [shared/discord-v1/convert/buf2struct/monetization.go.disabled:134](../../shared/discord-v1/convert/buf2struct/monetization.go.disabled#L134) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `State`<br>[proto/discord-v1/discordgo.proto:1201](../../proto/discord-v1/discordgo.proto#L1201) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/activity.go:173](../../shared/discord-v1/convert/struct2buf/activity.go#L173)<br>`ChannelMap`, `GuildMap`, `TrackPresences` | [shared/discord-v1/convert/buf2struct/activity.go:187](../../shared/discord-v1/convert/buf2struct/activity.go#L187)<br>`ChannelMap`, `GuildMap`, `Ready`, `TrackPresences` | Go 미사용: `Ready`, `TrackPresences`, `TrackStickers`<br>live runtime 객체를 wire에서 제거합니다. 필요한 도메인 값은 명시적인 snapshot으로 분리합니다. |
| `User`<br>[proto/discord-v1/discordgo.proto:1280](../../proto/discord-v1/discordgo.proto#L1280) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:10](../../shared/discord-v1/convert/struct2buf/structs.go#L10)<br>`AccentColor`, `Banner`, `GlobalName`, `MfaEnabled`, `PremiumType`, `System`, `Token` | [shared/discord-v1/convert/buf2struct/structs.go:9](../../shared/discord-v1/convert/buf2struct/structs.go#L9)<br>`AccentColor`, `Banner`, `GlobalName`, `MfaEnabled`, `PremiumType`, `System`, `Token` | Go 미사용: `AccentColor`, `Banner`, `GlobalName`, `MFAEnabled`, `PremiumType`, `System`, `Token`<br>Token은 모듈 snapshot에서 제거합니다. |
| `TeamMember`<br>[proto/discord-v1/discordgo.proto:1352](../../proto/discord-v1/discordgo.proto#L1352) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:126](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L126) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:126](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L126) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `Team`<br>[proto/discord-v1/discordgo.proto:1360](../../proto/discord-v1/discordgo.proto#L1360) | 비활성 양방향 | [shared/discord-v1/convert/struct2buf/application.go.disabled:105](../../shared/discord-v1/convert/struct2buf/application.go.disabled#L105) (disabled) | [shared/discord-v1/convert/buf2struct/application.go.disabled:105](../../shared/discord-v1/convert/buf2struct/application.go.disabled#L105) (disabled) | 본문의 전체 필드·presence 규칙 적용 |
| `Webhook`<br>[proto/discord-v1/discordgo.proto:1380](../../proto/discord-v1/discordgo.proto#L1380) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/structs.go:471](../../shared/discord-v1/convert/struct2buf/structs.go#L471)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/structs.go:466](../../shared/discord-v1/convert/buf2struct/structs.go#L466)<br>추가 후보 없음 | raw Token을 제거하고 instance-owned CredentialRef와 안전한 handle을 사용합니다. |
| `WebhookParams`<br>[proto/discord-v1/discordgo.proto:1396](../../proto/discord-v1/discordgo.proto#L1396) | 한 방향만 활성 | [shared/discord-v1/convert/struct2buf/interactions.go:442](../../shared/discord-v1/convert/struct2buf/interactions.go#L442)<br>`Components`, `Files`, `Flags` | 전용 함수 없음 | Go 미사용: `Components`, `Files`, `Flags` |
| `WebhookEdit`<br>[proto/discord-v1/discordgo.proto:1434](../../proto/discord-v1/discordgo.proto#L1434) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:427](../../shared/discord-v1/convert/struct2buf/interactions.go#L427)<br>`AllowedMentions`, `Attachments`, `Components`, `Embeds`, `Files` | [shared/discord-v1/convert/buf2struct/interactions.go:424](../../shared/discord-v1/convert/buf2struct/interactions.go#L424)<br>`AllowedMentions`, `Attachments`, `Components`, `Embeds`, `Files` | Go 미사용: `AllowedMentions`, `Attachments`, `Components`, `Embeds`, `Files`<br>patch와 첨부파일은 존재성이 있는 요청 전용 view를 사용합니다. |
| `ComponentEmoji`<br>[proto/discord-v1/discordgo.proto:1453](../../proto/discord-v1/discordgo.proto#L1453) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:190](../../shared/discord-v1/convert/struct2buf/components.go#L190)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:80](../../shared/discord-v1/convert/buf2struct/components.go#L80)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageComponent`<br>[proto/discord-v1/discordgo.proto:1527](../../proto/discord-v1/discordgo.proto#L1527) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:9](../../shared/discord-v1/convert/struct2buf/components.go#L9)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:9](../../shared/discord-v1/convert/buf2struct/components.go#L9)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `SelectMenuOption`<br>[proto/discord-v1/discordgo.proto:1544](../../proto/discord-v1/discordgo.proto#L1544) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:244](../../shared/discord-v1/convert/struct2buf/components.go#L244)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:138](../../shared/discord-v1/convert/buf2struct/components.go#L138)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `SelectMenuDefaultValue`<br>[proto/discord-v1/discordgo.proto:1553](../../proto/discord-v1/discordgo.proto#L1553) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:259](../../shared/discord-v1/convert/struct2buf/components.go#L259)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:153](../../shared/discord-v1/convert/buf2struct/components.go#L153)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `UnfurledMediaItem`<br>[proto/discord-v1/discordgo.proto:1559](../../proto/discord-v1/discordgo.proto#L1559) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:331](../../shared/discord-v1/convert/struct2buf/components.go#L331)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:225](../../shared/discord-v1/convert/buf2struct/components.go#L225)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MediaGalleryItem`<br>[proto/discord-v1/discordgo.proto:1564](../../proto/discord-v1/discordgo.proto#L1564) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:361](../../shared/discord-v1/convert/struct2buf/components.go#L361)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:258](../../shared/discord-v1/convert/buf2struct/components.go#L258)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ActionsRow`<br>[proto/discord-v1/discordgo.proto:1571](../../proto/discord-v1/discordgo.proto#L1571) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:153](../../shared/discord-v1/convert/struct2buf/components.go#L153)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:43](../../shared/discord-v1/convert/buf2struct/components.go#L43)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Button`<br>[proto/discord-v1/discordgo.proto:1577](../../proto/discord-v1/discordgo.proto#L1577) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:172](../../shared/discord-v1/convert/struct2buf/components.go#L172)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:62](../../shared/discord-v1/convert/buf2struct/components.go#L62)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `SelectMenu`<br>[proto/discord-v1/discordgo.proto:1589](../../proto/discord-v1/discordgo.proto#L1589) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:203](../../shared/discord-v1/convert/struct2buf/components.go#L203)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:93](../../shared/discord-v1/convert/buf2struct/components.go#L93)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `TextInput`<br>[proto/discord-v1/discordgo.proto:1603](../../proto/discord-v1/discordgo.proto#L1603) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:281](../../shared/discord-v1/convert/struct2buf/components.go#L281)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:175](../../shared/discord-v1/convert/buf2struct/components.go#L175)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Section`<br>[proto/discord-v1/discordgo.proto:1616](../../proto/discord-v1/discordgo.proto#L1616) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:300](../../shared/discord-v1/convert/struct2buf/components.go#L300)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:194](../../shared/discord-v1/convert/buf2struct/components.go#L194)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `TextDisplay`<br>[proto/discord-v1/discordgo.proto:1623](../../proto/discord-v1/discordgo.proto#L1623) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:320](../../shared/discord-v1/convert/struct2buf/components.go#L320)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:214](../../shared/discord-v1/convert/buf2struct/components.go#L214)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Thumbnail`<br>[proto/discord-v1/discordgo.proto:1628](../../proto/discord-v1/discordgo.proto#L1628) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:342](../../shared/discord-v1/convert/struct2buf/components.go#L342)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:236](../../shared/discord-v1/convert/buf2struct/components.go#L236)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MediaGallery`<br>[proto/discord-v1/discordgo.proto:1636](../../proto/discord-v1/discordgo.proto#L1636) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:379](../../shared/discord-v1/convert/struct2buf/components.go#L379)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:279](../../shared/discord-v1/convert/buf2struct/components.go#L279)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `FileComponent`<br>[proto/discord-v1/discordgo.proto:1642](../../proto/discord-v1/discordgo.proto#L1642) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:396](../../shared/discord-v1/convert/struct2buf/components.go#L396)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:298](../../shared/discord-v1/convert/buf2struct/components.go#L298)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Separator`<br>[proto/discord-v1/discordgo.proto:1649](../../proto/discord-v1/discordgo.proto#L1649) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:409](../../shared/discord-v1/convert/struct2buf/components.go#L409)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:316](../../shared/discord-v1/convert/buf2struct/components.go#L316)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Container`<br>[proto/discord-v1/discordgo.proto:1656](../../proto/discord-v1/discordgo.proto#L1656) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/components.go:431](../../shared/discord-v1/convert/struct2buf/components.go#L431)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/components.go:338](../../shared/discord-v1/convert/buf2struct/components.go#L338)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationCommand`<br>[proto/discord-v1/discordgo.proto:1701](../../proto/discord-v1/discordgo.proto#L1701) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:10](../../shared/discord-v1/convert/struct2buf/interactions.go#L10)<br>`Contexts`, `DefaultPermission`, `DmPermission`, `IntegrationTypes` | [shared/discord-v1/convert/buf2struct/interactions.go:11](../../shared/discord-v1/convert/buf2struct/interactions.go#L11)<br>`Contexts`, `DefaultPermission`, `DmPermission`, `IntegrationTypes` | Go 미사용: `Contexts`, `DMPermission`, `DefaultPermission`, `IntegrationTypes` |
| `ApplicationCommandOption`<br>[proto/discord-v1/discordgo.proto:1745](../../proto/discord-v1/discordgo.proto#L1745) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:71](../../shared/discord-v1/convert/struct2buf/interactions.go#L71)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:53](../../shared/discord-v1/convert/buf2struct/interactions.go#L53)<br>`Autocomplete` | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationCommandOptionChoice`<br>[proto/discord-v1/discordgo.proto:1776](../../proto/discord-v1/discordgo.proto#L1776) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:121](../../shared/discord-v1/convert/struct2buf/interactions.go#L121)<br>`NameLocalizations` | [shared/discord-v1/convert/buf2struct/interactions.go:114](../../shared/discord-v1/convert/buf2struct/interactions.go#L114)<br>`NameLocalizations` | Go 미사용: `NameLocalizations` |
| `ApplicationCommandPermissions`<br>[proto/discord-v1/discordgo.proto:1800](../../proto/discord-v1/discordgo.proto#L1800) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationCommandPermissionsList`<br>[proto/discord-v1/discordgo.proto:1808](../../proto/discord-v1/discordgo.proto#L1808) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `GuildApplicationCommandPermissions`<br>[proto/discord-v1/discordgo.proto:1813](../../proto/discord-v1/discordgo.proto#L1813) | 전용 변환 없음 | 전용 함수 없음 | 전용 함수 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `Interaction`<br>[proto/discord-v1/discordgo.proto:1832](../../proto/discord-v1/discordgo.proto#L1832) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:150](../../shared/discord-v1/convert/struct2buf/interactions.go#L150)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:138](../../shared/discord-v1/convert/buf2struct/interactions.go#L138)<br>`AuthorizingIntegrationOwners`, `Context`, `Entitlements` | Go 미사용: `AuthorizingIntegrationOwners`, `Context`, `Entitlements`<br>raw Token을 제거하고 instance-owned CredentialRef와 안전한 handle을 사용합니다. |
| `ApplicationCommandInteractionData`<br>[proto/discord-v1/discordgo.proto:1890](../../proto/discord-v1/discordgo.proto#L1890) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:205](../../shared/discord-v1/convert/struct2buf/interactions.go#L205)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:185](../../shared/discord-v1/convert/buf2struct/interactions.go#L185)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationCommandInteractionDataResolved`<br>[proto/discord-v1/discordgo.proto:1907](../../proto/discord-v1/discordgo.proto#L1907) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:290](../../shared/discord-v1/convert/struct2buf/interactions.go#L290)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:251](../../shared/discord-v1/convert/buf2struct/interactions.go#L251)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageComponentInteractionData`<br>[proto/discord-v1/discordgo.proto:1917](../../proto/discord-v1/discordgo.proto#L1917) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:335](../../shared/discord-v1/convert/struct2buf/interactions.go#L335)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:296](../../shared/discord-v1/convert/buf2struct/interactions.go#L296)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `MessageComponentInteractionDataResolved`<br>[proto/discord-v1/discordgo.proto:1928](../../proto/discord-v1/discordgo.proto#L1928) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:348](../../shared/discord-v1/convert/struct2buf/interactions.go#L348)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:309](../../shared/discord-v1/convert/buf2struct/interactions.go#L309)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ModalSubmitInteractionData`<br>[proto/discord-v1/discordgo.proto:1936](../../proto/discord-v1/discordgo.proto#L1936) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:381](../../shared/discord-v1/convert/struct2buf/interactions.go#L381)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:342](../../shared/discord-v1/convert/buf2struct/interactions.go#L342)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `ApplicationCommandInteractionDataOption`<br>[proto/discord-v1/discordgo.proto:1942](../../proto/discord-v1/discordgo.proto#L1942) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:225](../../shared/discord-v1/convert/struct2buf/interactions.go#L225)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:205](../../shared/discord-v1/convert/buf2struct/interactions.go#L205)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `InteractionResponse`<br>[proto/discord-v1/discordgo.proto:1988](../../proto/discord-v1/discordgo.proto#L1988) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:400](../../shared/discord-v1/convert/struct2buf/interactions.go#L400)<br>추가 후보 없음 | [shared/discord-v1/convert/buf2struct/interactions.go:361](../../shared/discord-v1/convert/buf2struct/interactions.go#L361)<br>추가 후보 없음 | 본문의 전체 필드·presence 규칙 적용 |
| `InteractionResponseData`<br>[proto/discord-v1/discordgo.proto:1994](../../proto/discord-v1/discordgo.proto#L1994) | 활성 양방향 | [shared/discord-v1/convert/struct2buf/interactions.go:411](../../shared/discord-v1/convert/struct2buf/interactions.go#L411)<br>`Attachments`, `Choices`, `CustomId`, `Files`, `Poll`, `Title` | [shared/discord-v1/convert/buf2struct/interactions.go:372](../../shared/discord-v1/convert/buf2struct/interactions.go#L372)<br>`Choices`, `CustomId`, `Poll`, `Title` | Go 미사용: `Attachments`, `Choices`, `CustomID`, `Files`, `Poll`, `Title`<br>patch와 첨부파일은 존재성이 있는 요청 전용 view를 사용합니다. |

### 부록 B. 기존 감사 API에서 도달한 199개 named type

이 목록은 기존 SDK `v0.29.0`, commit `6e8fa27c7917ea54d8b9ec26f126becae59058d2` 감사의 root 57개, struct 131개, scalar/alias 66개와 interface 2개를 보존한 역사적 closure입니다. 현재 API closure 204개나 API/event 합집합 282개의 상한이 아니며 최신 필드 선언으로 해석하지 않습니다. 이름이 바뀐 resolved 타입과 새 component, pins, UserPrimaryGuild 등은 본문 current-source delta 및 [현행 closure](../../tools/protocol/inventory/source-type-closure.json)를 따릅니다. 원본 `Token`, `Reader`, 세션 내부 정보나 손실 있는 patch 구조를 그대로 wire에 복제하지 않으며 P1과 이 문서의 안전한 view 및 경계 규칙을 적용합니다.

직접 원본 이름으로 대응하는 스키마가 없는 핵심 struct는 `ChannelFollow`, `GuildChannelCreateData`, `UpdateStatusData`입니다. `ChannelEdit`은 기존 helper 스키마에도 선언되어 있으므로 도메인 스키마 154개만 검색하여 미정의로 오분류하지 않습니다.

| 타입 | 원본 kind | 선언 근거 | 원본 필드 또는 alias 표현 |
|---|---|---|---|
| `ActionsRow` | `struct` | [discordgo/components.go:91](../../discordgo/components.go#L91) | `Components []MessageComponent`<br>`ID int` |
| `Activity` | `struct` | [discordgo/structs.go:2223](../../discordgo/structs.go#L2223) | `Name string`<br>`Type ActivityType`<br>`URL string`<br>`CreatedAt time.Time`<br>`ApplicationID string`<br>`State string`<br>`Details string`<br>`Timestamps TimeStamps`<br>`Emoji Emoji`<br>`Party Party`<br>`Assets Assets`<br>`Secrets Secrets`<br>`Instance bool`<br>`Flags int` |
| `ActivityType` | `int` | [discordgo/structs.go:2296](../../discordgo/structs.go#L2296) | `int` |
| `AllowedMentionType` | `string` | [discordgo/message.go:316](../../discordgo/message.go#L316) | `string` |
| `Application` | `struct` | [discordgo/structs.go:162](../../discordgo/structs.go#L162) | `ID string`<br>`Name string`<br>`Icon string`<br>`Description string`<br>`RPCOrigins []string`<br>`BotPublic bool`<br>`BotRequireCodeGrant bool`<br>`TermsOfServiceURL string`<br>`PrivacyProxyURL string`<br>`Owner *User`<br>`Summary string`<br>`VerifyKey string`<br>`Team *Team`<br>`GuildID string`<br>`PrimarySKUID string`<br>`Slug string`<br>`CoverImage string`<br>`Flags int`<br>`IntegrationTypesConfig map[ApplicationIntegrationType]*ApplicationIntegrationTypeConfig` |
| `ApplicationCommand` | `struct` | [discordgo/interactions.go:33](../../discordgo/interactions.go#L33) | `ID string`<br>`ApplicationID string`<br>`GuildID string`<br>`Version string`<br>`Type ApplicationCommandType`<br>`Name string`<br>`NameLocalizations *map[Locale]string`<br>`DefaultPermission *bool`<br>`DefaultMemberPermissions *int64`<br>`NSFW *bool`<br>`DMPermission *bool`<br>`Contexts *[]InteractionContextType`<br>`IntegrationTypes *[]ApplicationIntegrationType`<br>`Description string`<br>`DescriptionLocalizations *map[Locale]string`<br>`Options []*ApplicationCommandOption` |
| `ApplicationCommandInteractionData` | `struct` | [discordgo/interactions.go:340](../../discordgo/interactions.go#L340) | `ID string`<br>`Name string`<br>`CommandType ApplicationCommandType`<br>`Resolved *ApplicationCommandInteractionDataResolved`<br>`Options []*ApplicationCommandInteractionDataOption`<br>`TargetID string` |
| `ApplicationCommandInteractionDataOption` | `struct` | [discordgo/interactions.go:436](../../discordgo/interactions.go#L436) | `Name string`<br>`Type ApplicationCommandOptionType`<br>`Value interface{}`<br>`Options []*ApplicationCommandInteractionDataOption`<br>`Focused bool` |
| `ApplicationCommandInteractionDataResolved` | `struct` | [discordgo/interactions.go:368](../../discordgo/interactions.go#L368) | `Users map[string]*User`<br>`Members map[string]*Member`<br>`Roles map[string]*Role`<br>`Channels map[string]*Channel`<br>`Messages map[string]*Message`<br>`Attachments map[string]*MessageAttachment` |
| `ApplicationCommandOption` | `struct` | [discordgo/interactions.go:106](../../discordgo/interactions.go#L106) | `Type ApplicationCommandOptionType`<br>`Name string`<br>`NameLocalizations map[Locale]string`<br>`Description string`<br>`DescriptionLocalizations map[Locale]string`<br>`ChannelTypes []ChannelType`<br>`Required bool`<br>`Options []*ApplicationCommandOption`<br>`Autocomplete bool`<br>`Choices []*ApplicationCommandOptionChoice`<br>`MinValue *float64`<br>`MaxValue float64`<br>`MinLength *int`<br>`MaxLength int` |
| `ApplicationCommandOptionChoice` | `struct` | [discordgo/interactions.go:134](../../discordgo/interactions.go#L134) | `Name string`<br>`NameLocalizations map[Locale]string`<br>`Value interface{}` |
| `ApplicationCommandOptionType` | `uint8` | [discordgo/interactions.go:60](../../discordgo/interactions.go#L60) | `uint8` |
| `ApplicationCommandPermissionType` | `uint8` | [discordgo/interactions.go:173](../../discordgo/interactions.go#L173) | `uint8` |
| `ApplicationCommandPermissions` | `struct` | [discordgo/interactions.go:141](../../discordgo/interactions.go#L141) | `ID string`<br>`Type ApplicationCommandPermissionType`<br>`Permission bool` |
| `ApplicationCommandType` | `uint8` | [discordgo/interactions.go:20](../../discordgo/interactions.go#L20) | `uint8` |
| `ApplicationInstallParams` | `struct` | [discordgo/structs.go:151](../../discordgo/structs.go#L151) | `Scopes []string`<br>`Permissions int64` |
| `ApplicationIntegrationType` | `uint` | [discordgo/structs.go:140](../../discordgo/structs.go#L140) | `uint` |
| `ApplicationIntegrationTypeConfig` | `struct` | [discordgo/structs.go:157](../../discordgo/structs.go#L157) | `OAuth2InstallParams *ApplicationInstallParams` |
| `ApplicationRoleConnectionMetadata` | `struct` | [discordgo/structs.go:200](../../discordgo/structs.go#L200) | `Type ApplicationRoleConnectionMetadataType`<br>`Key string`<br>`Name string`<br>`NameLocalizations map[Locale]string`<br>`Description string`<br>`DescriptionLocalizations map[Locale]string` |
| `ApplicationRoleConnectionMetadataType` | `int` | [discordgo/structs.go:185](../../discordgo/structs.go#L185) | `int` |
| `Assets` | `struct` | [discordgo/structs.go:1533](../../discordgo/structs.go#L1533) | `LargeImageID string`<br>`SmallImageID string`<br>`LargeText string`<br>`SmallText string` |
| `AuditLogAction` | `int` | [discordgo/structs.go:2026](../../discordgo/structs.go#L2026) | `int` |
| `AuditLogChange` | `struct` | [discordgo/structs.go:1841](../../discordgo/structs.go#L1841) | `NewValue interface{}`<br>`OldValue interface{}`<br>`Key *AuditLogChangeKey` |
| `AuditLogChangeKey` | `string` | [discordgo/structs.go:1849](../../discordgo/structs.go#L1849) | `string` |
| `AuditLogEntry` | `struct` | [discordgo/structs.go:1830](../../discordgo/structs.go#L1830) | `TargetID string`<br>`Changes []*AuditLogChange`<br>`UserID string`<br>`ID string`<br>`ActionType *AuditLogAction`<br>`Options *AuditLogOptions`<br>`Reason string` |
| `AuditLogOptions` | `struct` | [discordgo/structs.go:1999](../../discordgo/structs.go#L1999) | `DeleteMemberDays string`<br>`MembersRemoved string`<br>`ChannelID string`<br>`MessageID string`<br>`Count string`<br>`ID string`<br>`Type *AuditLogOptionsType`<br>`RoleName string`<br>`ApplicationID string`<br>`AutoModerationRuleName string`<br>`AutoModerationRuleTriggerType string`<br>`IntegrationType string` |
| `AuditLogOptionsType` | `string` | [discordgo/structs.go:2016](../../discordgo/structs.go#L2016) | `string` |
| `AutoModerationAction` | `struct` | [discordgo/structs.go:1808](../../discordgo/structs.go#L1808) | `Type AutoModerationActionType`<br>`Metadata *AutoModerationActionMetadata` |
| `AutoModerationActionMetadata` | `struct` | [discordgo/structs.go:1793](../../discordgo/structs.go#L1793) | `ChannelID string`<br>`Duration int`<br>`CustomMessage string` |
| `AutoModerationActionType` | `int` | [discordgo/structs.go:1783](../../discordgo/structs.go#L1783) | `int` |
| `AutoModerationKeywordPreset` | `uint` | [discordgo/structs.go:1751](../../discordgo/structs.go#L1751) | `uint` |
| `AutoModerationRule` | `struct` | [discordgo/structs.go:1716](../../discordgo/structs.go#L1716) | `ID string`<br>`GuildID string`<br>`Name string`<br>`CreatorID string`<br>`EventType AutoModerationRuleEventType`<br>`TriggerType AutoModerationRuleTriggerType`<br>`TriggerMetadata *AutoModerationTriggerMetadata`<br>`Actions []AutoModerationAction`<br>`Enabled *bool`<br>`ExemptRoles *[]string`<br>`ExemptChannels *[]string` |
| `AutoModerationRuleEventType` | `int` | [discordgo/structs.go:1731](../../discordgo/structs.go#L1731) | `int` |
| `AutoModerationRuleTriggerType` | `int` | [discordgo/structs.go:1740](../../discordgo/structs.go#L1740) | `int` |
| `AutoModerationTriggerMetadata` | `struct` | [discordgo/structs.go:1761](../../discordgo/structs.go#L1761) | `KeywordFilter []string`<br>`RegexPatterns []string`<br>`Presets []AutoModerationKeywordPreset`<br>`AllowList *[]string`<br>`MentionTotalLimit int` |
| `Button` | `struct` | [discordgo/components.go:165](../../discordgo/components.go#L165) | `Label string`<br>`Style ButtonStyle`<br>`Disabled bool`<br>`Emoji *ComponentEmoji`<br>`URL string`<br>`CustomID string`<br>`SKUID string`<br>`ID int` |
| `ButtonStyle` | `uint` | [discordgo/components.go:139](../../discordgo/components.go#L139) | `uint` |
| `Channel` | `struct` | [discordgo/structs.go:359](../../discordgo/structs.go#L359) | `ID string`<br>`GuildID string`<br>`Name string`<br>`Topic string`<br>`Type ChannelType`<br>`LastMessageID string`<br>`LastPinTimestamp *time.Time`<br>`MessageCount int`<br>`MemberCount int`<br>`NSFW bool`<br>`Icon string`<br>`Position int`<br>`Bitrate int`<br>`Recipients []*User`<br>`Messages []*Message`<br>`PermissionOverwrites []*PermissionOverwrite`<br>`UserLimit int`<br>`ParentID string`<br>`RateLimitPerUser int`<br>`OwnerID string`<br>`ApplicationID string`<br>`ThreadMetadata *ThreadMetadata`<br>`Member *ThreadMember`<br>`Members []*ThreadMember`<br>`Flags ChannelFlags`<br>`AvailableTags []ForumTag`<br>`AppliedTags []string`<br>`DefaultReactionEmoji ForumDefaultReaction`<br>`DefaultThreadRateLimitPerUser int`<br>`DefaultSortOrder *ForumSortOrderType`<br>`DefaultForumLayout ForumLayout` |
| `ChannelEdit` | `struct` | [discordgo/structs.go:471](../../discordgo/structs.go#L471) | `Name string`<br>`Topic string`<br>`NSFW *bool`<br>`Position *int`<br>`Bitrate int`<br>`UserLimit int`<br>`PermissionOverwrites []*PermissionOverwrite`<br>`ParentID string`<br>`RateLimitPerUser *int`<br>`Flags *ChannelFlags`<br>`DefaultThreadRateLimitPerUser *int`<br>`Archived *bool`<br>`AutoArchiveDuration int`<br>`Locked *bool`<br>`Invitable *bool`<br>`AvailableTags *[]ForumTag`<br>`DefaultReactionEmoji *ForumDefaultReaction`<br>`DefaultSortOrder *ForumSortOrderType`<br>`DefaultForumLayout *ForumLayout`<br>`AppliedTags *[]string` |
| `ChannelFlags` | `int` | [discordgo/structs.go:324](../../discordgo/structs.go#L324) | `int` |
| `ChannelFollow` | `struct` | [discordgo/structs.go:503](../../discordgo/structs.go#L503) | `ChannelID string`<br>`WebhookID string` |
| `ChannelType` | `int` | [discordgo/structs.go:303](../../discordgo/structs.go#L303) | `int` |
| `ClientStatus` | `struct` | [discordgo/structs.go:1650](../../discordgo/structs.go#L1650) | `Desktop Status`<br>`Mobile Status`<br>`Web Status` |
| `ComponentEmoji` | `struct` | [discordgo/components.go:158](../../discordgo/components.go#L158) | `Name string`<br>`ID string`<br>`Animated bool` |
| `ComponentType` | `uint` | [discordgo/components.go:9](../../discordgo/components.go#L9) | `uint` |
| `Container` | `struct` | [discordgo/components.go:532](../../discordgo/components.go#L532) | `ID int`<br>`AccentColor *int`<br>`Spoiler bool`<br>`Components []MessageComponent` |
| `EmbedType` | `string` | [discordgo/message.go:447](../../discordgo/message.go#L447) | `string` |
| `Emoji` | `struct` | [discordgo/structs.go:602](../../discordgo/structs.go#L602) | `ID string`<br>`Name string`<br>`Roles []string`<br>`User *User`<br>`RequireColons bool`<br>`Managed bool`<br>`Animated bool`<br>`Available bool` |
| `EmojiParams` | `struct` | [discordgo/structs.go:643](../../discordgo/structs.go#L643) | `Name string`<br>`Image string`<br>`Roles []string` |
| `Entitlement` | `struct` | [discordgo/structs.go:2535](../../discordgo/structs.go#L2535) | `ID string`<br>`SKUID string`<br>`ApplicationID string`<br>`UserID string`<br>`Type EntitlementType`<br>`Deleted bool`<br>`StartsAt *time.Time`<br>`EndsAt *time.Time`<br>`GuildID string`<br>`Consumed *bool`<br>`SubscriptionID string` |
| `EntitlementFilterOptions` | `struct` | [discordgo/structs.go:2598](../../discordgo/structs.go#L2598) | `UserID string`<br>`SkuIDs []string`<br>`Before *time.Time`<br>`After *time.Time`<br>`Limit int`<br>`GuildID string`<br>`ExcludeEnded bool` |
| `EntitlementOwnerType` | `int` | [discordgo/structs.go:2577](../../discordgo/structs.go#L2577) | `int` |
| `EntitlementTest` | `struct` | [discordgo/structs.go:2586](../../discordgo/structs.go#L2586) | `SKUID string`<br>`OwnerID string`<br>`OwnerType EntitlementOwnerType` |
| `EntitlementType` | `int` | [discordgo/structs.go:2519](../../discordgo/structs.go#L2519) | `int` |
| `ExpireBehavior` | `int` | [discordgo/structs.go:243](../../discordgo/structs.go#L243) | `int` |
| `ExplicitContentFilterLevel` | `int` | [discordgo/structs.go:720](../../discordgo/structs.go#L720) | `int` |
| `File` | `struct` | [discordgo/message.go:238](../../discordgo/message.go#L238) | `Name string`<br>`ContentType string`<br>`Reader io.Reader` |
| `FileComponent` | `struct` | [discordgo/components.go:469](../../discordgo/components.go#L469) | `ID int`<br>`File UnfurledMediaItem`<br>`Spoiler bool` |
| `ForumDefaultReaction` | `struct` | [discordgo/structs.go:585](../../discordgo/structs.go#L585) | `EmojiID string`<br>`EmojiName string` |
| `ForumLayout` | `int` | [discordgo/structs.go:347](../../discordgo/structs.go#L347) | `int` |
| `ForumSortOrderType` | `int` | [discordgo/structs.go:337](../../discordgo/structs.go#L337) | `int` |
| `ForumTag` | `struct` | [discordgo/structs.go:593](../../discordgo/structs.go#L593) | `ID string`<br>`Name string`<br>`Moderated bool`<br>`EmojiID string`<br>`EmojiName string` |
| `GatewayBotResponse` | `struct` | [discordgo/structs.go:2198](../../discordgo/structs.go#L2198) | `URL string`<br>`Shards int`<br>`SessionStartLimit SessionInformation` |
| `Guild` | `struct` | [discordgo/structs.go:762](../../discordgo/structs.go#L762) | `ID string`<br>`Name string`<br>`Icon string`<br>`Region string`<br>`AfkChannelID string`<br>`OwnerID string`<br>`Owner bool`<br>`JoinedAt time.Time`<br>`DiscoverySplash string`<br>`Splash string`<br>`AfkTimeout int`<br>`MemberCount int`<br>`VerificationLevel VerificationLevel`<br>`Large bool`<br>`DefaultMessageNotifications MessageNotifications`<br>`Roles []*Role`<br>`Emojis []*Emoji`<br>`Stickers []*Sticker`<br>`Members []*Member`<br>`Presences []*Presence`<br>`MaxPresences int`<br>`MaxMembers int`<br>`Channels []*Channel`<br>`Threads []*Channel`<br>`VoiceStates []*VoiceState`<br>`Unavailable bool`<br>`ExplicitContentFilter ExplicitContentFilterLevel`<br>`NSFWLevel GuildNSFWLevel`<br>`Features []GuildFeature`<br>`MfaLevel MfaLevel`<br>`ApplicationID string`<br>`WidgetEnabled bool`<br>`WidgetChannelID string`<br>`SystemChannelID string`<br>`SystemChannelFlags SystemChannelFlag`<br>`RulesChannelID string`<br>`VanityURLCode string`<br>`Description string`<br>`Banner string`<br>`PremiumTier PremiumTier`<br>`PremiumSubscriptionCount int`<br>`PreferredLocale string`<br>`PublicUpdatesChannelID string`<br>`MaxVideoChannelUsers int`<br>`ApproximateMemberCount int`<br>`ApproximatePresenceCount int`<br>`Permissions int64`<br>`StageInstances []*StageInstance` |
| `GuildApplicationCommandPermissions` | `struct` | [discordgo/interactions.go:165](../../discordgo/interactions.go#L165) | `ID string`<br>`ApplicationID string`<br>`GuildID string`<br>`Permissions []*ApplicationCommandPermissions` |
| `GuildAuditLog` | `struct` | [discordgo/structs.go:1821](../../discordgo/structs.go#L1821) | `Webhooks []*Webhook`<br>`Users []*User`<br>`AuditLogEntries []*AuditLogEntry`<br>`Integrations []*Integration` |
| `GuildBan` | `struct` | [discordgo/structs.go:1710](../../discordgo/structs.go#L1710) | `Reason string`<br>`User *User` |
| `GuildChannelCreateData` | `struct` | [discordgo/restapi.go:1027](../../discordgo/restapi.go#L1027) | `Name string`<br>`Type ChannelType`<br>`Topic string`<br>`Bitrate int`<br>`UserLimit int`<br>`RateLimitPerUser int`<br>`Position int`<br>`PermissionOverwrites []*PermissionOverwrite`<br>`ParentID string`<br>`NSFW bool` |
| `GuildEmbed` | `struct` | [discordgo/structs.go:1814](../../discordgo/structs.go#L1814) | `Enabled *bool`<br>`ChannelID string` |
| `GuildFeature` | `string` | [discordgo/structs.go:1318](../../discordgo/structs.go#L1318) | `string` |
| `GuildMemberAddParams` | `struct` | [discordgo/structs.go:2169](../../discordgo/structs.go#L2169) | `AccessToken string`<br>`Nick string`<br>`Roles []string`<br>`Mute bool`<br>`Deaf bool` |
| `GuildMemberParams` | `struct` | [discordgo/structs.go:2113](../../discordgo/structs.go#L2113) | `Nick string`<br>`Roles *[]string`<br>`ChannelID *string`<br>`Mute *bool`<br>`Deaf *bool`<br>`CommunicationDisabledUntil *time.Time` |
| `GuildNSFWLevel` | `int` | [discordgo/structs.go:730](../../discordgo/structs.go#L730) | `int` |
| `GuildOnboarding` | `struct` | [discordgo/structs.go:1128](../../discordgo/structs.go#L1128) | `GuildID string`<br>`Prompts *[]GuildOnboardingPrompt`<br>`DefaultChannelIDs []string`<br>`Enabled *bool`<br>`Mode *GuildOnboardingMode` |
| `GuildOnboardingMode` | `int` | [discordgo/structs.go:1116](../../discordgo/structs.go#L1116) | `int` |
| `GuildOnboardingPrompt` | `struct` | [discordgo/structs.go:1157](../../discordgo/structs.go#L1157) | `ID string`<br>`Type GuildOnboardingPromptType`<br>`Options []GuildOnboardingPromptOption`<br>`Title string`<br>`SingleSelect bool`<br>`Required bool`<br>`InOnboarding bool` |
| `GuildOnboardingPromptOption` | `struct` | [discordgo/structs.go:1185](../../discordgo/structs.go#L1185) | `ID string`<br>`ChannelIDs []string`<br>`RoleIDs []string`<br>`Emoji *Emoji`<br>`Title string`<br>`Description string`<br>`EmojiID string`<br>`EmojiName string`<br>`EmojiAnimated *bool` |
| `GuildOnboardingPromptType` | `int` | [discordgo/structs.go:1147](../../discordgo/structs.go#L1147) | `int` |
| `GuildParams` | `struct` | [discordgo/structs.go:1354](../../discordgo/structs.go#L1354) | `Name string`<br>`Region string`<br>`VerificationLevel *VerificationLevel`<br>`DefaultMessageNotifications int`<br>`ExplicitContentFilter int`<br>`AfkChannelID string`<br>`AfkTimeout int`<br>`Icon string`<br>`OwnerID string`<br>`Splash string`<br>`DiscoverySplash string`<br>`Banner string`<br>`SystemChannelID string`<br>`SystemChannelFlags SystemChannelFlag`<br>`RulesChannelID string`<br>`PublicUpdatesChannelID string`<br>`PreferredLocale Locale`<br>`Features []GuildFeature`<br>`Description string`<br>`PremiumProgressBarEnabled *bool` |
| `GuildPreview` | `struct` | [discordgo/structs.go:928](../../discordgo/structs.go#L928) | `ID string`<br>`Name string`<br>`Icon string`<br>`Splash string`<br>`DiscoverySplash string`<br>`Emojis []*Emoji`<br>`Features []string`<br>`ApproximateMemberCount int`<br>`ApproximatePresenceCount int`<br>`Description string` |
| `GuildScheduledEvent` | `struct` | [discordgo/structs.go:973](../../discordgo/structs.go#L973) | `ID string`<br>`GuildID string`<br>`ChannelID string`<br>`CreatorID string`<br>`Name string`<br>`Description string`<br>`ScheduledStartTime time.Time`<br>`ScheduledEndTime *time.Time`<br>`PrivacyLevel GuildScheduledEventPrivacyLevel`<br>`Status GuildScheduledEventStatus`<br>`EntityType GuildScheduledEventEntityType`<br>`EntityID string`<br>`EntityMetadata GuildScheduledEventEntityMetadata`<br>`Creator *User`<br>`UserCount int`<br>`Image string` |
| `GuildScheduledEventEntityMetadata` | `struct` | [discordgo/structs.go:1059](../../discordgo/structs.go#L1059) | `Location string` |
| `GuildScheduledEventEntityType` | `int` | [discordgo/structs.go:1095](../../discordgo/structs.go#L1095) | `int` |
| `GuildScheduledEventParams` | `struct` | [discordgo/structs.go:1014](../../discordgo/structs.go#L1014) | `ChannelID string`<br>`Name string`<br>`Description string`<br>`ScheduledStartTime *time.Time`<br>`ScheduledEndTime *time.Time`<br>`PrivacyLevel GuildScheduledEventPrivacyLevel`<br>`Status GuildScheduledEventStatus`<br>`EntityType GuildScheduledEventEntityType`<br>`EntityMetadata *GuildScheduledEventEntityMetadata`<br>`Image string` |
| `GuildScheduledEventPrivacyLevel` | `int` | [discordgo/structs.go:1067](../../discordgo/structs.go#L1067) | `int` |
| `GuildScheduledEventStatus` | `int` | [discordgo/structs.go:1080](../../discordgo/structs.go#L1080) | `int` |
| `GuildScheduledEventUser` | `struct` | [discordgo/structs.go:1108](../../discordgo/structs.go#L1108) | `GuildScheduledEventID string`<br>`User *User`<br>`Member *Member` |
| `GuildTemplate` | `struct` | [discordgo/structs.go:1218](../../discordgo/structs.go#L1218) | `Code string`<br>`Name string`<br>`Description *string`<br>`UsageCount int`<br>`CreatorID string`<br>`Creator *User`<br>`CreatedAt time.Time`<br>`UpdatedAt time.Time`<br>`SourceGuildID string`<br>`SerializedSourceGuild *Guild`<br>`IsDirty bool` |
| `GuildTemplateParams` | `struct` | [discordgo/structs.go:1254](../../discordgo/structs.go#L1254) | `Name string`<br>`Description string` |
| `Integration` | `struct` | [discordgo/structs.go:226](../../discordgo/structs.go#L226) | `ID string`<br>`Name string`<br>`Type string`<br>`Enabled bool`<br>`Syncing bool`<br>`RoleID string`<br>`EnableEmoticons bool`<br>`ExpireBehavior ExpireBehavior`<br>`ExpireGracePeriod int`<br>`User *User`<br>`Account IntegrationAccount`<br>`SyncedAt time.Time` |
| `IntegrationAccount` | `struct` | [discordgo/structs.go:253](../../discordgo/structs.go#L253) | `ID string`<br>`Name string` |
| `Interaction` | `struct` | [discordgo/interactions.go:221](../../discordgo/interactions.go#L221) | `ID string`<br>`AppID string`<br>`Type InteractionType`<br>`Data InteractionData`<br>`GuildID string`<br>`ChannelID string`<br>`Message *Message`<br>`AppPermissions int64`<br>`Member *Member`<br>`User *User`<br>`Locale Locale`<br>`GuildLocale *Locale`<br>`Context InteractionContextType`<br>`AuthorizingIntegrationOwners map[ApplicationIntegrationType]string`<br>`Token string`<br>`Version int`<br>`Entitlements []*Entitlement` |
| `InteractionContextType` | `uint` | [discordgo/interactions.go:209](../../discordgo/interactions.go#L209) | `uint` |
| `InteractionData` | `interface{}` | [discordgo/interactions.go:335](../../discordgo/interactions.go#L335) | `interface{}`; `ApplicationCommandInteractionData`, `MessageComponentInteractionData`, `ModalSubmitInteractionData` |
| `InteractionResponse` | `struct` | [discordgo/interactions.go:592](../../discordgo/interactions.go#L592) | `Type InteractionResponseType`<br>`Data *InteractionResponseData` |
| `InteractionResponseData` | `struct` | [discordgo/interactions.go:598](../../discordgo/interactions.go#L598) | `TTS bool`<br>`Content string`<br>`Components []MessageComponent`<br>`Embeds []*MessageEmbed`<br>`AllowedMentions *MessageAllowedMentions`<br>`Files []*File`<br>`Attachments *[]*MessageAttachment`<br>`Poll *Poll`<br>`Flags MessageFlags`<br>`Choices []*ApplicationCommandOptionChoice`<br>`CustomID string`<br>`Title string` |
| `InteractionResponseType` | `uint8` | [discordgo/interactions.go:571](../../discordgo/interactions.go#L571) | `uint8` |
| `InteractionType` | `uint8` | [discordgo/interactions.go:183](../../discordgo/interactions.go#L183) | `uint8` |
| `Invite` | `struct` | [discordgo/structs.go:279](../../discordgo/structs.go#L279) | `Guild *Guild`<br>`Channel *Channel`<br>`Inviter *User`<br>`Code string`<br>`CreatedAt time.Time`<br>`MaxAge int`<br>`Uses int`<br>`MaxUses int`<br>`Revoked bool`<br>`Temporary bool`<br>`Unique bool`<br>`TargetUser *User`<br>`TargetType InviteTargetType`<br>`TargetApplication *Application`<br>`ApproximatePresenceCount int`<br>`ApproximateMemberCount int`<br>`ExpiresAt *time.Time` |
| `InviteTargetType` | `uint8` | [discordgo/structs.go:270](../../discordgo/structs.go#L270) | `uint8` |
| `Locale` | `string` | [discordgo/locales.go:5](../../discordgo/locales.go#L5) | `string` |
| `MediaGallery` | `struct` | [discordgo/components.go:436](../../discordgo/components.go#L436) | `ID int`<br>`Items []MediaGalleryItem` |
| `MediaGalleryItem` | `struct` | [discordgo/components.go:462](../../discordgo/components.go#L462) | `Media UnfurledMediaItem`<br>`Description *string`<br>`Spoiler bool` |
| `Member` | `struct` | [discordgo/structs.go:1558](../../discordgo/structs.go#L1558) | `GuildID string`<br>`JoinedAt time.Time`<br>`Nick string`<br>`Deaf bool`<br>`Mute bool`<br>`Avatar string`<br>`Banner string`<br>`User *User`<br>`Roles []string`<br>`PremiumSince *time.Time`<br>`Flags MemberFlags`<br>`Pending bool`<br>`Permissions int64`<br>`CommunicationDisabledUntil *time.Time` |
| `MemberFlags` | `int` | [discordgo/structs.go:1542](../../discordgo/structs.go#L1542) | `int` |
| `MembershipState` | `int` | [discordgo/oauth2.go:17](../../discordgo/oauth2.go#L17) | `int` |
| `Message` | `struct` | [discordgo/message.go:49](../../discordgo/message.go#L49) | `ID string`<br>`ChannelID string`<br>`GuildID string`<br>`Content string`<br>`Timestamp time.Time`<br>`EditedTimestamp *time.Time`<br>`MentionRoles []string`<br>`TTS bool`<br>`MentionEveryone bool`<br>`Author *User`<br>`Attachments []*MessageAttachment`<br>`Components []MessageComponent`<br>`Embeds []*MessageEmbed`<br>`Mentions []*User`<br>`Reactions []*MessageReactions`<br>`Pinned bool`<br>`Type MessageType`<br>`WebhookID string`<br>`Member *Member`<br>`MentionChannels []*Channel`<br>`Activity *MessageActivity`<br>`Application *MessageApplication`<br>`MessageReference *MessageReference`<br>`ReferencedMessage *Message`<br>`MessageSnapshots []MessageSnapshot`<br>`Interaction *MessageInteraction`<br>`InteractionMetadata *MessageInteractionMetadata`<br>`Flags MessageFlags`<br>`Thread *Channel`<br>`StickerItems []*StickerItem`<br>`Poll *Poll` |
| `MessageActivity` | `struct` | [discordgo/message.go:467](../../discordgo/message.go#L467) | `Type MessageActivityType`<br>`PartyID string` |
| `MessageActivityType` | `int` | [discordgo/message.go:473](../../discordgo/message.go#L473) | `int` |
| `MessageAllowedMentions` | `struct` | [discordgo/message.go:334](../../discordgo/message.go#L334) | `Parse []AllowedMentionType`<br>`Roles []string`<br>`Users []string`<br>`RepliedUser bool` |
| `MessageApplication` | `struct` | [discordgo/message.go:484](../../discordgo/message.go#L484) | `ID string`<br>`CoverImage string`<br>`Description string`<br>`Icon string`<br>`Name string` |
| `MessageAttachment` | `struct` | [discordgo/message.go:354](../../discordgo/message.go#L354) | `ID string`<br>`URL string`<br>`ProxyURL string`<br>`Filename string`<br>`ContentType string`<br>`Width int`<br>`Height int`<br>`Size int`<br>`Ephemeral bool`<br>`DurationSecs float64`<br>`Waveform string`<br>`Flags MessageAttachmentFlags` |
| `MessageAttachmentFlags` | `int` | [discordgo/message.go:370](../../discordgo/message.go#L370) | `int` |
| `MessageComponent` | `interface{}` | [discordgo/components.go:31](../../discordgo/components.go#L31) | `interface{}`; `ActionsRow`, `Button`, `Container`, `FileComponent`, `MediaGallery`, `Section`, `SelectMenu`, `Separator`, `TextDisplay`, `TextInput`, `Thumbnail` |
| `MessageComponentInteractionData` | `struct` | [discordgo/interactions.go:383](../../discordgo/interactions.go#L383) | `CustomID string`<br>`ComponentType ComponentType`<br>`Resolved MessageComponentInteractionDataResolved`<br>`Values []string` |
| `MessageComponentInteractionDataResolved` | `struct` | [discordgo/interactions.go:393](../../discordgo/interactions.go#L393) | `Users map[string]*User`<br>`Members map[string]*Member`<br>`Roles map[string]*Role`<br>`Channels map[string]*Channel` |
| `MessageEdit` | `struct` | [discordgo/message.go:266](../../discordgo/message.go#L266) | `Content *string`<br>`Components *[]MessageComponent`<br>`Embeds *[]*MessageEmbed`<br>`AllowedMentions *MessageAllowedMentions`<br>`Flags MessageFlags`<br>`Files []*File`<br>`Attachments *[]*MessageAttachment`<br>`ID string`<br>`Channel string`<br>`Embed *MessageEmbed` |
| `MessageEmbed` | `struct` | [discordgo/message.go:429](../../discordgo/message.go#L429) | `URL string`<br>`Type EmbedType`<br>`Title string`<br>`Description string`<br>`Timestamp string`<br>`Color int`<br>`Footer *MessageEmbedFooter`<br>`Image *MessageEmbedImage`<br>`Thumbnail *MessageEmbedThumbnail`<br>`Video *MessageEmbedVideo`<br>`Provider *MessageEmbedProvider`<br>`Author *MessageEmbedAuthor`<br>`Fields []*MessageEmbedField` |
| `MessageEmbedAuthor` | `struct` | [discordgo/message.go:414](../../discordgo/message.go#L414) | `URL string`<br>`Name string`<br>`IconURL string`<br>`ProxyIconURL string` |
| `MessageEmbedField` | `struct` | [discordgo/message.go:422](../../discordgo/message.go#L422) | `Name string`<br>`Value string`<br>`Inline bool` |
| `MessageEmbedFooter` | `struct` | [discordgo/message.go:378](../../discordgo/message.go#L378) | `Text string`<br>`IconURL string`<br>`ProxyIconURL string` |
| `MessageEmbedImage` | `struct` | [discordgo/message.go:385](../../discordgo/message.go#L385) | `URL string`<br>`ProxyURL string`<br>`Width int`<br>`Height int` |
| `MessageEmbedProvider` | `struct` | [discordgo/message.go:408](../../discordgo/message.go#L408) | `URL string`<br>`Name string` |
| `MessageEmbedThumbnail` | `struct` | [discordgo/message.go:393](../../discordgo/message.go#L393) | `URL string`<br>`ProxyURL string`<br>`Width int`<br>`Height int` |
| `MessageEmbedVideo` | `struct` | [discordgo/message.go:401](../../discordgo/message.go#L401) | `URL string`<br>`Width int`<br>`Height int` |
| `MessageFlags` | `int` | [discordgo/message.go:205](../../discordgo/message.go#L205) | `int` |
| `MessageInteraction` | `struct` | [discordgo/message.go:609](../../discordgo/message.go#L609) | `ID string`<br>`Type InteractionType`<br>`Name string`<br>`User *User`<br>`Member *Member` |
| `MessageInteractionMetadata` | `struct` | [discordgo/message.go:620](../../discordgo/message.go#L620) | `ID string`<br>`Type InteractionType`<br>`User *User`<br>`AuthorizingIntegrationOwners map[ApplicationIntegrationType]string`<br>`OriginalResponseMessageID string`<br>`InteractedMessageID string`<br>`TriggeringInteractionMetadata *MessageInteractionMetadata` |
| `MessageNotifications` | `int` | [discordgo/structs.go:1263](../../discordgo/structs.go#L1263) | `int` |
| `MessageReactions` | `struct` | [discordgo/message.go:460](../../discordgo/message.go#L460) | `Count int`<br>`Me bool`<br>`Emoji *Emoji` |
| `MessageReference` | `struct` | [discordgo/message.go:509](../../discordgo/message.go#L509) | `Type MessageReferenceType`<br>`MessageID string`<br>`ChannelID string`<br>`GuildID string`<br>`FailIfNotExists *bool` |
| `MessageReferenceType` | `int` | [discordgo/message.go:499](../../discordgo/message.go#L499) | `int` |
| `MessageSend` | `struct` | [discordgo/message.go:245](../../discordgo/message.go#L245) | `Content string`<br>`Embeds []*MessageEmbed`<br>`TTS bool`<br>`Components []MessageComponent`<br>`Files []*File`<br>`AllowedMentions *MessageAllowedMentions`<br>`Reference *MessageReference`<br>`StickerIDs []string`<br>`Flags MessageFlags`<br>`Poll *Poll`<br>`File *File`<br>`Embed *MessageEmbed` |
| `MessageSnapshot` | `struct` | [discordgo/message.go:494](../../discordgo/message.go#L494) | `Message *Message` |
| `MessageType` | `int` | [discordgo/message.go:22](../../discordgo/message.go#L22) | `int` |
| `MfaLevel` | `int` | [discordgo/structs.go:741](../../discordgo/structs.go#L741) | `int` |
| `ModalSubmitInteractionData` | `struct` | [discordgo/interactions.go:406](../../discordgo/interactions.go#L406) | `CustomID string`<br>`Components []MessageComponent` |
| `Party` | `struct` | [discordgo/structs.go:2281](../../discordgo/structs.go#L2281) | `ID string`<br>`Size []int` |
| `PermissionOverwrite` | `struct` | [discordgo/structs.go:519](../../discordgo/structs.go#L519) | `ID string`<br>`Type PermissionOverwriteType`<br>`Deny int64`<br>`Allow int64` |
| `PermissionOverwriteType` | `int` | [discordgo/structs.go:510](../../discordgo/structs.go#L510) | `int` |
| `Poll` | `struct` | [discordgo/structs.go:2408](../../discordgo/structs.go#L2408) | `Question PollMedia`<br>`Answers []PollAnswer`<br>`AllowMultiselect bool`<br>`LayoutType PollLayoutType`<br>`Duration int`<br>`Results *PollResults`<br>`Expiry *time.Time` |
| `PollAnswer` | `struct` | [discordgo/structs.go:2388](../../discordgo/structs.go#L2388) | `AnswerID int`<br>`Media *PollMedia` |
| `PollAnswerCount` | `struct` | [discordgo/structs.go:2395](../../discordgo/structs.go#L2395) | `ID int`<br>`Count int`<br>`MeVoted bool` |
| `PollLayoutType` | `int` | [discordgo/structs.go:2374](../../discordgo/structs.go#L2374) | `int` |
| `PollMedia` | `struct` | [discordgo/structs.go:2382](../../discordgo/structs.go#L2382) | `Text string`<br>`Emoji *ComponentEmoji` |
| `PollResults` | `struct` | [discordgo/structs.go:2402](../../discordgo/structs.go#L2402) | `Finalized bool`<br>`AnswerCounts []*PollAnswerCount` |
| `PremiumTier` | `int` | [discordgo/structs.go:750](../../discordgo/structs.go#L750) | `int` |
| `Presence` | `struct` | [discordgo/structs.go:1503](../../discordgo/structs.go#L1503) | `User *User`<br>`Status Status`<br>`Activities []*Activity`<br>`Since *int`<br>`ClientStatus ClientStatus` |
| `Role` | `struct` | [discordgo/structs.go:1378](../../discordgo/structs.go#L1378) | `ID string`<br>`Name string`<br>`Managed bool`<br>`Mentionable bool`<br>`Hoist bool`<br>`Color int`<br>`Position int`<br>`Permissions int64`<br>`Icon string`<br>`UnicodeEmoji string`<br>`Flags RoleFlags` |
| `RoleFlags` | `int` | [discordgo/structs.go:1420](../../discordgo/structs.go#L1420) | `int` |
| `RoleParams` | `struct` | [discordgo/structs.go:1451](../../discordgo/structs.go#L1451) | `Name string`<br>`Color *int`<br>`Hoist *bool`<br>`Permissions *int64`<br>`Mentionable *bool`<br>`UnicodeEmoji *string`<br>`Icon *string` |
| `SKU` | `struct` | [discordgo/structs.go:2451](../../discordgo/structs.go#L2451) | `ID string`<br>`Type SKUType`<br>`ApplicationID string`<br>`Name string`<br>`Slug string`<br>`Flags SKUFlags` |
| `SKUFlags` | `int` | [discordgo/structs.go:2439](../../discordgo/structs.go#L2439) | `int` |
| `SKUType` | `int` | [discordgo/structs.go:2426](../../discordgo/structs.go#L2426) | `int` |
| `Secrets` | `struct` | [discordgo/structs.go:2288](../../discordgo/structs.go#L2288) | `Join string`<br>`Spectate string`<br>`Match string` |
| `Section` | `struct` | [discordgo/components.go:333](../../discordgo/components.go#L333) | `ID int`<br>`Components []MessageComponent`<br>`Accessory MessageComponent` |
| `SelectMenu` | `struct` | [discordgo/components.go:243](../../discordgo/components.go#L243) | `MenuType SelectMenuType`<br>`CustomID string`<br>`Placeholder string`<br>`MinValues *int`<br>`MaxValues int`<br>`DefaultValues []SelectMenuDefaultValue`<br>`Options []SelectMenuOption`<br>`Disabled bool`<br>`ChannelTypes []ChannelType`<br>`ID int` |
| `SelectMenuDefaultValue` | `struct` | [discordgo/components.go:223](../../discordgo/components.go#L223) | `ID string`<br>`Type SelectMenuDefaultValueType` |
| `SelectMenuDefaultValueType` | `string` | [discordgo/components.go:213](../../discordgo/components.go#L213) | `string` |
| `SelectMenuOption` | `struct` | [discordgo/components.go:203](../../discordgo/components.go#L203) | `Label string`<br>`Value string`<br>`Description string`<br>`Emoji *ComponentEmoji`<br>`Default bool` |
| `SelectMenuType` | `ComponentType` | [discordgo/components.go:231](../../discordgo/components.go#L231) | `ComponentType`; `ComponentType` |
| `Separator` | `struct` | [discordgo/components.go:504](../../discordgo/components.go#L504) | `ID int`<br>`Divider *bool`<br>`Spacing *SeparatorSpacingSize` |
| `SeparatorSpacingSize` | `uint` | [discordgo/components.go:495](../../discordgo/components.go#L495) | `uint` |
| `SessionInformation` | `struct` | [discordgo/structs.go:2205](../../discordgo/structs.go#L2205) | `Total int`<br>`Remaining int`<br>`ResetAfter int`<br>`MaxConcurrency int` |
| `StageInstance` | `struct` | [discordgo/structs.go:2332](../../discordgo/structs.go#L2332) | `ID string`<br>`GuildID string`<br>`ChannelID string`<br>`Topic string`<br>`PrivacyLevel StageInstancePrivacyLevel`<br>`DiscoverableDisabled bool`<br>`GuildScheduledEventID string` |
| `StageInstanceParams` | `struct` | [discordgo/structs.go:2351](../../discordgo/structs.go#L2351) | `ChannelID string`<br>`Topic string`<br>`PrivacyLevel StageInstancePrivacyLevel`<br>`SendStartNotification bool` |
| `StageInstancePrivacyLevel` | `int` | [discordgo/structs.go:2364](../../discordgo/structs.go#L2364) | `int` |
| `Status` | `string` | [discordgo/structs.go:1657](../../discordgo/structs.go#L1657) | `string` |
| `Sticker` | `struct` | [discordgo/structs.go:675](../../discordgo/structs.go#L675) | `ID string`<br>`PackID string`<br>`Name string`<br>`Description string`<br>`Tags string`<br>`Type StickerType`<br>`FormatType StickerFormat`<br>`Available bool`<br>`GuildID string`<br>`User *User`<br>`SortValue int` |
| `StickerFormat` | `int` | [discordgo/structs.go:655](../../discordgo/structs.go#L655) | `int` |
| `StickerItem` | `struct` | [discordgo/structs.go:690](../../discordgo/structs.go#L690) | `ID string`<br>`Name string`<br>`FormatType StickerFormat` |
| `StickerType` | `int` | [discordgo/structs.go:666](../../discordgo/structs.go#L666) | `int` |
| `Subscription` | `struct` | [discordgo/structs.go:2474](../../discordgo/structs.go#L2474) | `ID string`<br>`UserID string`<br>`SKUIDs []string`<br>`EntitlementIDs []string`<br>`RenewalSKUIDs []string`<br>`CurrentPeriodStart time.Time`<br>`CurrentPeriodEnd time.Time`<br>`Status SubscriptionStatus`<br>`CanceledAt *time.Time`<br>`Country string` |
| `SubscriptionStatus` | `int` | [discordgo/structs.go:2508](../../discordgo/structs.go#L2508) | `int` |
| `SystemChannelFlag` | `int` | [discordgo/structs.go:1273](../../discordgo/structs.go#L1273) | `int` |
| `Team` | `struct` | [discordgo/oauth2.go:34](../../discordgo/oauth2.go#L34) | `ID string`<br>`Name string`<br>`Description string`<br>`Icon string`<br>`OwnerID string`<br>`Members []*TeamMember` |
| `TeamMember` | `struct` | [discordgo/oauth2.go:26](../../discordgo/oauth2.go#L26) | `User *User`<br>`TeamID string`<br>`MembershipState MembershipState`<br>`Permissions []string` |
| `TextDisplay` | `struct` | [discordgo/components.go:386](../../discordgo/components.go#L386) | `Content string` |
| `TextInput` | `struct` | [discordgo/components.go:291](../../discordgo/components.go#L291) | `CustomID string`<br>`Label string`<br>`Style TextInputStyle`<br>`Placeholder string`<br>`Value string`<br>`Required bool`<br>`MinLength int`<br>`MaxLength int`<br>`ID int` |
| `TextInputStyle` | `uint` | [discordgo/components.go:324](../../discordgo/components.go#L324) | `uint` |
| `ThreadMember` | `struct` | [discordgo/structs.go:554](../../discordgo/structs.go#L554) | `ID string`<br>`UserID string`<br>`JoinTimestamp time.Time`<br>`Flags int`<br>`Member *Member` |
| `ThreadMetadata` | `struct` | [discordgo/structs.go:539](../../discordgo/structs.go#L539) | `Archived bool`<br>`AutoArchiveDuration int`<br>`ArchiveTimestamp time.Time`<br>`Locked bool`<br>`Invitable bool` |
| `ThreadStart` | `struct` | [discordgo/structs.go:527](../../discordgo/structs.go#L527) | `Name string`<br>`AutoArchiveDuration int`<br>`Type ChannelType`<br>`Invitable bool`<br>`RateLimitPerUser int`<br>`AppliedTags []string` |
| `ThreadsList` | `struct` | [discordgo/structs.go:570](../../discordgo/structs.go#L570) | `Threads []*Channel`<br>`Members []*ThreadMember`<br>`HasMore bool` |
| `Thumbnail` | `struct` | [discordgo/components.go:409](../../discordgo/components.go#L409) | `ID int`<br>`Media UnfurledMediaItem`<br>`Description *string`<br>`Spoiler bool` |
| `TimeStamps` | `struct` | [discordgo/structs.go:1512](../../discordgo/structs.go#L1512) | `EndTimestamp int64`<br>`StartTimestamp int64` |
| `UnfurledMediaItem` | `struct` | [discordgo/components.go:582](../../discordgo/components.go#L582) | `URL string` |
| `UpdateStatusData` | `struct` | [discordgo/wsapi.go:324](../../discordgo/wsapi.go#L324) | `IdleSince *int`<br>`Activities []*Activity`<br>`AFK bool`<br>`Status string` |
| `User` | `struct` | [discordgo/user.go:44](../../discordgo/user.go#L44) | `ID string`<br>`Email string`<br>`Username string`<br>`Avatar string`<br>`Locale string`<br>`Discriminator string`<br>`GlobalName string`<br>`Token string`<br>`Verified bool`<br>`MFAEnabled bool`<br>`Banner string`<br>`AccentColor int`<br>`Bot bool`<br>`PublicFlags UserFlags`<br>`PremiumType UserPremiumType`<br>`System bool`<br>`Flags int` |
| `UserFlags` | `int` | [discordgo/user.go:9](../../discordgo/user.go#L9) | `int` |
| `UserGuild` | `struct` | [discordgo/structs.go:1300](../../discordgo/structs.go#L1300) | `ID string`<br>`Name string`<br>`Icon string`<br>`Owner bool`<br>`Permissions int64`<br>`Features []GuildFeature`<br>`ApproximateMemberCount int`<br>`ApproximatePresenceCount int` |
| `UserPremiumType` | `int` | [discordgo/user.go:33](../../discordgo/user.go#L33) | `int` |
| `VerificationLevel` | `int` | [discordgo/structs.go:708](../../discordgo/structs.go#L708) | `int` |
| `VoiceRegion` | `struct` | [discordgo/structs.go:260](../../discordgo/structs.go#L260) | `ID string`<br>`Name string`<br>`Optimal bool`<br>`Deprecated bool`<br>`Custom bool` |
| `VoiceState` | `struct` | [discordgo/structs.go:1486](../../discordgo/structs.go#L1486) | `GuildID string`<br>`ChannelID string`<br>`UserID string`<br>`Member *Member`<br>`SessionID string`<br>`Deaf bool`<br>`Mute bool`<br>`SelfDeaf bool`<br>`SelfMute bool`<br>`SelfStream bool`<br>`SelfVideo bool`<br>`Suppress bool`<br>`RequestToSpeakTimestamp *time.Time` |
| `Webhook` | `struct` | [discordgo/webhook.go:4](../../discordgo/webhook.go#L4) | `ID string`<br>`Type WebhookType`<br>`GuildID string`<br>`ChannelID string`<br>`User *User`<br>`Name string`<br>`Avatar string`<br>`Token string`<br>`ApplicationID string` |
| `WebhookEdit` | `struct` | [discordgo/webhook.go:48](../../discordgo/webhook.go#L48) | `Content *string`<br>`Components *[]MessageComponent`<br>`Embeds *[]*MessageEmbed`<br>`Files []*File`<br>`Attachments *[]*MessageAttachment`<br>`AllowedMentions *MessageAllowedMentions` |
| `WebhookParams` | `struct` | [discordgo/webhook.go:29](../../discordgo/webhook.go#L29) | `Content string`<br>`Username string`<br>`AvatarURL string`<br>`TTS bool`<br>`Files []*File`<br>`Components []MessageComponent`<br>`Embeds []*MessageEmbed`<br>`Attachments []*MessageAttachment`<br>`AllowedMentions *MessageAllowedMentions`<br>`Flags MessageFlags`<br>`ThreadName string` |
| `WebhookType` | `int` | [discordgo/webhook.go:20](../../discordgo/webhook.go#L20) | `int` |

interface를 빈 placeholder로 끝내지 않습니다. concrete 집합은 다음과 같습니다.

- `MessageComponent`: `ActionsRow`, `Button`, `SelectMenu`, `TextInput`, `Section`, `TextDisplay`, `Thumbnail`, `MediaGallery`, `FileComponent`, `Separator`, `Container`.
- `InteractionData`: `ApplicationCommandInteractionData`, `MessageComponentInteractionData`, `ModalSubmitInteractionData`.

외부 타입 `time.Time`은 timestamp 또는 nullable timestamp로, `io.Reader`는 완료된 업로드 참조로만 전환합니다. `Session` receiver, `RequestOption` 함수, HTTP client, Gateway socket과 `VoiceConnection`은 API DTO root가 아닙니다. 외부 raw JSON과 credential은 본문의 projection 경계를 통과해야 합니다.
