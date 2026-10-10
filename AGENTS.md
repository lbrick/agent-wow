# Instructions for AI coding agents

- Do not edit README.md or any docs unless explicitly instructed.
- The client speaks two protocols, selected by `client_build` (`pkg/protocol`):
    - 12340 (WoW 3.3.5a) against AzerothCore. Refer to https://github.com/azerothcore/azerothcore-wotlk (revision `d80ce1d8`).
    - 5875 (WoW 1.12.1) against cMaNGOS Classic. Refer to https://github.com/cmangos/mangos-classic (revision `8ec338a1`).
    - A change to a shared code path must keep both builds' tests passing.
- The environment may have a running AzerothCore or cMaNGOS server (authserver/realmd and worldserver/mangosd) that can be used for end to end testing.
    - You are only allowed to interact with it via the public APIs (just like any client server model).
    - Do not change any of its internals (e.g. server configurations, database schemas or records, source code, etc).
    - Do not send malformed or experimental packets to a shared server that other players use; test those against a local one.
