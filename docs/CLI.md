# CLI

```
deckhand present  <deck> [--port 7777] [--ip 192.168.1.20] [--open] [--no-lan] [--hub https://…] [--slug ship-it]
deckhand validate <deck> [--quiet]
deckhand push     <deck> --hub https://… [--slug ship-it]
deckhand serve    [--addr :8080] [--pg postgres://…] [--deck-origin https://decks.…]
deckhand version
deckhand import   <file.pptx>          (stub: explains that it is planned)
```

`<deck>` is a directory, a `.zip` or a `.tar.gz` (see [FORMAT.md](FORMAT.md)).

## `present`

1. Loads and validates the deck (same rules as `validate`).
2. Starts a server on every interface, port 7777 or the next free one.
3. Prints the stage, remote and audience links and two QR codes: scan
   **REMOTE** with your phone, show **AUDIENCE** to the room.
4. `--open` opens the stage in your default browser. Press `f` there for
   fullscreen, `b` for a black screen, `q` to show the audience QR.
5. `--hub` pushes the deck to a hub and relays every state change so people
   outside the LAN can follow (milestone 4). Losing the hub only prints a
   warning; the local presentation is unaffected.

The LAN IP is auto-detected (docker, VPN, VM and link-local interfaces are
skipped). Use `--ip` when the guess is wrong, `--no-lan` to stay on
127.0.0.1.

Exit codes: 0 on Ctrl-C, 1 on an invalid deck or a port that cannot be bound.

## `validate`

Prints title, ratio, slides (● has notes, ◎ has public notes), warnings and
errors. Exit 0 when the deck can be presented, 1 otherwise. `--quiet` prints
only warnings and errors.

## Terminal output

Colours are used only when stdout is a terminal and `NO_COLOR` is unset.
