# Terminal Chess

A chess board for the terminal, written in Go with no dependencies. Play
moves yourself in standard algebraic notation, or load a PGN file and step
through famous games — then branch off to try your own ideas.

The board is drawn with Unicode block characters and 24-bit color, and pieces
slide to their squares when they move.

## Features

- Full legal-move validation: check, pins, castling, en passant, promotion,
  checkmate and stalemate (verified with [perft](https://www.chessprogramming.org/Perft_Results))
- PGN files with multiple games, comments, variations and annotations
- Step through a game move by move, or jump to the start or end
- Type a move at any point to start your own variation; rewind to where you
  branched to resume the original game
- Five color themes

## Requirements

- Go 1.26 or later
- A terminal with 24-bit color and Unicode support, at least 115 columns by
  44 rows
- Linux or macOS (uses `stty`)

## Install

```sh
go install github.com/recallog/terminal-chess@latest
```

Or build from source:

```sh
git clone https://github.com/recallog/terminal-chess.git
cd terminal-chess
go build -o terminal-chess .
```

## Usage

```sh
terminal-chess                       # new game
terminal-chess pgn/famous-games.pgn  # replay a game from a PGN file
```

When a PGN file contains several games, pick one from the list. The game
opens at its final position.

### Keys

| Key | Action |
| --- | --- |
| *type a move*, Enter | Play a move in SAN: `e4`, `Nf3`, `exd5`, `O-O`, `e8=Q` |
| Backspace | Erase a character |
| Esc | Clear the typed move (or close the help window) |
| ← / → | Previous / next move |
| Home / End | First / last move |
| F1–F5 | Switch color theme |
| ? | Show the help window |
| Ctrl-D | Quit |

## Roadmap

- **Play against a chess engine** (next step): connect to a
  [UCI](https://www.chessprogramming.org/UCI) engine such as
  [Stockfish](https://stockfishchess.org/), so you can play against the
  computer or have it suggest and evaluate moves.

## Development

```sh
go test ./...
```

All the code is in `main.go`: rendering, the move generator, the PGN parser
and the input loop. `docs/piece-design.md` shows how the pieces are drawn.

## License

[MIT](LICENSE)
