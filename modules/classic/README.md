# Classic modules (WoW 1.12.1)

Gameplay modules for `client_build: 5875` against cMaNGOS Classic (revision `8ec338a1`). Together they let an agent see nearby units, walk and fight through the gameplay JSON-RPC API. Packet formats follow the mangos-classic source and are decoded in `internal/wire`.

| Module | Requires | JSON-RPC methods | What it does |
|---|---|---|---|
| `world` | none | `world.self`, `world.units`, `world.unit`, `world.set_position` | Tracks the player and nearby units from `SMSG_UPDATE_OBJECT` (plain and compressed), `SMSG_MONSTER_MOVE`, `SMSG_DESTROY_OBJECT` and teleports. Units carry distance and a `reaction` (hostile, neutral, friendly) from the server's faction rules |
| `movement` | `world` | `movement.move_to`, `movement.face` | Walks a straight line with `MSG_MOVE_*` at run speed, with heartbeats every 500 ms on the session clock. At most 150 yards per call; no path-finding. Acknowledges forced speed changes, teleports and world ports |
| `combat` | `world`, `movement` | `combat.attack`, `combat.stop_attack`, `combat.cast`, `combat.status` | Faces, selects and swings at a unit; casts spells; logs the player's hits, swing errors, kills and experience |
| `time` | none | `time.query_time`, `time.status` | go-module-template's example for the 1.12 `SMSG_QUERY_TIME_RESPONSE` |

## Limits

- Creature `health` and `max_health` are percentages; the server sends hit points only for the player.
- `reaction` uses each faction template's default. Factions the server resolves through the player's reputation can differ.
- `move_to` interpolates height between the start and the requested `z`. Without `z` it keeps the start height. Pick points in open ground.
- The world module tracks units and players only; game objects, items and corpses are not exposed.

## Build and install

```bash
# from the repository root
sh modules/classic/build.sh                    # images localhost/agent-wow-classic-<module>:dev
for m in world movement combat time; do
  mkdir -p ~/.config/agent-wow/modules/$m
  cp modules/classic/$m/{module.yaml,compose.yaml,module.pb} ~/.config/agent-wow/modules/$m/
done
```

`generate.sh` regenerates the Go bindings and descriptor sets after a `.proto` change. It runs `protoc` inside `golang:1.27.1`.

## Podman

agent-wow runs `docker compose`. On a host with podman and podman-compose instead of Docker:

- Put a `docker` wrapper on `PATH` that runs `podman`. podman-compose has no `--project-directory`, so the wrapper must change into that directory and drop the flag.
- Set `DOCKER_HOST=unix:///run/user/$UID/podman/podman.sock` for agent-wow. The runner checks only that a Unix socket is configured.

## Development aid

`cmd/capture` logs a character in without modules and saves the gameplay packets it receives. With `AGENT_WOW_CAPTURE` pointing at the capture directory, `go test ./modules/classic/internal/wire` parses them all.
