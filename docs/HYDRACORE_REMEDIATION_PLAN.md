# HydraCore: remediation-runbook после неудачной реализации

Статус: готов к исполнению coding-агентом

Baseline проверки: 2026-08-20

Репозитории: `hydracore`, `HYDRA-ULTIMATE`, `HydraBox`

Режим проверки: только удалённый CI; локальные тесты и сборки запрещены

## 1. Назначение и иерархия требований

Этот документ не заменяет:

- `docs/HYDRACORE_DEOVERENGINEERING_PLAN.md`;
- `docs/VK_PARASITE_4X4_PLAN.md`.

Они остаются источниками требований и архитектурных решений. Этот runbook
добавляет то, чего не хватало после первой попытки реализации:

- фактические дефекты текущего незакоммиченного diff;
- найденных production consumers в HydraBox;
- безопасный межрепозиторный порядок изменений;
- разделение server-first и client-second релизов;
- release, rollback и observation gates;
- точные критерии, по которым coding-агент может объявить фазу закрытой.

При противоречии агент не выбирает удобный вариант сам. Он останавливает
затронутый пакет работ, показывает конфликт владельцу и сохраняет более строгий
security/compatibility invariant. Нельзя использовать текущий частично
реализованный diff как новую спецификацию.

Соответствие master-фаз:

| Master-фаза | Пакеты этого runbook |
|---|---|
| 0. Consumer matrix | R0, R2, R11 |
| 1. Wire/capabilities | R2, R8 |
| 2. Release/CI | R3 |
| 3–6. Consumer-gated API cleanup | R11 |
| 7. External info | R10 |
| 8. Topology 4×4 | R1, R4–R9 |
| 9. Telemetry collapse | R12 |
| 10. Docs/dead code | R13 |

## 2. Конечный результат

Программа завершена, когда одновременно верно следующее:

1. Номер релиза продукта не связан с внутренним auth layout byte.
2. `authProtocolVersion = 9`, длина и offsets кадров сохранены.
3. Capabilities не публикуют wire range, lane count и implementation
   milestones.
4. Новый сервер принимает legacy-сессии на 4 lanes и новые сессии на 16 lanes.
5. Новый клиент использует ровно 4 calls × 4 workers; топология не является
   пользовательской настройкой.
6. Старые клиенты продолжают работать во время server-first rollout.
7. GitHub Release содержит только runtime allowlist, без Markdown, schemas,
   custom source archive, release manifest, checksum/provenance sidecars.
8. Android bundle сохраняет реально используемые signature, digest,
   anti-rollback и crash-safe механизмы.
9. `HydraCoreValidateConfig(..., "remote_v2")` остаётся trust boundary.
10. Subscription envelope/JWE, BuildInfo и runtime APIs удаляются из core только
    после фактической миграции Android consumer.
11. Транспортная telemetry сохраняется до завершения 4×4 observation window.
12. Для каждого затронутого репозитория есть зелёный удалённый CI и ссылка на
    run; локально tests/builds не запускались.

## 3. Неподвижный requirement ledger

Coding-агент обязан проверять каждую фазу по этим ID и перечислять их в отчёте.

### INV-01 — локальные запуски запрещены

Нельзя локально выполнять:

- `go test`, `go build`, `go run`, `make`;
- Gradle/Flutter/Dart build или test;
- `pytest`, `python verify.py`, `compileall`;
- code generation;
- release packaging и создание AAR/SO/tarball.

Разрешены чтение, `rg`, `git status`, `git diff`, `git diff --check`, точечное
редактирование и просмотр уже существующих артефактов. Команды из CI-матрицы
ниже исполняет только удалённый runner.

### INV-02 — чужие изменения сохраняются

- Не использовать `git reset --hard`, `git checkout --`, destructive clean или
  массовый restore.
- Не удалять untracked файлы только потому, что они не относятся к patch.
- Не менять index/staging без явного разрешения владельца.
- Не форматировать целый репозиторий.
- Перед каждым пакетом фиксировать `status`, staged diff и unstaged diff.

### INV-03 — auth layout остаётся прежним

- Оставить `authProtocolVersion = 9`.
- Оставить `frame[4]` и strict equality при decode request/ack.
- Не менять frame magic, длину, offsets и endian layout.
- Не создавать wire v10, min/max negotiation или второй handshake.
- Версия меняется только при реальном изменении байтового формата, которого в
  этой программе нет.

### INV-04 — topology является единственным product mode

Финальные константы:

```go
const (
    LegacyLaneCount  = 4
    CallCount        = 4
    WorkersPerCall   = 4
    LaneCount        = CallCount * WorkersPerCall // 16
    MaximumLaneCount = 32
)
```

Финальная раскладка:

```text
call 0: lanes 0, 4, 8, 12
call 1: lanes 1, 5, 9, 13
call 2: lanes 2, 6, 10, 14
call 3: lanes 3, 7, 11, 15
```

Формулы:

```go
callIndex := int(workerID) % CallCount
workerIndexWithinCall := int(workerID) / CallCount
joinLink := options.JoinLinks[callIndex%len(options.JoinLinks)]
```

Не добавлять `workers_per_call`, `WorkerGroup`, negotiation, feature flag или
runtime topology registry. Для 1–4 `join_links` переиспользуется целый call
slot, а не отдельные workers произвольным round-robin.

### INV-05 — server-first без flag-day

- Server release сначала принимает totals `{4, 16}`.
- Client в этом первом release всё ещё создаёт total=4.
- VPS разворачиваются и наблюдаются до появления client=16.
- Затем subscription перестаёт задавать topology через `workers`.
- Старый core при отсутствующем поле использует свой default=4; новый core —
  default=16.
- Только после этого публикуется/активируется client=16.

Это migration seam, а не negotiation: сервер не выбирает topology клиента, а
каждая версия клиента имеет один собственный default.

### INV-06 — security boundaries не упрощаются

Обязательно сохранить:

- `HydraCoreValidateConfig(config, "remote_v2")`;
- JWE algorithm/type/content-type/key-size/payload-size checks до переноса
  ownership;
- Ed25519 bundle signature;
- trusted embedded key и `keyId`;
- artifact ABI/size/SHA-256 validation;
- `releaseSequence` anti-rollback;
- atomic install/replace, candidate probe и crash-safe fallback;
- TLS roots/time, redirect refusal, request bounds и secret redaction.

### INV-07 — deletion-first

Не создавать replacement framework для удаляемой сложности:

- capabilities v3;
- subscription v3;
- новый release manifest;
- новый provider/cache interface для двух HTTP endpoints;
- новую telemetry database/queue/schema generator/analyzer;
- новый binary/classfile parser;
- deprecated stubs без production consumer.

Использовать существующие parser, manifest, updater, state migration chain и CI
jobs. Новая сущность допустима только если без неё нельзя выразить фактический
runtime state; сначала это доказывается caller trace.

## 4. Зафиксированный baseline

### 4.1. HydraCore

Репозиторий: `C:\Users\user\Desktop\hydracore\hydracore`.

На baseline:

- ветка `debug`;
- 22 unstaged modified files;
- staged удаления:
  `release/HYDRACORE_PERFORMANCE_BASELINE`,
  `release/verify_attribution_boundaries.sh`,
  `release/verify_hydracore_performance.sh`;
- untracked master-планы, `topology_4x4_test.go` и Graphify artifacts;
- `release/HYDRACORE_VERSION` содержит
  `v1.13.16-extended-hydracore.11-debug.44`;
- текущий diff смешивает capabilities, release, external info, subscription и
  4×4 changes и не является release-ready.

### 4.2. HYDRA-ULTIMATE

Репозиторий: `C:\Users\user\Desktop\hydra\HYDRA-ULTIMATE`.

На baseline:

- ветка `debug`;
- 7 modified files в calls/capability/subscription tests и services;
- `hydra/contracts/calls_configuration.py` всё ещё задаёт `MAX_WORKERS = 4`;
- `hydra/core/state_models.py` содержит `SCHEMA_VERSION = 15`;
- последняя calls migration — `v14 -> v15` и записывает topology=4.

Перед любым изменением здесь полностью прочитать корневой `AGENTS.md`. Новая
state schema требует ровно одну последовательную идемпотентную migration,
upgrade/rollback tests и запрета старому runtime читать новый state после
rollback.

### 4.3. HydraBox

Репозиторий: `C:\Users\user\Desktop\HydraBox\hydrabox`.

На baseline:

- ветка `debug`, worktree чистый;
- submodule `hydracore` pinned на `c3abeeb3`/debug.44 lineage;
- `HydraCoreCapabilities.parseStrict` требует удаляемые milestone/wire поля;
- `isCompatibleRelease` exact-gate'ит старую реализацию;
- `AppBootstrapController` отклоняет несовместимый document;
- `outbound_schema.dart` ошибочно требует `workers == 8`;
- Android minSdk равен 26;
- production bundle updater, signature verifier, candidate probe и active/
  previous slots существуют и реально используются.

### 4.4. Ограничение Graphify

Существующий graph охватывает только `transport/call/vk-parasite` и построен
до части текущих изменений. Он полезен для связей `ParasiteTunnel`, auth,
recovery, flow migration и telemetry, но не доказывает cross-repo consumer
absence. Source search по всем трём репозиториям всегда сильнее graph result.

## 5. Реестр незакрытых проблем

| ID | Приоритет | Проблема | Gate закрытия |
|---|---:|---|---|
| BUG-01 | P0 | `uint32` recovery masks всё ещё смешаны с `uint8` | Нет `uint8` операций над recovery masks; remote compile/tests green |
| BUG-02 | P0 | HydraBox strict parser не принимает сокращённые capabilities | Consumer release принимает old+new documents до producer removal |
| BUG-03 | P0 | Core=16, HYDRA=4, HydraBox=8 | Один rollout contract: old client=4, new=16, server={4,16}, subscription не задаёт topology |
| BUG-04 | P1 | Per-worker KCP/lane gauges перестали обновляться | Snapshot lane 15 содержит актуальные gauges в remote tests |
| BUG-05 | P1 | Pacing test требует 500 ms между всеми 16 lanes | Test проверяет около 1 s только внутри одного call slot |
| BUG-06 | P1 | debug.44 можно перезаписать несовместимым contract | Использованы новые monotonic versions; debug.44 неизменяем |
| BUG-07 | P2 | Release всё ещё публикует source tar/manifest/SHA256SUMS/capabilities dump | Literal 9-asset allowlist и inventory gate |
| BUG-08 | P2 | Bundle manifest строится дважды | Один producer/signing invocation в publish job |
| BUG-09 | P2 | Release notes содержат старую историю | Один короткий текущий note; история только в changelog |
| BUG-10 | P2 | Всегда строится legacy API21 AAR | Legacy build/helper удалены; main AAR проверен remote Android CI |
| BUG-11 | P2 | External info timeout стал 5 s и tests урезаны неверно | 4.5 s и минимальные пять behavior groups в remote CI |
| BUG-12 | Gate | Старый Android updater может установить bundle с новым capability shape | Existing `coreApiMajor` безопасно разделяет contracts; old app reject/new app accepts old+new |

## 6. Общий граф исполнения

```text
R0 baseline/consumer matrix
  |
  +--> R1 repair current core diff (не публиковать)
  |
  +--> R2 HydraBox compatibility bridge release
            |
            +--> old capability JSON accepted
            +--> new reduced JSON accepted
            +--> bundle coreApiMajor 1/2 accepted
            +--> subscription topology field stripped/omitted
                     |
R3 release pipeline cleanup
  |
R4 server {4,16}, client still 4
  |
R5 new server release -> VPS deploy -> observation gate
  |
R6 HYDRA max=16 state migration + subscription omits workers
  |
R7 core client=16 + complete 4x4 behavior
  |
R8 reduced capability producer + bundle API major 2
  |
R9 client release + HydraBox pin/update
  |
R10 independent external-info cleanup
  |
R11 consumer-gated phases 3–6
  |
R12 production observation -> telemetry collapse
  |
R13 final docs/dead-code pass
```

R2, R5, R6 и R9 — release/deployment gates. Код следующего пакета можно
готовить параллельно в отдельной ветке, но нельзя публиковать или объявлять
rollout завершённым раньше соответствующего gate.

## 7. Порядок работы coding-агента

Для каждого пакета R0–R13:

1. Перечитать этот пакет, связанные master sections и repository `AGENTS.md`.
2. Выполнить `git status --short --branch`, `git diff --name-status` и
   `git diff --cached --name-status` во всех затронутых repositories.
3. Найти production entrypoint, каждого caller и tests через `rg`.
4. Сформулировать минимальный contract и список файлов до редактирования.
5. Сделать один связный change без unrelated formatting.
6. Добавить/исправить tests, но не запускать их локально.
7. Выполнить только static readback, `rg`, `git diff` и `git diff --check`.
8. Передать commit-ready patch и remote CI matrix.
9. Не выполнять push, merge, `gh release`, deploy или state migration на живой
   VPS без отдельного явного разрешения владельца.
10. Закрыть пакет только после зелёного remote CI и, для rollout packages,
    после production evidence.

Если создание commits разрешено, использовать сообщения из пакетов ниже. Если
нет — не менять index, а перечислить точные файлы для каждого будущего commit.

## 8. R0 — containment и повторный consumer matrix

### Цель

Не потерять уже сделанную работу и не продолжать исправлять смешанный diff
вслепую.

### Действия

1. Зафиксировать status/diffstat всех трёх repositories в рабочем отчёте.
2. Отдельно классифицировать:
   - user-owned pre-existing changes;
   - изменения первой попытки агента;
   - staged удаления;
   - untracked docs/tests/generated/Graphify artifacts.
3. Не добавлять `graphify-out` в product commits.
4. Составить field-by-field consumer matrix для:
   - HydraCore capability JSON;
   - `workers` и `max_workers_per_session`;
   - bundle manifest;
   - BuildInfo;
   - subscription libbox API;
   - aggregate/dedicated runtime events;
   - managed URLTest parameters.
5. Для каждого поля записать: producer, parser, behavioral branch, persisted
   storage, fixture, release/update dependency.
6. Проверить наличие новых `AGENTS.md` перед редактированием каждого repo.

### Acceptance

- Ни один exported symbol/JSON field не помечен unused только по одному repo.
- HydraBox parser/updater и HYDRA state/subscription учтены.
- Dirty changes не потеряны и не смешаны с generated artifacts.
- BUG-01…BUG-12 имеют owner и пакет закрытия.

### Stop-gate

Если невозможно определить владельца существующего изменения или production
caller, агент не удаляет код и запрашивает решение владельца.

## 9. R1 — починить текущий HydraCore diff до внутренне связного состояния

### Цель

Устранить явные compile/test regressions первой попытки. Этот пакет не меняет
version, не публикуется и не переключает production client на 16.

### R1.1. Закончить `uint32` mask migration

Файлы:

- `transport/call/vk-parasite/lane_tunnel.go`;
- `transport/call/vk-parasite/lane_tunnel_test.go`;
- `transport/call/vk-parasite/flow_migration.go`;
- `transport/call/vk-parasite/topology_4x4_test.go`.

Точные исправления:

- заменить каждый `uint8(1 << workerID)` на `uint32(1) << workerID`;
- операции set/clear/test для `recoveryPending` и `recoveryDeferred` выполнять
  одним типом `uint32`;
- проверить bit 15 и сохранность bits 0..14;
- проверить остальные реальные lane masks, включая ordered/unordered flow и
  migration;
- не расширять `windowDemandBits`: это двухбитная temporal history, а не lane
  mask;
- не создавать generic bitset helper.

Static gate:

```text
rg по recoveryPending/recoveryDeferred не находит uint8 cast
rg по uint8(1 << ...) не находит lane-mask operations
```

### R1.2. Восстановить per-worker gauges

Каноническое место — `ParasiteTunnel.telemetryWorkerSnapshots`. Не возвращать
дублирующий refresh одновременно в `TelemetryValues`.

Использовать прежнюю реализацию из `git show HEAD:.../lane_tunnel.go` только как
источник списка вычислений, затем адаптировать её к dynamic `t.lanes`.

Перед `lane.metrics.Snapshot(metrics)` обновить существующие gauges из уже
имеющегося lane/KCP state, включая как минимум:

- `LaneCount` для worker snapshot как размер текущей session либо documented
  per-worker semantics, одинаковые на client/server;
- `LaneFlowCount` по фактическим flows этой lane;
- `KCPWaitSnd`;
- `KCPRTTMS`, `KCPRTOMS`, `KCPRTTVarMS`;
- `KCPInflightSegments`;
- уже имеющиеся queue depth/capacity, admission, pacing, delivery, generation,
  state, min RTT, application-limited и ACK age.

Не менять counters через `Set`; counters продолжают обновляться в местах
событий. Не читать KCP/lane state без существующих locks.

Remote regression test обязан доказать, что snapshot worker/lane 15 содержит
актуальные non-stale значения KCP, lane и flow gauges. Недостаточно проверить
только наличие JSON keys.

### R1.3. Исправить pacing test, а не алгоритм

Текущая формула при interval=4 s и 16 lanes даёт 250 ms между соседними lane
IDs. Это нормально, потому что соседи принадлежат разным calls.

Переписать stale assertion:

- проверять группы `slot, slot+4, slot+8, slot+12`;
- внутри одного call slot соседние probes должны быть разнесены примерно на
  1 s с существующим test tolerance;
- отдельно допускается 250 ms между adjacent IDs разных calls;
- не менять `probeOffset()` только ради старого порога 500 ms.

### R1.4. Не потерять transport telemetry до observation

- Не удалять worker snapshot metric lists.
- Не удалять recovery/quarantine/TURN/DTLS/auth/goodput/RTT/retransmission
  данные, нужные R5/R12.
- Не начинать master-фазу 9 в этом patch.

### Acceptance

- BUG-01, BUG-04 и BUG-05 закрыты кодом и remote test definitions.
- Static search не находит mixed-width lane masks.
- Tunnel aggregate и worker gauges имеют по одному owner.
- Version/release files не менялись.
- Remote HydraCore server/client role compile and package tests green.

Предлагаемый commit: `fix(calls): complete dynamic lane mask and telemetry repair`.

## 10. R2 — HydraBox consumer-first compatibility bridge

### Цель

До удаления capability fields и до нового bundle научить production Android
consumer безопасно принимать старый и новый contracts.

### R2.1. Упростить capability parser

Основные файлы:

- `lib/singbox/hydracore_capabilities.dart`;
- `lib/singbox/singbox_runtime.dart`;
- `lib/app/app_bootstrap_controller.dart`;
- `test/hydracore_capabilities_test.dart`;
- `test/hydracore_compatibility_test.dart`;
- `test/fixtures/hydracore_client_capabilities_*.json`;
- `scripts/verify_hydra_ultimate_contract.py`;
- `scripts/verify_extended_core.py`;
- `android/app/libs/README.md`.

Удалить из required parse/model/compatibility gate:

- `call_vk_parasite_wire.{min,max}`;
- `call_vk_eight_lane_kcp`;
- `call_vk_four_lane_kcp`;
- `call_vk_pre_kcp_admission`;
- `call_vk_relay_flow_control`;
- `call_vk_worker_hot_swap`;
- `call_vk_flow_migration`;
- `call_vk_turn_tcp_fallback`;
- `call_vk_transport_health`.

Старый подробный JSON должен приниматься потому, что лишние поля игнорируются,
а не потому, что создаётся второй legacy parser. Новый сокращённый JSON должен
проходить тем же кодом.

Оставить required только реально используемые consumer contracts. В частности,
пока нельзя удалять:

- identity/API version/core role;
- `call_vk_parasite` и реально используемые call modes;
- validation profiles с `remote_v2`;
- `RemotePolicy.safeOutboundTypes`, пока
  `singbox_config_builder.dart` использует его;
- runtime/schema fields, которые читает Android runtime/updater;
- остальные feature fields, по которым UI/config builder действительно
  ветвятся.

Не пытаться в одном commit свести весь client capability JSON к VPS-примеру из
master-плана: сначала нужен полный field consumer matrix.

### R2.2. Закрыть bundle compatibility gap

Старый app принимает только bundle `coreApiMajor == 1`, а updater реально
может установить новый signed core. Изменение capability contract поэтому
должно использовать существующий compatibility gate:

1. Этот HydraBox release принимает bundle API majors `{1, 2}`.
2. Он одинаково понимает old detailed и new reduced capability JSON.
3. Старые HydraBox releases продолжают принимать только major 1 и безопасно
   отвергают будущий bundle major 2 до download/activation.
4. HydraCore producer переключает bundle на major 2 только в R8.

Не добавлять новое поле, updater protocol или capabilities v3. Это ровно одно
breaking-contract применение уже существующего `coreApiMajor`.

Remote tests:

- old app parser rejects manifest major 2;
- new parser accepts signed major 1 и 2;
- unsupported major 3 rejected;
- signature, digest, ABI, releaseSequence and candidate-probe checks не
  ослаблены;
- reduced capability JSON успешно проходит bootstrap после candidate probe.

### R2.3. Подготовить topology migration seam

В `outbound_schema.dart` удалить неверное требование `workers == 8`.

Целевое поведение consumer:

- поле отсутствует — допустимо;
- legacy cached value `4` — принимается на subscription boundary, но удаляется
  из sanitized config перед `remote_v2`/native runtime;
- target value `16` — также не нужен и удаляется;
- другие значения, особенно 8, не объявляются поддерживаемой topology;
- credentials, join links, bounds и timeout checks сохраняются.

Причина удаления поля: topology выбирает embedded/updated core version через
свой единственный default, а не subscription producer. Не заменять поле другим
topology knob.

Добавить fixtures для missing/4/16/8 и проверить, что в финальном raw sing-box
config, отправленном core, `workers` отсутствует.

### R2.4. Release gate

- Выпустить HydraBox compatibility release с прежним embedded core debug.44.
- Зафиксировать app version/build и distribution channel.
- Подтвердить remote Android/Dart CI.
- Не считать R2 закрытым по merged code без опубликованного consumer release.

Предлагаемый commit: `fix(core-contract): accept reduced capabilities and topology defaults`.

## 11. R3 — release/CI cleanup до следующего HydraCore release

### Цель

Следующий server release уже не должен повторять текущий asset bloat и не
должен перезаписывать debug.44.

### R3.1. Один источник build version

- Использовать `release/HYDRACORE_VERSION` и существующий env/ldflag path.
- Удалить workflow step, создающий synthetic/mutable Git tags.
- Текущий debug.44 считать неизменяемым опубликованным contract.
- Для server release выделить следующий свободный monotonic version
  `V_server` (на baseline ожидается не ниже debug.45, но номер проверить в
  GitHub перед изменением).
- Для client=16 выделить отдельный последующий `V_client`.
- Не использовать `--clobber` как способ заменить несовместимый release под
  старым tag.

### R3.2. Удалить legacy AAR

В `cmd/internal/build_libbox/main.go`:

- удалить unconditional `libbox-legacy.aar` build;
- удалить `filterTags`, если после этого нет callers;
- оставить один основной AAR;
- не вводить новый boolean/matrix flag для legacy build.

Основание: production HydraBox имеет minSdk 26; API21 consumer не найден.

### R3.3. Сохранить нужный bundle, убрать release clutter

Bundle идёт по security-gated Path B: production updater существует.

Итоговый GitHub Release allowlist — ровно девять файлов:

```text
hydracore-android-arm64-v8a-libbox.so
hydracore-android-armeabi-v7a-libbox.so
hydracore-android-x86_64-libbox.so
hydracore-bundle-manifest-v1.json
hydracore-bundle-manifest-v1.sig
hydracore-client-libbox-sources.jar
hydracore-client-libbox.aar
hydracore-vps-linux-amd64.tar.gz
hydracore-vps-linux-arm64.tar.gz
```

Удалить из release и `dist` final inventory:

- `HYDRA_SUBSCRIPTION_V2.md` и schemas;
- standalone `.sha256`;
- `SHA256SUMS`;
- `hydracore-source.tar.gz`;
- `hydracore-release-manifest.json`;
- provenance sidecars;
- standalone `hydracore-client-capabilities.json` asset.

`capabilitiesSha256` пока сохранить внутри signed bundle manifest: его реально
сравнивает `CoreCandidateProbeClient`. Capability JSON можно создать как
временный CI input вне final `dist`, но нельзя публиковать как asset.

### R3.4. Собирать/sign manifest один раз

- Не запускать `cmd/internal/build_core_bundle` сначала unsigned, затем signed.
- После загрузки producer artifacts вызвать существующую command один раз в
  publish job с signing key.
- Этим же вызовом получить три ABI `.so`, один manifest и одну signature.
- Не создавать новую signing wrapper/tool.

### R3.5. Literal inventory gate

Перед `gh release upload`:

1. Сформировать literal sorted expected list из девяти имён.
2. Получить sorted actual regular files непосредственно из `dist`.
3. Выполнить exact diff и оборвать job при missing/unexpected file.
4. Передать в `gh release upload` те же девять explicit paths, не glob.
5. Проверить, что каждый Linux tar содержит ровно `sing-box`, version совпадает,
   а один VPS capability smoke подтверждает identity/role/call mode.

### R3.6. Release notes/docs

- `release/HYDRACORE_RELEASE_NOTES.md` содержит только текущий release.
- Удалить все `Previous debug.* notes`.
- История остаётся в `CHANGELOG.md`.
- В `HYDRACORE.md` описать auth byte как internal layout marker, server-first
  rollout и правильную call-slot mapping.
- В `docs/CALL_VK_TELEMETRY.md` убрать product coupling к wire number.
- Исторические changelog entries не переписывать.

### Acceptance

- BUG-06…BUG-10 закрыты.
- Workflow не содержит `dist/*`, `git archive`, `SHA256SUMS` и второго bundle
  build.
- Remote artifact inventory job показывает ровно девять expected assets.
- Android AAR/ABI inspection, Linux packages и debug/stable semantics green.

Предлагаемый commit: `ci(release): publish only signed runtime artifacts`.

## 12. R4 — server compatibility foundation, client остаётся на 4

### Цель

Подготовить и выпустить сервер, способный принять будущий client=16, не меняя
поведение текущего client artifact в том же release.

### R4.1. Dynamic tunnel

Файлы:

- `transport/call/vk-parasite/lane_tunnel.go`;
- `transport/call/vk-parasite/server.go`;
- `transport/call/vk-parasite/client.go`;
- связанные package tests.

Требования:

- internal constructor принимает session lane count;
- containers/capacities/recovery slices используют фактический `len(t.lanes)`;
- structural bound `1..MaximumLaneCount` остаётся отдельно от production policy;
- server создаёт tunnel по authenticated `request.WorkerTotal`;
- существующая session ID не может сменить `WorkerTotal`;
- lane 15 attach допустим только для 16-lane session, lane 16 rejected.

### R4.2. Auth validation

Decoder структурно проверяет:

- total > 0;
- total <= 32;
- workerID < total;
- auth byte равен 9;
- strings и frame length остаются bounded.

Production server policy после decode принимает только total 4 или 16.
Auth decoder не должен дублировать deployment policy.

### R4.3. Server policy

- `HardMaxWorkers = 16`.
- Default server max = 16.
- Явный max разрешён только 4 или 16, не любое число между ними.
- max=4 принимает только total4.
- max=16 принимает total4 и total16.
- total8 и totals5..15 rejected.
- `IngressWorkers` остаётся отдельной настройкой и не смешивается с lane total.

### R4.4. Client policy этого release

В `V_server` production client должен:

- default `Workers` на 4;
- принимать только 4;
- создавать 4-lane tunnel;
- не выполнять 4×4 call mapping.

Это временно не новый flag: используется существующее поле/options validation.
В R7 тот же единственный default меняется на 16 отдельным release.

### R4.5. Recovery, health и cold-start

- Recovery quorum вычисляется из session size: 4 => 3, 16 => 12.
- Health/usable/active/telemetry используют actual lane count.
- Initial aggregate pacing/capacity не должен автоматически вырасти в 4 раза.
- TURN allocation gate остаётся глобальным существующим gate.
- Не добавлять quota classifier/backoff до production evidence массового 486.

### Обязательные remote tests

- auth total4/total16+lane15 и все reject cases;
- byte length/offset regression;
- tunnel size4/16, lane15 attach, lane16 reject;
- bit15 recovery/migration/flow masks;
- quorum behavior 3/4 и 12/16, не только formula assertion;
- simultaneous legacy4 и new16 sessions на одном server;
- max4/max16/total8/session-total-mismatch;
- server role package/race tests.

### Acceptance

- Server side полностью поддерживает 4/16.
- Client artifact всё ещё создаёт 4.
- Wire byte/layout не изменены.
- Remote server/client role tests green.
- Никакого release до R2 и R3 gates.

Предлагаемый commit: `feat(calls): accept legacy and 4x4 server sessions`.

## 13. R5 — server release, deploy и обязательное наблюдение

### Release

1. Назначить новый `V_server`, не переиспользуя debug.44.
2. Bundle manifest остаётся API major 1, если capability shape ещё старый; либо
   major 2 только после опубликованного R2 consumer. Никогда не публиковать
   reduced shape под major 1.
3. Получить зелёные remote jobs и зафиксировать URLs.
4. Проверить ровно девять release assets.
5. Развернуть только VPS artifacts сначала. Не активировать client=16.

### Production evidence

Зафиксировать до перехода к R6:

- версия/commit на каждой целевой VPS;
- server config max=16 либо готовность применить его в R6;
- legacy total4 sessions успешно создаются;
- auth/DTLS/TURN failure rates не ухудшились;
- active/usable lanes и recovery остаются нормальными;
- rollback artifact/version доступны.

### Rollback

Пока все clients total4, VPS можно откатить на предыдущий server. После
активации client16 rollback VPS на server4 запрещён до отката/остановки новых
clients.

### Acceptance

R5 закрывается только ссылками на release, CI и deployment/telemetry evidence.
Merged code без фактического server-first deploy не закрывает пакет.

## 14. R6 — HYDRA state/config и subscription migration без flag-day

### Цель

Поднять server max до16 и перестать передавать client topology в subscription,
не ломая старые приложения.

### R6.1. State migration

Перед изменением перечитать `HYDRA-ULTIMATE/AGENTS.md` и актуальные schema
constants. На baseline нужен переход 15 -> 16, но агент использует фактический
current `SCHEMA_VERSION` на момент работы.

Изменить:

- `hydra/contracts/calls_configuration.py`;
- `hydra/services/calls.py`;
- calls state model/validation;
- `hydra/core/state_migrations/calls_vk.py` или фактический owner;
- migration registry;
- upgrade/rollback docs/tests;
- subscription fixtures/tests;
- telemetry expectations, если worker count используется как desired gauge.

Migration должна:

- изменить persisted `max_workers_per_session` с legacy4 на16 только для
  `vk_parasite` config, где значение соответствует старому managed default;
- не переписывать unrelated/user-invalid values молча;
- удалить persisted client `workers` как topology knob, если поле является
  managed product default;
- быть pure, idempotent и ровно одной ступенью;
- сохранять future-schema rejection и atomic state semantics.

### R6.2. Subscription output

`vk_parasite_outbound()` больше не публикует `workers`.

Это обязательная compatibility техника:

| Consumer | Subscription `workers` | Runtime result |
|---|---|---|
| Старый core | отсутствует | его default = 4 |
| Новый core | отсутствует | его default = 16 |
| Новый HydraBox + cached `workers:4` | consumer удаляет поле | embedded core default |
| Любой consumer + `workers:8` | не считается поддерживаемой topology | reject/strip по documented boundary |

Не добавлять app-version branching на HYDRA, topology negotiation или новый
subscription contract. `join_links` 1..4, credentials и security requirements
остаются.

### R6.3. Server configuration

- Generated inbound использует `max_workers_per_session = 16`.
- Apply выполняется только после R5 server deploy.
- Migration/apply имеет snapshot и rollback по правилам HYDRA.
- Rollback на runtime, не понимающий новую state schema, запрещён; upgrade
  procedure должна восстановить совместимый snapshot/runtime pair.

### Remote tests

- raw validation старой и новой schema;
- vN->vN+1 migration, full chain и second-run idempotency;
- future version rejection;
- atomic save/revision/rollback paths;
- subscription output не содержит `workers`;
- generated server inbound max=16;
- старые fixtures migration;
- targeted calls/subscription tests;
- полный `python verify.py`;
- Linux integration apply/rollback smoke.

### Acceptance

- HYDRA больше не выбирает client lane count.
- Новый server config принимает 4/16.
- Старые clients продолжают default4 на subscription без поля.
- Remote state migration/upgrade/rollback evidence приложено.

Предлагаемые commits:

1. `feat(calls): migrate server capacity to sixteen lanes`;
2. `refactor(subscription): stop publishing client lane topology`.

## 15. R7 — завершить core client 4×4

### Цель

После server deploy и R6 переключить новый client на единственную topology16.

### Изменения

В `transport/call/vk-parasite/client.go`:

- default `Workers = 16`;
- explicit value принимается только 16;
- error сообщает “exactly sixteen VK lanes (four workers per call)”;
- missing subscription field проходит через default;
- mapping вынесена максимум в один маленький pure helper;
- `join_links` остаются 1..4 unique bounded links;
- readiness после первого подключённого worker сохраняется.

Не ожидать все16 workers перед ready. Не создавать group runtime state.

### Обязательные remote tests

1. Table mapping для 1/2/3/4 links.
2. Каждый slot имеет IDs `slot+4k`.
3. При 3 links slot3 целиком повторно использует link0.
4. Production client default16; explicit4 rejected.
5. Один 16-lane in-memory pair:
   - 16 active workers с обеих сторон;
   - ordered и unordered traffic;
   - hot swap lane15;
   - logical session живёт после replacement.
6. Lower-level recovery suite по возможности остаётся на internal tunnel4, чтобы
   не увеличить CI cost в четыре раза.
7. Client role, libbox, Android AAR и race jobs green.

### Acceptance

- BUG-03 закрыт end-to-end вместе с R5/R6/R9.
- Новый client никогда не создаёт 8 lanes.
- Wire/layout unchanged.
- Cold-start resource usage отражено в remote CI output без нового blocking
  performance threshold.

Предлагаемый commit: `feat(calls): use four calls with four workers each`.

## 16. R8 — удалить public wire/milestone capabilities

### Preconditions

- R2 HydraBox release опубликован.
- Старый app безопасно отвергает bundle API major2.
- Новый app принимает bundle majors1/2 и оба capability shapes.
- HYDRA consumer gate уже требует только product features.

### Producer changes

В HydraCore удалить:

- `WireCompatibility`;
- `ProtocolSet.CallVKParasiteWire`;
- `callWireMin/callWireMax` из build-tag files;
- four/eight lane flags;
- pre-KCP/relay/hot-swap/flow-migration/TURN-fallback/transport-health milestone
  flags;
- exact CI assertions и tests на эти поля.

Оставить:

- `api_version = 2`;
- identity/version/role;
- product features, по которым consumer реально ветвится;
- `call_vk_parasite` и call mode;
- `call_vk_telemetry` до R12;
- client-only RemotePolicy/runtime/config fields до их отдельных migrations.

`authProtocolVersion` остаётся только в transport/auth tests. Не переименовывать
его в product version и не экспортировать.

### Bundle contract

- Bundle с reduced capability shape публикуется как existing `coreApiMajor = 2`.
- Major1 больше не используется для новых reduced-shape bundles.
- `capabilitiesSha256` остаётся подписанным/проверяемым полем, пока его сравнивает
  candidate probe.
- Не вводить capabilities API v3.

### Acceptance

Repo-wide active-code search не находит:

```text
call_vk_parasite_wire
callWireMin
callWireMax
call_vk_four_lane_kcp
call_vk_eight_lane_kcp
call_vk_pre_kcp_admission
call_vk_relay_flow_control
call_vk_worker_hot_swap
call_vk_flow_migration
call_vk_turn_tcp_fallback
call_vk_transport_health
```

Допустимы только исторические changelog/migration fixtures с явным комментарием.
Old detailed и new reduced documents проходят HydraBox remote tests.

Предлагаемый commit: `refactor(capabilities): expose product contracts only`.

## 17. R9 — client release и HydraBox activation

### Release order

1. Завершить R5/R6 и подтвердить, что VPS fleet принимает16.
2. Назначить новый `V_client`, отличный от `V_server` и debug.44.
3. Собрать remote CI artifacts с client16 и bundle API major2.
4. Проверить nine-asset inventory и signed manifest.
5. Опубликовать core release.
6. В HydraBox обновить submodule/pin, fixture, provenance, README и expected
   version на `V_client`.
7. Выпустить HydraBox с embedded client16.
8. Проверить user-driven bundle updater: check, download, isolated probe,
   activate, healthy mark, rollback.

### Rollback rules

- Client/app можно откатить на core4, пока server принимает4.
- VPS нельзя откатывать на server4, пока существуют активные/доступные client16.
- Bundle anti-rollback нельзя обходить ручным снижением `releaseSequence`.
- При candidate failure использовать существующий previous slot; не копировать
  `.so` вручную поверх active.

### Acceptance

- Новый app реально создаёт16-lane sessions на deployed server.
- Старый app с subscription без `workers` продолжает создавать4.
- App bootstrap принимает reduced capabilities.
- Release/CI/update/deployment URLs приложены.
- BUG-02, BUG-03, BUG-06 и BUG-12 закрыты production evidence.

## 18. R10 — отдельно завершить external info simplification

### Цель

Сохранить public `LookupOutboundExternalInfo(outboundTag)` и удалить только
cache/singleflight state.

### Изменения

- Общий timeout вернуть к 4.5 s.
- Per-source attempt оставить 2 s.
- Оставить primary Cloudflare trace и простой ipify fallback loop.
- Оставить instance cancellation через existing context.
- Оставить custom outbound resolve dialer, TLS roots/time.
- Redirects запрещены.
- Body limit 64 KiB.
- IP/country validation и error joining сохраняются.
- Не возвращать resolver/cache/singleflight/provider interface.

### Remote tests

- invalid nil request/blank/unknown tag;
- primary success и fallback не вызван;
- primary failure + fallback success в правильном порядке;
- malformed and oversized response;
- request/instance context cancellation.

### Acceptance

- Public gRPC/libbox API unchanged.
- `daemon/instance.go` не хранит resolver state.
- BUG-11 закрыт.

Предлагаемый commit: `refactor(external-info): use bounded direct fallback lookup`.

## 19. R11 — master-фазы 3–6 только consumer-first

Эти работы не входят в 4×4 release. Каждая выполняется отдельной серией
consumer -> rollout evidence -> producer deletion.

### R11.1. Phase 3: BuildInfo

1. Найти Android/Dart/Kotlin callers `HydraCoreBuildInfo`.
2. Перевести diagnostics на capabilities version/role и existing release/update
   metadata.
3. Выпустить HydraBox consumer.
4. Только затем удалить:
   - `experimental/libbox/hydracore_build_info.go`;
   - test;
   - `hydraCoreSourceCommit` linker flag.
5. Не создавать новый runtime lineage JSON.

Gate: Android diagnostics показывает version; repo-wide callers отсутствуют;
remote app/core CI green.

### R11.2. Phase 4: bundle simplification, Path B

Consumer существует, поэтому bundle нельзя удалить как dead code.

Фактически используемые поля на baseline:

- schema/distribution/version;
- `releaseSequence`, `keyId`, `publishedAt`;
- core API major;
- runtime/config/subscription ranges;
- `capabilitiesSha256`;
- artifacts ABI/name/size/hash/minSdk.

Для каждого удаляемого manifest field сначала обновить HydraBox parser и
release его. Security fields не удалять. `coreApiMinor`, source/upstream commits
и избыточные ranges можно удалить только после отдельного caller/UX trace.

Не заменять Android loader patch новым bytecode parser/dependency. Отдельно
решить source-owned loader; если это невозможно без classfile rewriting,
представить владельцу вариант полного отказа от updateable bundle, но не
выполнять его без решения.

### R11.3. Phase 5: subscription ownership

Целевой flow:

```text
HYDRA produces v2 envelope/JWE
  -> HydraBox fetches, authenticates, validates product requirements
  -> HydraBox selects profile/resource
  -> raw sing-box JSON
  -> HydraCoreValidateConfig(raw, "remote_v2")
  -> runtime apply/check
```

Порядок:

1. HydraBox реализует/использует existing JWE library и старые plain/JWE v2
   fixtures.
2. HydraBox перестаёт вызывать все libbox subscription methods.
3. Выпускается consumer и фиксируется minimum supported app version.
4. Наблюдение подтверждает отсутствие старых ABI calls.
5. HydraCore удаляет subscription Go files/tests, `contract/subscription` и
   go-jose dependency только при отсутствии других callers.
6. `HydraCoreValidateConfig` и profile `remote_v2` остаются.

Не создавать subscription v3 и deprecated stubs.

### R11.4. Phase 6: runtime events и URLTest

1. Составить method/parameter matrix из реальных Kotlin/Dart callers.
2. Если aggregate runtime stream используется, перевести consumer на one-shot
   snapshot + existing dedicated status/groups/clash/URLTest streams.
3. Выпустить Android consumer.
4. Затем удалить aggregate gRPC messages, libbox command/handler/converters и
   duplicate delta loop.
5. Generated protobuf меняет только разрешённый удалённый codegen job.
6. Для URLTest оставить только параметры, которые UI реально задаёт; не
   заменять positional API новым options object.

Gate: одно runtime state имеет одного transport owner; UI progress/cancel
сохранены там, где реально используются.

## 20. R12 — observation window и только затем telemetry collapse

### R12.1. До observation ничего не удалять

Сохранять metrics, достаточные для решений по:

- active/usable lanes;
- TURN allocation/failure/latency;
- DTLS и inner-auth failures;
- VK credential failures/cache behavior;
- RTT, RTO, retransmission и goodput;
- queue drops/backpressure;
- recovery/quarantine/reset/session replacement;
- CPU/goroutines/memory для сравнения 4 и16.

### R12.2. Evidence record

До начала владелец задаёт observation interval и traffic/sample coverage.
Агент записывает:

- start/end timestamps;
- versions server/client/HydraBox/HYDRA;
- fleet/sample size;
- legacy4/new16 share;
- incidents/rollbacks;
- metrics, которые реально изменили operational decision;
- список reports, экспортированных перед deletion.

Без этих данных phase9 имеет статус `blocked by observation`, а не `complete`.

### R12.3. Collapse

После gate:

- оставить примерно 20–30 реально полезных transport metrics;
- удалить duplicate global allowlists и host CPU/RSS/kernel collectors из core;
- удалить unused public telemetry path/options;
- перейти на existing structured journal path;
- удалить custom file sink/rotation/state pointer целиком;
- в HYDRA удалить duplicate telemetry lab/application service, если владелец не
  подтвердил его как product feature;
- не создавать новую telemetry platform.

Security: records не содержат email, password, token, join link, raw session
ID или payload; parser bounded и принимает known marker/schema.

### Acceptance

- Transport correctness не зависит от telemetry delivery.
- Один факт не хранится одновременно в native file и journal.
- Observation evidence приложено.
- Удаления имеют remote core/HYDRA CI и rollback note.

## 21. R13 — финальный docs/dead-code pass

Обновить только active docs и верхние changelog entries. Исторические записи
не переписывать.

Repo-wide классифицировать остатки:

```text
call_vk_parasite_wire
callWireMin / callWireMax
call_vk_four_lane_kcp
call_vk_eight_lane_kcp
HYDRACORE_CALLS_WIRE
HydraCoreBuildInfo
HydraCore*Subscription*
hydracore-bundle-manifest
capabilitiesSha256
provenance
.sha256
SubscribeRuntimeEvents
RuntimeEventHandler
TelemetryStateDirectory
TelemetryOutputPath
calls-telemetry.jsonl
dist/*
Previous debug.
workers = 8
MAX_WORKERS = 4
```

Каждый остаток должен быть одним из:

- production caller/contract, который сознательно сохраняется;
- historical changelog;
- migration fixture старых данных;
- ошибка, которую нужно удалить до закрытия.

Удалить случайные generated/Graphify artifacts из commit scope, но не с диска
без разрешения владельца. Финальный `git status` всех трёх repositories должен
быть либо clean, либо содержать явно перечисленные owner changes.

## 22. Remote CI matrix

Все команды в этом разделе выполняются только удалёнными runners.

### 22.1. HydraCore server PR/release

- base Go suite;
- `with_call_server` role suite;
- `transport/call/vk-parasite` и parent package;
- existing Calls race suite;
- simultaneous4/16 and lane15 behavioral tests;
- Linux amd64/arm64 packaging;
- release inventory exact-match job;
- VPS capability/runtime smoke.

### 22.2. HydraCore client PR/release

- `with_call_client` role suite;
- relevant `experimental/libbox` suites;
- 16-lane in-memory correctness test;
- lane15 recovery/migration/hot-swap;
- existing race suite;
- Android AAR build and ABI/version inspection;
- signed bundle manifest/probe artifacts;
- correctness timing/peak memory reported, но без нового performance threshold.

Generic `with_call` и отдельный `TestWireV9` job не возвращать, если role/package
jobs уже выполняют те же tests.

### 22.3. HYDRA-ULTIMATE

- calls contract/config/infrastructure tests;
- subscription generation/JWE tests;
- state raw/model/migration/idempotency/future-version tests;
- atomic save/revision/rollback tests;
- architecture/boundary tests;
- полный `python verify.py`;
- Linux integration upgrade/apply/rollback smoke.

### 22.4. HydraBox

- Dart format/analyze/test jobs;
- old/new capability fixtures и bootstrap compatibility;
- subscription missing/4/16/8 sanitization tests;
- `remote_v2` handoff tests;
- Android unit/instrumented tests;
- bundle manifest major1/2 compatibility;
- signature/digest/ABI/anti-rollback/atomic replace/candidate probe/rollback;
- embedded core pin/provenance verification.

## 23. Release ledger

Агент поддерживает эту таблицу в PR description или итоговом отчёте, не
подменяя `planned` словом `done`.

| Milestone | Version/commit | CI URL | Deployment evidence | Status |
|---|---|---|---|---|
| HydraBox compatibility bridge | TBD | TBD | published app/build | planned |
| HydraCore `V_server` | TBD | TBD | release + 9 assets | planned |
| VPS server-first deploy | TBD | N/A | fleet/version/legacy4 telemetry | planned |
| HYDRA schema/config release | TBD | TBD | migration/apply/rollback | planned |
| HydraCore `V_client` | TBD | TBD | release + signed major2 bundle | planned |
| HydraBox client16 release | TBD | TBD | embedded/update activation | planned |
| 4×4 observation | versions above | dashboard/report links | start/end/sample | planned |
| Telemetry collapse | TBD | TBD | post-rollout verification | blocked by observation |

## 24. Stop conditions

Coding-агент обязан остановить публикацию/удаление, если:

1. Найден новый production consumer удаляемого field/symbol.
2. HydraBox compatibility release ещё не опубликован, а core producer уже
   готов удалить capability fields.
3. Bundle major2 принимается старым strict app.
4. Хотя бы одна VPS не подтверждена как server4/16 перед client16 rollout.
5. HYDRA собирается публиковать explicit `workers:16` до доказанной
   cross-version совместимости.
6. Remote CI не запущен или нет URL/result.
7. State migration не имеет rollback/upgrade evidence.
8. Telemetry deletion предлагается до observation evidence.
9. Release inventory содержит неожиданный файл.
10. Для исправления предлагается wire v10, negotiation, new schema/framework
    или security weakening.
11. Worktree содержит изменения неизвестного владельца, которые невозможно
    изолировать.

Остановка одного пакета не блокирует независимый read-only analysis или
подготовку другого patch, но blocked пакет нельзя объявлять закрытым.

## 25. Definition of done по уровням

### Code-complete

- Patch минимален и reviewable.
- Tests добавлены/обновлены.
- Static searches и `git diff --check` чисты.
- Локальные tests/builds не запускались.

### CI-complete

- Все relevant remote jobs green.
- Есть URLs и commit SHA.
- Artifact inventories приложены.

### Rollout-complete

- Consumer опубликован раньше producer breaking change.
- Server deployed раньше client16.
- Cross-version behavior подтверждено production evidence.
- Rollback path проверен и ещё допустим.

### Phase-complete

- Выполнены code, CI и rollout gates фазы.
- Requirement IDs и закрытые BUG IDs перечислены.
- Нет скрытых deferred consumer migrations.

## 26. Формат отчёта coding-агента

Для каждого пакета агент отвечает строго по шаблону:

```text
Пакет: R<n> — <название>
Статус: code-complete | CI-complete | rollout-complete | blocked

Изменено:
- <repo/path>: <минимальное изменение>

Удалено:
- <код/поле/asset>; новый owner: <owner>

Сохранённые инварианты:
- INV-..

Consumers:
- <producer -> parser -> behavioral use>

Tests подготовлены:
- <test/job>

Remote CI:
- <URL> — <result> — <commit SHA>

Release/deploy:
- <version, assets, fleet/app build, evidence>

Static checks:
- <rg/diff/readback results>

Worktree:
- <staged/unstaged/untracked по каждому repo>

Blocked/deferred:
- <точная причина и следующий gate>

Локальные тесты и сборки не запускались по требованию владельца.
```

Запрещено писать «всё завершено», если есть только code changes без remote CI,
consumer release, server-first deploy или observation evidence.

## 27. Минимальный commit/release порядок

Итоговая последовательность, которую нельзя сжимать в один giant PR:

1. HydraCore: R1 repair, без version/release.
2. HydraBox: R2 dual capability/bundle parser + topology-field removal.
3. Опубликовать HydraBox compatibility bridge.
4. HydraCore: R3 release pipeline/legacy AAR cleanup.
5. HydraCore: R4 server4/16, client4.
6. Опубликовать `V_server`; deploy VPS; закрыть R5 evidence.
7. HYDRA: R6 state max16 + subscription без `workers`.
8. HydraCore: R7 client16.
9. HydraCore: R8 reduced capabilities + bundle API major2.
10. Опубликовать `V_client`.
11. HydraBox: pin/embed `V_client`, release и updater verification.
12. HydraCore: R10 external-info cleanup как независимый commit.
13. Выполнить R11 BuildInfo/bundle/subscription/runtime cleanup отдельными
    consumer-first сериями.
14. Провести R12 observation; только затем telemetry collapse.
15. R13 final docs/dead-code pass.

Этот порядок намеренно оставляет две версии core: server-compatible и
client16. Release order является механизмом совместимости; новый wire,
negotiation и topology flags для этого не нужны.
