# Piece design

Each piece is drawn in a 7×3 area with Unicode [Block Elements](https://en.wikipedia.org/wiki/Block_Elements)
(U+2580–U+259F), plus `✚` (U+271A) for the king's cross. The designs are defined in
`pieceDesign()` in `main.go`; this page shows them exactly as drawn.

## Pieces

White and black pieces share the same shapes; only the color differs.

```
  King     Queen      Rook     Bishop    Knight     Pawn
 ▞▀✚▀▚     ▐▃▀▃▌     ▙█▄█▄▌    ▞▜▂▛▚      ▟▓▙        ▃
 ▚▄█▄▞      ▟▒▙      ▐▓▚▇▊      ▓██      ▟▇▓▌▚      ▟█▙
 ▂▅▇▅▂     ▟▇▇▇▙     ▟▓▇▚▇     ▟▇▄▇▙      ▄▇▇▞      ▚▄▞
```

## Colors

Each of the three rows of a piece gets its own color, giving a top-to-bottom gradient:

| Side | Top row | Bottom row |
| --- | --- | --- |
| White | silk `rgb(255, 255, 250)` | pearl `rgb(190, 185, 175)` |
| Black | bright yellow `rgb(255, 255, 120)` | deep gold `rgb(230, 160, 20)` |

Pieces are drawn in bold, over the square's background color.

## Squares

A square is 9 columns by 5 rows (`cellW`, `cellH`). The piece is centered in it,
leaving one column on each side and one row above and below:

```
 ┌─────────┐
 │         │
 │   ▟▓▙   │
 │  ▟▇▓▌▚  │
 │   ▄▇▇▞  │
 │         │
 └─────────┘
```

Square colors come from the current theme (F1–F5): each theme blends a dark
start color into a bright end color diagonally across the board, with light and
dark squares taking alternate halves of that blend (`tileColor()`).
