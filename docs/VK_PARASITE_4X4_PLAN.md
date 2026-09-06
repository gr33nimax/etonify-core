# План перехода `vk_parasite` на 4 звонка × 4 воркера

## 1. Цель

Расширить один логический `vk_parasite`-сеанс с четырёх KCP-линий до шестнадцати:

- четыре логических VK-звонка;
- четыре независимых воркера/KCP-линии на каждый звонок;
- шестнадцать воркеров и шестнадцать KCP-линий всего;
- один физический `net.Conn` на одну KCP-линию, как и сейчас;
- без бондинга нескольких физических соединений внутри одной KCP-линии;
- без нового wire-формата и без `wire v10`;
- с приёмом старых четырёхлинейных клиентов новым сервером во время раскатки.

Проверки и сборки выполняются только в удалённом CI. Локально их не запускать.

## 2. Зафиксированные архитектурные решения

### 2.1. Топология

Использовать шестнадцать независимых линий. Не превращать `kcpLane.worker` в список и не добавлять striping/fan-in под одной KCP-сессией.

Базовые константы:

```go
const (
    LegacyLaneCount = 4
    CallCount       = 4
    WorkersPerCall  = 4
    LaneCount       = CallCount * WorkersPerCall // 16
    MaximumLaneCount = 32
)
```

`MaximumLaneCount` нужен только как предел 32-битных in-memory масок. Не делать его пользовательской настройкой.

### 2.2. Нумерация линий

Распределять линии по звонкам вперемешку, а не непрерывными блоками:

```text
call 0: lane 0, 4, 8, 12
call 1: lane 1, 5, 9, 13
call 2: lane 2, 6, 10, 14
call 3: lane 3, 7, 11, 15
```

Формулы:

```go
callIndex := int(workerID) % CallCount
workerIndexWithinCall := int(workerID) / CallCount
```

Такой порядок сохраняет справедливый старт: сначала запускается по одному воркеру каждого звонка, затем второй воркер каждого звонка и так далее. Существующий глобальный TURN-gate продолжает разносить реальные Allocate-запросы по времени.

Для `join_links` сохранить текущий контракт «от одной до четырёх уникальных ссылок». Четыре логических слота звонков заполняются циклически:

```go
callIndex := int(workerID) % CallCount
joinLink := options.JoinLinks[callIndex%len(options.JoinLinks)]
```

При четырёх ссылках каждая получает ровно четыре воркера. При меньшем числе ссылок целые четырёхворкерные группы переиспользуют ссылки; нельзя распределять отдельные воркеры группы произвольным round-robin по ссылкам.

### 2.3. Совместимость без нового wire

Wire-кадры уже содержат:

- `WorkerID uint16`;
- `WorkerTotal uint16`;
- `laneID uint16` в reset/flow-control;
- неизменную раскладку остальных полей.

Поэтому увеличение `WorkerTotal` с 4 до 16 не меняет байтовый формат. `WorkerTotal` уже является достаточным описанием размера сессии.

Нужно:

- оставить `authProtocolVersion = 9`;
- оставить проверку байта версии в auth request/ack;
- оставить существующие magic-значения кадров;
- не добавлять `{min,max}`-переговоры;
- не добавлять новый handshake;
- не вводить `wire v10` только из-за числа линий.

Новый сервер должен принимать `WorkerTotal == 4` и `WorkerTotal == 16`, создавая тоннель нужного размера. Новый клиент всегда создаёт шестнадцатилинейную сессию. Это даёт безопасную схему «сначала сервер, затем клиенты».

### 2.4. Что удалить как лишнее

Удалить декоративную capability-модель версий, которая не участвует в согласовании:

- `WireCompatibility {Min, Max}`;
- `ProtocolSet.CallVKParasiteWire`;
- `callWireMin` и `callWireMax` из build-tag файлов;
- CI-ассерты `call_vk_parasite_wire == {"min":9,"max":9}`;
- тесты и документацию, утверждающие наличие диапазона совместимости.

Также удалить устаревшие topology-флаги:

- `CallVKEightLaneKCP` / `call_vk_eight_lane_kcp`;
- `CallVKFourLaneKCP` / `call_vk_four_lane_kcp`.

Не вводить им замену. `call_vk_parasite=true` уже сообщает о наличии единственного поддерживаемого native-режима. Из subscription fixtures и `supportedFeatures` удалить требование `call_vk_four_lane_kcp`.

Внутренний однобайтовый `authProtocolVersion` не удалять из существующего кадра: удаление байта само стало бы новым несовместимым wire-форматом. Считать его локальным маркером раскладки auth-кадра, а не версией продукта:

- не экспортировать наружу;
- не включать в capability, subscription, release manifest или CI policy;
- не менять при изменении числа линий, алгоритма recovery, pacing или иных деталей реализации;
- менять только если действительно меняется байтовая раскладка auth-кадра.

То есть wire должен быть отвязан от release coordination, но существующий байт не нужно физически вырезать ценой несовместимости.

## 3. Инварианты, которые нельзя нарушать

После изменения должны выполняться все условия:

1. Одна линия содержит не более одного активного `laneWorker`.
2. Замена воркера сохраняет текущую generation/epoch-семантику.
3. `WorkerID < WorkerTotal` проверяется на границе auth.
4. `WorkerTotal` не равен нулю и не превышает `MaximumLaneCount`.
5. Серверная политика разрешает только 4 и 16 линий, даже если парсер структурно способен разобрать другое значение.
6. Новый клиент отправляет только `WorkerTotal == 16`.
7. Все lane-ID на проводе остаются `uint16`.
8. Все наборы линий в памяти вмещают lane 15.
9. `windowDemandBits` остаётся `uint8`: это двухоконная история спроса с маской `0b11`, а не набор линий.
10. Ordered flow остаётся закреплён за одной линией; unordered/flowlet traffic может использовать все шестнадцать.
11. Сервер может одновременно держать разные сессии с `WorkerTotal == 4` и `WorkerTotal == 16`.
12. Клиент v9/4 продолжает работать с новым сервером; клиент v9/16 требует новый сервер.

## 4. Пакет работ A — сделать размер тоннеля свойством сессии

Файл: `transport/call/vk-parasite/lane_tunnel.go`.

### A1. Конструктор

Передать число линий во внутренний конструктор:

```go
func newParasiteTunnel(
    seed uint32,
    laneCount uint16,
    log logger.ContextLogger,
    metrics *telemetry.Accumulator,
) (*ParasiteTunnel, error)
```

Проверить в одном месте:

- `seed != 0`;
- `laneCount > 0`;
- `laneCount <= MaximumLaneCount`.

Не добавлять `TunnelOptions`, builder, factory или интерфейс. Для одного числа достаточно аргумента.

### A2. Динамические контейнеры

Заменить:

```go
lanes               [LaneCount]*kcpLane
recoveryProgress    [LaneCount]int64
recoverySuggestedAt [LaneCount]time.Time
```

на срезы длины `laneCount`:

```go
lanes               []*kcpLane
recoveryProgress    []int64
recoverySuggestedAt []time.Time
```

Создать их в конструкторе. Не хранить вторую копию `laneCount` в структуре: источник истины — `len(t.lanes)`.

Добавить один маленький метод только если он реально сокращает повторяющиеся приведения:

```go
func (t *ParasiteTunnel) laneCount() uint16 {
    return uint16(len(t.lanes))
}
```

### A3. Все обращения к глобальному `LaneCount`

В методах `ParasiteTunnel` и `kcpLane` заменить глобальный предел на размер конкретного тоннеля:

- bounds checks в `telemetryWorker`, `reserveWorkerGeneration`, `DropWorker`, reset/probe/recovery methods, `WorkerEpoch`, `workerReadyAfter`, `LaneGeneration`;
- циклы выбора и перебора линий;
- modulo в `trySendEncoded`;
- сравнение `ActiveWorkers()` для `fullyAttached`;
- агрегаты telemetry;
- `TransportHealthSnapshot.TotalLanes`;
- очистку recovery slices в `Close`.

Не делать механическую замену `LaneCount` во всём файле. Константа по-прежнему нужна клиентской топологии и тестам; внутри уже созданного тоннеля нужен `len(t.lanes)`.

### A4. Вызовы конструктора

- `client.go`: создавать тоннель с `LaneCount` (16).
- `server.go`: создавать тоннель с `request.WorkerTotal`.
- тестовые helper/construction sites: явно выбирать 4 или 16 по смыслу теста.

Большую часть существующих низкоуровневых recovery/KCP-тестов можно оставить четырёхлинейными, чтобы удалённый CI не стал в четыре раза тяжелее. Добавить отдельный узкий набор шестнадцатилинейных тестов для новой ёмкости.

## 5. Пакет работ B — расширить только настоящие lane-маски

Файлы:

- `transport/call/vk-parasite/lane_tunnel.go`;
- `transport/call/vk-parasite/flow_migration.go`;
- связанные тесты.

Изменить `uint8` на `uint32` только у:

- `sendFlowState.laneMask`;
- `ParasiteTunnel.recoveryDeferred`;
- `ParasiteTunnel.recoveryPending`.

Каждый бит строить явно:

```go
bit := uint32(1) << laneID
```

Исправить все операции установки, проверки и очистки битов в send-flow, migration и recovery paths.

Не менять:

- `kcpLane.windowDemandBits`;
- `laneRecoveryResult`;
- `recoveryLastOutcome`;
- другие `uint8`, являющиеся enum, счётчиком или двухбитной историей.

Не вводить общий bitset-пакет или generic-абстракцию. Трёх полей `uint32` достаточно до явно зафиксированного потолка 32.

## 6. Пакет работ C — отвязать auth-парсер от локальной константы

Файл: `transport/call/vk-parasite/auth.go`.

### C1. Структурная валидация

Вынести общую минимальную проверку request metadata, используемую encode и decode:

```go
func validWorkerMetadata(request authRequest) bool {
    return request.Conv != 0 &&
        request.WorkerTotal > 0 &&
        request.WorkerTotal <= MaximumLaneCount &&
        request.WorkerID < request.WorkerTotal
}
```

Здесь не проверять конкретное множество `{4,16}`. Auth-парсер проверяет корректность кадра, а серверная политика — поддерживаемую топологию.

### C2. Версия

Оставить `authProtocolVersion = 9`, `frame[4]` и equality checks без изменений.

Не использовать поле `ProtocolVersion` для переговоров. Оно может продолжать отражать прочитанный байт для диагностики.

## 7. Пакет работ D — клиент 4 × 4

Файл: `transport/call/vk-parasite/client.go`.

### D1. Нормализация опций

- default `Workers` сделать равным 16;
- новый клиент должен принимать только `Workers == 16`;
- текст ошибки должен говорить «exactly sixteen VK lanes (four workers per call) are required»;
- диапазон `join_links` оставить 1..4;
- не добавлять `workers_per_call` в JSON/API: значение сейчас является частью единственной продуктовой топологии, а не пользовательским выбором.

Существующее поле `Workers` пока оставить для совместимости конфигурационного API. Не расширять эту задачу до удаления публичных option-полей.

### D2. Привязка воркера к звонку

Заменить прямое `workerID % len(JoinLinks)` на явную двухступенчатую привязку через четыре call slots:

```go
callIndex := workerID % CallCount
joinLink := options.JoinLinks[callIndex%len(options.JoinLinks)]
```

Вынести формулу в небольшой pure helper и тестировать таблицей. Не создавать `CallGroup`/`WorkerGroup`: runtime-состояния группы не требуется.

### D3. Старт и readiness

Оставить текущий цикл запуска воркеров, `readyOnce` и правило «клиент готов после первого подключённого воркера». Интерливированная нумерация сама обеспечивает справедливый порядок групп.

Не ждать подключения всех 16 воркеров перед готовностью — это увеличит startup latency и сделает квотную ошибку одного воркера фатальной для всей сессии.

## 8. Пакет работ E — сервер 4/16 без flag-day

Файл: `transport/call/vk-parasite/server.go`.

### E1. Лимиты

- `HardMaxWorkers = LaneCount` (16);
- `defaultMaxWorkers = LaneCount` (16);
- `MaxWorkersPerSession` трактовать как верхний предел, а не как точное равенство локальной константе;
- допустимые session totals: 4 и 16;
- если конфиг задаёт максимум 4, принимать только legacy-сессии;
- если максимум 16, принимать и 4, и 16.

Добавить один policy helper:

```go
func supportedSessionLaneCount(total uint16) bool {
    return total == LegacyLaneCount || total == LaneCount
}
```

Не принимать промежуточные 5..15 значения: они не соответствуют ни старой, ни новой проверенной топологии.

### E2. Создание сессии

В `getOrCreateSession` создавать `ParasiteTunnel` с `request.WorkerTotal`.

Существующая проверка:

```go
session.expected == request.WorkerTotal
```

должна остаться. Она не даёт воркерам одной session ID подменить размер тоннеля после создания.

### E3. Граница доверия

До создания/поиска сессии проверить:

- `supportedSessionLaneCount(request.WorkerTotal)`;
- `int(request.WorkerTotal) <= MaxWorkersPerSession`;
- `request.WorkerID < request.WorkerTotal` уже гарантирован auth decoder, но сервер не должен полагаться на значение больше своей политики.

## 9. Пакет работ F — recovery, health и pacing

### F1. Quarantine threshold

Заменить абсолютное `quarantinedLaneCount() >= 3` на сохранение прежней доли 3/4:

```go
threshold := (3*len(t.lanes) + 3) / 4 // ceil(75%)
```

Для 4 линий порог остаётся 3, для 16 становится 12. Reason/event переименовать из `three_quarantined_lanes` в нейтральное `quarantined_lane_quorum`.

Не добавлять конфиг для этого порога до появления измеренной необходимости.

### F2. Health

В `supervisor.go` определять healthy-состояние сравнением:

```go
health.ActiveLanes == health.TotalLanes
```

а не с package-level `LaneCount`. Snapshot уже несёт фактическое число линий конкретного тоннеля.

### F3. Pacing

Текущий cold start равен примерно 2 Mbit/s на линию, то есть 8 Mbit/s на четыре линии. При 16 линиях неизменная константа дала бы около 32 Mbit/s стартового агрегата и синхронный удар по TURN/VK policer.

Для первого релиза использовать существующий minimum как новый initial:

```go
lanePacingInitialBPS = lanePacingMinimumBPS
```

Это сохраняет агрегатный cold start примерно на прежнем уровне, после чего каждая линия растёт через существующий ACK-clocked probing.

Не добавлять общий per-call pacer в этой задаче. Интерливированные lane IDs уже разнесут probe offsets одного звонка примерно на одну секунду внутри четырёхсекундного интервала.

Максимум, gains и probe state сначала оставить прежними. Их менять только по результатам удалённого 16-lane load test и telemetry, а не по предположению.

## 10. Пакет работ G — TURN и VK-креды

Не копировать `WorkerGroup` из qWDTT.

В hydracore уже есть всё необходимое:

- credential cache по `joinLink`;
- `singleflight` по `joinLink`;
- debounced invalidation;
- глобальный fetch gate;
- глобальный TURN gate;
- `turnAllocationSpacing = 250ms`;
- DTLS/recovery gates.

Поэтому в основной реализации не добавлять:

- второй credential cache;
- group-level mutex;
- отдельный allocate ticker;
- новый dispatcher;
- новую сущность `WorkerGroup`.

Quota-aware обработку TURN 486 вынести в последующий change только если удалённая telemetry подтвердит массовые 486. Текущий exponential reconnect backoff и глобальный gate уже дают безопасное базовое поведение. Не распознавать 486 через строки ошибок в рамках этой задачи.

## 11. Пакет работ H — capability и документационный cleanup

### H1. `common/hydracore`

В `capabilities.go` удалить:

- `CallVKEightLaneKCP`;
- `CallVKFourLaneKCP`;
- `CallVKParasiteWire`;
- `WireCompatibility`.

В `call_client.go`, `call_server.go`, `call_enabled.go`, `call_disabled.go` удалить `callWireMin`/`callWireMax`.

Не добавлять scalar capability версии: реальных runtime-потребителей нет, а auth сам отклоняет несовместимый формат.

### H2. Subscription requirements

В `experimental/libbox/hydracore_subscription.go` удалить `call_vk_four_lane_kcp` из `supportedFeatures`.

В subscription test fixtures удалить его из `requirements.core.features`. Не добавлять новый topology feature: `call_vk_parasite` достаточно.

Внешний генератор HYDRA-ULTIMATE сейчас действительно выдаёт `call_vk_four_lane_kcp`; удалить это требование одновременно. Не переносить туда `wire`, `lane_count`, `workers_per_call`, recovery-флаги или другие детали реализации.

### H3. Внешний runtime gate

В HYDRA-ULTIMATE `hydra/contracts/hydracore_calls.py` сейчас требует точный `wire == 9` и длинный список внутренних milestone-флагов. Заменить его минимальной проверкой:

- `identity.core_id == "io.hydrabox.hydracore"`;
- `identity.role == "vps"`;
- `features.call_vk_parasite == true`;
- `protocols.call_modes` содержит `vk_parasite`.

Не проверять topology, wire, hot-swap, flow migration, recovery, telemetry и иные детали одной атомарно поставляемой реализации. Фактическую совместимость активного конфига продолжает проверять `sing-box check`; несовместимый auth-кадр отклоняет сам transport.

### H4. Release artifacts

Не публиковать subscription-документацию как binary release assets. Сейчас `.github/workflows/hydracore.yml` сам:

1. копирует `HYDRA_SUBSCRIPTION_V2.md` и две schema в `dist`;
2. создаёт для них `.sha256`;
3. включает их в Android provenance;
4. загружает весь `dist/*` в GitHub Release.

Удалить эти copy/hash/provenance/verify шаги. Контракт может оставаться версионированным исходником в репозитории, пока subscription subsystem ещё существует; отдельный release asset для него не нужен.

Заменить `gh release upload "$version" dist/*` явным allowlist runtime-артефактов. Иначе любой временный файл из `dist` автоматически становится публичным API релиза.

Не публиковать отдельные `.sha256`, если потребитель уже проверяет GitHub `asset.digest`. Не удалять Ed25519-подпись независимо обновляемого Android bundle без проверки реального Android consumer: это security boundary, в отличие от дублирующих checksum/provenance файлов.

### H5. Следующий отдельный cleanup subscription ownership

Не смешивать полный перенос subscription subsystem с изменением 4→16. После него сделать отдельный deletion-first change:

- HydraBox/HYDRA-ULTIMATE владеют envelope, fetch, JWE, schema и profile selection;
- HydraCore получает конечный sing-box config и сохраняет строгую `remote_v2` validation как security boundary;
- из core удаляются `contract/subscription`, `hydracore_subscription*.go` и их libbox accessors только после миграции Android consumer;
- capability-документ сокращается до полей, по которым отдельный consumer реально ветвится; compile-time implementation milestones удаляются;
- `hydracore-client-capabilities.json` и `capabilitiesSha256` удаляются из bundle, если Android updater их не читает.

Не создавать v3-контракт, replacement registry или новую negotiation-схему. Цель — убрать второй слой политики, а не переименовать его.

### H6. Документация

Обновить `HYDRACORE.md`:

- четыре звонка, четыре линии на звонок, шестнадцать линий всего;
- таблица mapping lane → call;
- `workers` default/exact = 16 для нового клиента;
- server max default = 16;
- новый сервер принимает legacy total=4;
- rollout server-first;
- убрать утверждение «frozen at 4 for wire-v9 contract»;
- убрать fake `{min,max}` capability;
- пояснить, что wire version меняется только при изменении формата кадра.

В `docs/CALL_VK_TELEMETRY.md` заменить формулировки, без необходимости привязывающие telemetry к номеру wire, на «authenticated control frames». Сам формат кадров не менять.

Исторические release notes не переписывать. Добавить новую запись в changelog/release notes текущего релиза.

## 12. Удалённые тесты, которые обязан добавить агент

Локально тесты не запускать. Реализовать тесты и передать их в CI.

### 12.1. Topology mapping

Table-driven test для 1, 2, 3 и 4 `join_links`:

- каждая из 16 линий получает ожидаемый call slot;
- каждый call slot содержит ровно четыре line ID;
- при четырёх ссылках каждая ссылка получает четыре воркера;
- при трёх ссылках целая группа call 3 переиспользует link 0, а не размазывается по ссылкам.

### 12.2. Auth metadata

Проверить:

- encode/decode total=4;
- encode/decode total=16 и workerID=15;
- reject total=0;
- reject total>32;
- reject workerID>=WorkerTotal;
- reject неверного auth version byte;
- байтовая длина и offsets auth frame не изменились.

### 12.3. Dynamic tunnel

Отдельные проверки конструктора на 4 и 16 линий:

- правильное число lanes;
- уникальные/non-zero KCP conversations;
- lane 15 принимает worker;
- lane 16 отклоняется в 16-lane tunnel;
- telemetry `TotalLanes` и capacities используют фактический размер.

### 12.4. 32-bit masks

Проверить верхние биты:

- ordered flow может быть pinned к lane 15;
- unordered flow может накопить биты всех 16 линий;
- migration на lane 15 сохраняет маску;
- recovery pending/deferred корректно ставит и очищает bit 15;
- операции над lane 15 не обнуляют биты 0..14.

### 12.5. Recovery quorum

Для четырёхлинейного тоннеля подтвердить старую границу 3. Для шестнадцатилинейного:

- 11 quarantined lanes не вызывают quorum replacement;
- 12 достигают нового quorum;
- aggregate no-progress deadline продолжает защищать от преждевременного закрытия там, где это предусмотрено state machine.

### 12.6. Server compatibility

Проверить на одном новом сервере:

- legacy session total=4 создаёт четырёхлинейный tunnel;
- new session total=16 создаёт шестнадцатилинейный tunnel;
- обе сессии могут существовать одновременно;
- total=8 отклоняется как неподдерживаемая topology;
- max=4 отклоняет total=16;
- max=16 принимает total=4 и total=16;
- второй worker той же session ID с другим `WorkerTotal` отклоняется.

### 12.7. End-to-end package tests

Добавить один 16-lane in-memory tunnel pair test:

- подключить все 16 пар;
- убедиться в `ActiveWorkers()==16` с обеих сторон;
- отправить ordered и unordered traffic;
- заменить worker lane 15 по epoch/generation;
- убедиться, что logical session остаётся живой.

Не дублировать весь существующий четырёхлинейный suite для 16 линий.

## 13. Удалённая CI-матрица

Агент должен подготовить изменения для следующих удалённых jobs:

1. `go test ./transport/call/vk-parasite`.
2. `go test ./transport/call/vk`.
3. `go test ./experimental/libbox` для relevant build tags/roles.
4. Role-specific client/server compile/test jobs без exact wire и implementation-milestone assertions; generic `with_call` job удалить, если client+server jobs уже покрывают те же packages.
5. Race job для `vk-parasite`, если он уже есть в CI; новый локальный job не добавлять без необходимости.
6. Детерминированный 16-lane emulator/load test с разумным CI timeout.

Не запускать отдельный `TestWireV9` job, если тот же тест уже выполняется обычным package test без специальных условий. CI должен отдельно публиковать фактическое время и peak memory 16-lane test. Не задавать pass/fail-порог производительности до появления baseline; correctness gate достаточно для первого PR.

## 14. Порядок реализации для AI-кодера

Выполнять строго небольшими логическими коммитами:

1. Параметризовать размер `ParasiteTunnel`, не меняя production default.
2. Перевести три настоящие lane-маски на `uint32`.
3. Добавить auth structural validation и server policy `{4,16}`.
4. Перевести production client на 16 и внедрить call-slot mapping.
5. Исправить recovery quorum, health и conservative pacing initial rate.
6. Удалить capability range и stale four/eight-lane flags вместе с exact gate в HYDRA-ULTIMATE.
7. Убрать subscription docs/schema из release assets и заменить upload glob allowlist-ом.
8. Обновить документацию и remote CI expectations.

Полный перенос Hydra Subscription из core выполнять отдельным change после обновления Android consumer; он не должен блокировать транспортный rollout.

После каждого шага агент обязан просмотреть diff и сделать repo-wide `rg` по удалённым/переименованным символам. Тесты и сборку локально не запускать.

## 15. Rollout

### Этап 1 — сервер

Развернуть новый сервер первым:

- wire byte остаётся 9;
- сервер принимает total=4 и total=16;
- старые клиенты продолжают создавать total=4 sessions.

До клиентского rollout проверить server telemetry и отсутствие регрессии legacy sessions.

### Этап 2 — клиент

Развернуть клиент с total=16:

- четыре call slots;
- четыре воркера на slot;
- старый сервер такой клиент не поддерживает, поэтому порядок rollout обязателен.

### Этап 3 — наблюдение

Смотреть удалённую telemetry:

- active/usable lanes;
- TURN allocate failure rate;
- VK credential fetch/cache-hit rate;
- recovery/quarantine counts;
- aggregate goodput;
- RTT/retransmit/admission pressure;
- session replacement reasons;
- CPU, goroutines и memory на клиенте и VPS.

Если появляются массовые TURN 486, отдельным минимальным change добавить typed quota classification и 30–60s jittered backoff. До данных эту ветку не реализовывать.

## 16. Критерии готовности

Изменение готово, когда удалённый CI и review подтверждают:

- новый клиент создаёт 16 независимых KCP-линий;
- каждые четыре линии относятся к одному из четырёх call slots;
- четыре `join_links` получают по четыре воркера;
- новый сервер принимает одновременно legacy 4 и new 16 sessions;
- wire byte и frame layouts не изменились;
- capability `{min,max}` и stale four/eight-lane flags удалены;
- HYDRA-ULTIMATE не сверяет wire или внутренние recovery milestone-флаги;
- subscription `.md`/schema и их checksum не публикуются как release assets;
- release upload использует явный allowlist вместо `dist/*`;
- lane 15 проходит send, migration и recovery paths;
- `windowDemandBits` остался двухбитной историей;
- health/telemetry используют фактический размер тоннеля;
- cold-start aggregate не вырос автоматически в четыре раза;
- не добавлены negotiation, `workers_per_call`, `WorkerGroup`, второй cache/gate или иные сущности «на будущее».

## 17. Явные запреты для AI-кодера

Не делать в этом изменении:

- `wire v10`;
- min/max negotiation;
- поддержку произвольных 1..32 lane counts на сервере;
- пользовательский `workers_per_call`;
- bonding четырёх transport connections под одной KCP lane;
- новый общий per-call congestion controller;
- копию qWDTT dispatcher/group logic;
- разбор TURN quota по строке ошибки;
- полную переработку `lane_tunnel.go` на новые интерфейсы/пакеты;
- массовый rewrite существующих тестов;
- новый subscription contract вместо удаления лишнего слоя;
- удаление bundle signature без подтверждения Android update path;
- локальный запуск тестов или сборок.

Сначала нужен минимальный корректный переход 4 → 16 с server-side legacy compatibility. Любая следующая абстракция должна быть обоснована данными удалённой эксплуатации.
