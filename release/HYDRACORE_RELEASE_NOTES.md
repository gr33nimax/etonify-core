# HydraCore debug release notes

This prerelease ships the protocol-v10 `vk_parasite` transport: QUIC over four
required VK/TURN paths, with four paths by default and up to twenty workers in
multiples of four.

Protocol 10 removed the DTLS layer. It ran underneath the RTP-shaped wrapper, so
every byte of it travelled inside the sealed payload and was never visible to an
observer on the path, while costing 37 bytes and one AES-GCM pass per packet in
each direction. Worker authentication is now the first stream of each QUIC
connection, and the VPS serves every worker from one QUIC listener on the shared
UDP socket, telling connections apart by their QUIC connection ID.

A client older than protocol 10 cannot talk to a protocol 10 VPS at all. Client
and VPS must come from the same release manifest and source commit.

Transport health is part of the typed runtime stream. Reports carry the outbound
tag and runtime generation; material state, challenge, lane, and failure changes
wake the existing stream without JSON polling across JNI.

debug.59 carries the runtime hardening from the September audit round. Workers
survive a network rebind that lands while their reconnect sits in backoff, a
dial completed for an old network generation is rejected instead of used, and
the first path failure no longer ends startup while other initial attempts are
still in flight. Cached TURN credentials are refreshed only after a confirmed
authentication rejection, and join credentials never reach the ordinary log.

Three client-facing abilities are new behind capability flags, so an older
client paired with this core keeps its own behaviour: a DoH resolver keeps its
query string (`dns_query`), the automatic `urltest` group honours the client's
probe timeout and concurrency (`urltest_probe_budget`), and the TURN edge a
transport last reached is readable across processes for a workerless
reachability probe (`turn_edge_endpoint`). `SetLogLevel` understands `off` as
its own instruction — every factory, including one that started at DEBUG, can
be released at runtime and built again.

The release contains separate Android client and Linux VPS runtimes. The VPS
advertises `call_vk_parasite_server`; the client advertises
`call_vk_parasite_client`; both advertise `call_vk_parasite_quic`.

CI verifies every release before publication. Assets are the Android AAR and
sources, three Android shared libraries, two Linux archives, and a signed bundle
manifest.
