// Terminal Chess is a chess board for the terminal: play moves in standard
// algebraic notation or replay games from a PGN file.
package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

type RGB struct{ R, G, B int }

func (c RGB) fg() string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B) }
func (c RGB) bg() string { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c.R, c.G, c.B) }

const (
	reset       = "\x1b[0m"
	bold        = "\x1b[1m"
	clearScreen = "\x1b[2J\x1b[H"
	// With autowrap off, lines wider than the terminal are clipped instead
	// of spilling onto the next row and shifting the board.
	wrapOff = "\x1b[?7l"
	wrapOn  = "\x1b[?7h"
)

// Board layout, in terminal cells.
const (
	cellW  = 9
	cellH  = 5
	labelW = 4
	boardW = labelW + 1 + 8*cellW
	boardH = 1 + 8*cellH + 1
	gutter = "   " // between the board and the side panel
)

var (
	labelFg = RGB{140, 140, 150}

	// White pieces: silk to pearl
	whiteTop    = RGB{255, 255, 250}
	whiteBottom = RGB{190, 185, 175}

	// Black pieces are drawn in gold: bright yellow to deep gold
	blackTop    = RGB{255, 255, 120}
	blackBottom = RGB{230, 160, 20}
)

func lerp(a, b RGB, t float64) RGB {
	return RGB{
		R: int(float64(a.R)*(1-t) + float64(b.R)*t),
		G: int(float64(a.G)*(1-t) + float64(b.G)*t),
		B: int(float64(a.B)*(1-t) + float64(b.B)*t),
	}
}

type Piece int

const (
	Empty Piece = iota
	WhiteKing
	WhiteQueen
	WhiteRook
	WhiteBishop
	WhiteKnight
	WhitePawn
	BlackKing
	BlackQueen
	BlackRook
	BlackBishop
	BlackKnight
	BlackPawn
)

func (p Piece) glyph() string {
	switch p {
	case WhiteKing:
		return "K"
	case WhiteQueen:
		return "Q"
	case WhiteRook:
		return "R"
	case WhiteBishop:
		return "B"
	case WhiteKnight:
		return "N"
	case WhitePawn:
		return "P"
	case BlackKing:
		return "k"
	case BlackQueen:
		return "q"
	case BlackRook:
		return "r"
	case BlackBishop:
		return "b"
	case BlackKnight:
		return "n"
	case BlackPawn:
		return "p"
	default:
		return "."
	}
}

func (p Piece) isWhite() bool { return p >= WhiteKing && p <= WhitePawn }
func (p Piece) isBlack() bool { return p >= BlackKing && p <= BlackPawn }

// is reports whether p belongs to side.
func (p Piece) is(side color) bool {
	if side == white {
		return p.isWhite()
	}
	return p.isBlack()
}

// kind is the uppercase SAN letter of the piece ('P' for pawns, '.' for Empty).
func (p Piece) kind() byte { return strings.ToUpper(p.glyph())[0] }

// pieceOf returns the piece of the given SAN letter (KQRBNP) and side.
func pieceOf(kind byte, side color) Piece {
	p := Empty
	switch kind {
	case 'K':
		p = WhiteKing
	case 'Q':
		p = WhiteQueen
	case 'R':
		p = WhiteRook
	case 'B':
		p = WhiteBishop
	case 'N':
		p = WhiteKnight
	case 'P':
		p = WhitePawn
	default:
		return Empty
	}
	if side == black {
		p += BlackKing - WhiteKing
	}
	return p
}

type design [3]string

func pieceDesign(p Piece) design {
	switch p {
	case WhitePawn, BlackPawn:
		return design{"   ▃   ", "  ▟█▙  ", "  ▚▄▞  "}
	case WhiteKnight, BlackKnight:
		return design{"  ▟▓▙  ", " ▟▇▓▌▚ ", "  ▄▇▇▞ "}
	case WhiteBishop, BlackBishop:
		return design{" ▞▜▂▛▚ ", "  ▓██  ", " ▟▇▄▇▙ "}
	case WhiteRook, BlackRook:
		return design{" ▙█▄█▄▌", " ▐▓▚▇▊ ", " ▟▓▇▚▇ "}
	case WhiteQueen, BlackQueen:
		return design{" ▐▃▀▃▌ ", "  ▟▒▙  ", " ▟▇▇▇▙ "}
	case WhiteKing, BlackKing:
		return design{" ▞▀✚▀▚ ", " ▚▄█▄▞ ", " ▂▅▇▅▂ "}
	default:
		return design{}
	}
}

func pieceColor(p Piece, row int) RGB {
	t := float64(row) / 2.0
	switch {
	case p.isWhite():
		// Subtle vertical gradient
		return lerp(whiteTop, whiteBottom, t)
	case p.isBlack():
		// Stronger contrast for black pieces
		return lerp(blackTop, blackBottom, t)
	default:
		return RGB{}
	}
}

type Theme struct {
	Name       string
	StartColor RGB
	EndColor   RGB
}

var themes = []Theme{
	{"Original", RGB{15, 15, 50}, RGB{220, 70, 140}},
	{"Greenish", RGB{10, 40, 10}, RGB{80, 180, 80}},
	{"Blueish", RGB{10, 10, 60}, RGB{100, 150, 255}},
	{"Amberish", RGB{40, 20, 0}, RGB{255, 180, 50}},
	{"Monochrome", RGB{10, 10, 10}, RGB{180, 180, 180}},
}

var currentThemeIdx = 0

func tileColor(r, c int) RGB {
	theme := themes[currentThemeIdx]
	t := float64(r+c) / 14.0
	midColor := lerp(theme.StartColor, theme.EndColor, 0.5)
	if (r+c)%2 == 0 {
		return lerp(theme.StartColor, midColor, t)
	}
	return lerp(midColor, theme.EndColor, t)
}

type color bool

const (
	white color = true
	black color = false
)

func (c color) String() string {
	if c == white {
		return "White"
	}
	return "Black"
}

// Indices into game.castling.
const (
	castleWK = iota // white kingside
	castleWQ        // white queenside
	castleBK        // black kingside
	castleBQ        // black queenside
)

// castleIdx returns the castling-rights index for side, kingside or queenside.
func castleIdx(side color, kingside bool) int {
	idx := castleWK
	if side == black {
		idx = castleBK
	}
	if !kingside {
		idx++
	}
	return idx
}

// homeRow is the back rank of side in board coordinates.
func homeRow(side color) int {
	if side == white {
		return 7
	}
	return 0
}

type game struct {
	Board      [8][8]Piece
	Turn       color
	MoveNum    int
	History    []string
	Moves      []string // Raw moves for navigation
	PGN        []string // Moves loaded from the PGN file; nil if none
	HistoryIdx int      // Current position in history
	Tags       map[string]string
	Error      string
	lastMove   struct{ from, to [2]int }
	epFile     int
	castling   [4]bool
}

// move is a fully resolved move. Castling is encoded as the king's two-square move.
type move struct {
	fr, fc, tr, tc int
	promo          Piece // Empty unless the move is a pawn promotion
	castle         bool
	ep             bool // en passant capture; the captured pawn is on [fr][tc]
}

func (g *game) reset() {
	g.Board = [8][8]Piece{
		{BlackRook, BlackKnight, BlackBishop, BlackQueen, BlackKing, BlackBishop, BlackKnight, BlackRook},
		{BlackPawn, BlackPawn, BlackPawn, BlackPawn, BlackPawn, BlackPawn, BlackPawn, BlackPawn},
		{Empty, Empty, Empty, Empty, Empty, Empty, Empty, Empty},
		{Empty, Empty, Empty, Empty, Empty, Empty, Empty, Empty},
		{Empty, Empty, Empty, Empty, Empty, Empty, Empty, Empty},
		{Empty, Empty, Empty, Empty, Empty, Empty, Empty, Empty},
		{WhitePawn, WhitePawn, WhitePawn, WhitePawn, WhitePawn, WhitePawn, WhitePawn, WhitePawn},
		{WhiteRook, WhiteKnight, WhiteBishop, WhiteQueen, WhiteKing, WhiteBishop, WhiteKnight, WhiteRook},
	}
	g.Turn = white
	g.MoveNum = 1
	g.History = nil
	g.HistoryIdx = 0
	g.epFile = -1
	g.castling = [4]bool{true, true, true, true}
	g.lastMove.from = [2]int{-1, -1}
	g.lastMove.to = [2]int{-1, -1}
}

func newGame() *game {
	g := &game{}
	g.reset()
	return g
}

// goToMove replays g.Moves[:idx] from the starting position. If a move fails,
// the replay stops just before it and the error names the offending move.
// Rewinding to (or before) the point where a typed variation left the PGN
// drops the variation and resumes the PGN.
func (g *game) goToMove(idx int) error {
	if g.inVariation() && idx <= g.branchIdx() {
		g.Moves = g.PGN
	}
	idx = max(0, min(idx, len(g.Moves)))

	g.reset()
	for i, m := range g.Moves[:idx] {
		if err := g.play(m); err != nil {
			return fmt.Errorf("move %d (%s): %w", i+1, m, err)
		}
		g.HistoryIdx = i + 1
	}
	return nil
}

// branchIdx is the number of leading moves g.Moves shares with the PGN.
func (g *game) branchIdx() int {
	n := 0
	for n < len(g.Moves) && n < len(g.PGN) && g.Moves[n] == g.PGN[n] {
		n++
	}
	return n
}

// inVariation reports whether moves typed by the user replaced or extended
// the loaded PGN.
func (g *game) inVariation() bool {
	return g.PGN != nil && g.branchIdx() < len(g.Moves)
}

// playTyped plays a move the user typed at the current position, dropping any
// moves after it. g.Moves is copied so the PGN is never overwritten.
func (g *game) playTyped(m move, notation string) {
	g.Moves = append(slices.Clone(g.Moves[:g.HistoryIdx]), notation)
	g.playMove(m, notation)
	g.HistoryIdx = len(g.Moves)
}

// play resolves and applies notation, records it, and passes the turn.
func (g *game) play(notation string) error {
	m, err := g.resolveMove(notation)
	if err != nil {
		return err
	}
	g.playMove(m, notation)
	return nil
}

// playMove applies an already resolved move, records it, and passes the turn.
func (g *game) playMove(m move, notation string) {
	g.applyMove(m)
	prefix := ""
	if g.Turn == white {
		prefix = fmt.Sprintf("%d.", g.MoveNum)
	} else {
		g.MoveNum++
	}
	g.History = append(g.History, prefix+notation)
	g.Turn = !g.Turn
}

// executeMove resolves and applies notation for g.Turn. It does not pass the turn.
func (g *game) executeMove(notation string) error {
	m, err := g.resolveMove(notation)
	if err != nil {
		return err
	}
	g.applyMove(m)
	return nil
}

// applyMove updates the board, castling rights and en passant state. m must be legal.
func (g *game) applyMove(m move) {
	p := g.Board[m.fr][m.fc]
	g.Board[m.tr][m.tc] = p
	g.Board[m.fr][m.fc] = Empty
	if m.ep {
		g.Board[m.fr][m.tc] = Empty
	}
	if m.promo != Empty {
		g.Board[m.tr][m.tc] = m.promo
	}
	if m.castle {
		if m.tc == 6 {
			g.Board[m.tr][5], g.Board[m.tr][7] = g.Board[m.tr][7], Empty
		} else {
			g.Board[m.tr][3], g.Board[m.tr][0] = g.Board[m.tr][0], Empty
		}
	}

	// Castling rights are lost when the king moves, or when anything moves
	// from or to a rook's home corner (the rook moved or was captured).
	switch p {
	case WhiteKing:
		g.castling[castleWK], g.castling[castleWQ] = false, false
	case BlackKing:
		g.castling[castleBK], g.castling[castleBQ] = false, false
	}
	for _, sq := range [][2]int{{m.fr, m.fc}, {m.tr, m.tc}} {
		switch sq {
		case [2]int{7, 7}:
			g.castling[castleWK] = false
		case [2]int{7, 0}:
			g.castling[castleWQ] = false
		case [2]int{0, 7}:
			g.castling[castleBK] = false
		case [2]int{0, 0}:
			g.castling[castleBQ] = false
		}
	}

	if (p == WhitePawn || p == BlackPawn) && abs(m.tr-m.fr) == 2 {
		g.epFile = m.tc
	} else {
		g.epFile = -1
	}

	g.lastMove.from = [2]int{m.fr, m.fc}
	g.lastMove.to = [2]int{m.tr, m.tc}
}

// cleanSAN strips whitespace, check/mate marks, annotations and an "e.p." suffix.
func cleanSAN(s string) string {
	const marks = "+#!? \t"
	s = strings.TrimRight(strings.TrimSpace(s), marks)
	return strings.TrimRight(strings.TrimSuffix(s, "e.p."), marks)
}

// resolveMove parses SAN for the side to move and returns the single legal
// move it denotes.
func (g *game) resolveMove(notation string) (move, error) {
	orig := notation
	san := cleanSAN(notation)
	if san == "" {
		return move{}, fmt.Errorf("enter a move")
	}

	legal := g.legalMoves(g.Turn)
	if len(legal) == 0 {
		return move{}, fmt.Errorf("game is over")
	}

	switch san {
	case "O-O", "0-0", "O-O-O", "0-0-0":
		tc := 6
		if len(san) == 5 {
			tc = 2
		}
		for _, m := range legal {
			if m.castle && m.tc == tc {
				return m, nil
			}
		}
		return move{}, fmt.Errorf("illegal castling %q", orig)
	}

	// Promotion: "e8=Q" or "e8Q".
	promo := byte(0)
	if i := strings.IndexByte(san, '='); i >= 0 {
		if i != len(san)-2 {
			return move{}, fmt.Errorf("invalid promotion in %q", orig)
		}
		promo, san = san[i+1], san[:i]
	} else if n := len(san); n >= 3 && strings.IndexByte("QRBN", san[n-1]) >= 0 && (san[n-2] == '1' || san[n-2] == '8') {
		promo, san = san[n-1], san[:n-1]
	}
	if promo != 0 && strings.IndexByte("QRBN", promo) < 0 {
		return move{}, fmt.Errorf("invalid promotion piece in %q", orig)
	}

	if len(san) < 2 {
		return move{}, fmt.Errorf("invalid move %q", orig)
	}
	target := san[len(san)-2:]
	if target[0] < 'a' || target[0] > 'h' || target[1] < '1' || target[1] > '8' {
		return move{}, fmt.Errorf("invalid target square in %q", orig)
	}
	tc := int(target[0] - 'a')
	tr := 8 - int(target[1]-'0')

	prefix := san[:len(san)-2]
	kind := byte('P')
	if prefix != "" && strings.IndexByte("KQRBN", prefix[0]) >= 0 {
		kind, prefix = prefix[0], prefix[1:]
	}
	prefix = strings.TrimSuffix(prefix, "x")
	srcFile, srcRank := -1, -1
	for i := 0; i < len(prefix); i++ {
		ch := prefix[i]
		switch {
		case ch >= 'a' && ch <= 'h' && srcFile < 0 && srcRank < 0:
			srcFile = int(ch - 'a')
		case ch >= '1' && ch <= '8' && srcRank < 0:
			srcRank = 8 - int(ch-'0')
		default:
			return move{}, fmt.Errorf("invalid move %q", orig)
		}
	}

	var found []move
	needsPromo := false
	for _, m := range legal {
		if m.castle || m.tr != tr || m.tc != tc || g.Board[m.fr][m.fc].kind() != kind {
			continue
		}
		if (srcFile >= 0 && m.fc != srcFile) || (srcRank >= 0 && m.fr != srcRank) {
			continue
		}
		if m.promo != Empty {
			if promo == 0 {
				needsPromo = true
				continue
			}
			if m.promo.kind() != promo {
				continue
			}
		} else if promo != 0 {
			continue
		}
		found = append(found, m)
	}

	switch {
	case len(found) == 1:
		return found[0], nil
	case len(found) > 1:
		return move{}, fmt.Errorf("ambiguous move %q", orig)
	case needsPromo:
		return move{}, fmt.Errorf("%q needs a promotion piece (e.g. =Q)", orig)
	default:
		return move{}, fmt.Errorf("illegal move %q", orig)
	}
}

var (
	knightSteps = [8][2]int{{-2, -1}, {-2, 1}, {-1, -2}, {-1, 2}, {1, -2}, {1, 2}, {2, -1}, {2, 1}}
	kingSteps   = [8][2]int{{-1, -1}, {-1, 0}, {-1, 1}, {0, -1}, {0, 1}, {1, -1}, {1, 0}, {1, 1}}
	rookDirs    = [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	bishopDirs  = [4][2]int{{-1, -1}, {-1, 1}, {1, -1}, {1, 1}}
)

func onBoard(r, c int) bool { return r >= 0 && r < 8 && c >= 0 && c < 8 }

// pawnDir is the row delta of a forward pawn step for side.
func pawnDir(side color) int {
	if side == white {
		return -1
	}
	return 1
}

// attacked reports whether square (r, c) is attacked by any piece of side by.
func (g *game) attacked(r, c int, by color) bool {
	// A pawn attacks diagonally forward, so look one row behind (r, c) from its point of view.
	pr := r - pawnDir(by)
	for _, dc := range []int{-1, 1} {
		if onBoard(pr, c+dc) && g.Board[pr][c+dc] == pieceOf('P', by) {
			return true
		}
	}
	for _, s := range knightSteps {
		if nr, nc := r+s[0], c+s[1]; onBoard(nr, nc) && g.Board[nr][nc] == pieceOf('N', by) {
			return true
		}
	}
	for _, s := range kingSteps {
		if nr, nc := r+s[0], c+s[1]; onBoard(nr, nc) && g.Board[nr][nc] == pieceOf('K', by) {
			return true
		}
	}
	slides := func(dirs [4][2]int, slider Piece) bool {
		queen := pieceOf('Q', by)
		for _, d := range dirs {
			for nr, nc := r+d[0], c+d[1]; onBoard(nr, nc); nr, nc = nr+d[0], nc+d[1] {
				if p := g.Board[nr][nc]; p != Empty {
					if p == slider || p == queen {
						return true
					}
					break
				}
			}
		}
		return false
	}
	return slides(rookDirs, pieceOf('R', by)) || slides(bishopDirs, pieceOf('B', by))
}

func (g *game) isInCheck(side color) bool {
	king := pieceOf('K', side)
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			if g.Board[r][c] == king {
				return g.attacked(r, c, !side)
			}
		}
	}
	return false // No king: only in hand-built test positions
}

// pseudoMoves lists the moves of side that obey piece movement rules, without
// checking whether they leave side's own king in check. Castling is fully
// checked except for the king's destination square.
func (g *game) pseudoMoves(side color) []move {
	var moves []move
	add := func(fr, fc, tr, tc int) {
		moves = append(moves, move{fr: fr, fc: fc, tr: tr, tc: tc})
	}
	// free reports whether (tr, tc) is on the board and not held by side.
	free := func(tr, tc int) bool {
		return onBoard(tr, tc) && !g.Board[tr][tc].is(side)
	}

	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			p := g.Board[r][c]
			if !p.is(side) {
				continue
			}
			switch p.kind() {
			case 'P':
				g.pawnMoves(side, r, c, &moves)
			case 'N':
				for _, s := range knightSteps {
					if free(r+s[0], c+s[1]) {
						add(r, c, r+s[0], c+s[1])
					}
				}
			case 'K':
				for _, s := range kingSteps {
					if free(r+s[0], c+s[1]) {
						add(r, c, r+s[0], c+s[1])
					}
				}
			default: // sliders
				var dirs [][2]int
				if p.kind() != 'B' {
					dirs = append(dirs, rookDirs[:]...)
				}
				if p.kind() != 'R' {
					dirs = append(dirs, bishopDirs[:]...)
				}
				for _, d := range dirs {
					for tr, tc := r+d[0], c+d[1]; free(tr, tc); tr, tc = tr+d[0], tc+d[1] {
						add(r, c, tr, tc)
						if g.Board[tr][tc] != Empty {
							break
						}
					}
				}
			}
		}
	}

	moves = append(moves, g.castlingMoves(side)...)
	return moves
}

func (g *game) pawnMoves(side color, r, c int, moves *[]move) {
	dir := pawnDir(side)
	startRow, epRow, lastRow := 6, 3, 0
	if side == black {
		startRow, epRow, lastRow = 1, 4, 7
	}
	add := func(tr, tc int, ep bool) {
		if tr == lastRow {
			for _, k := range []byte("QRBN") {
				*moves = append(*moves, move{fr: r, fc: c, tr: tr, tc: tc, promo: pieceOf(k, side)})
			}
			return
		}
		*moves = append(*moves, move{fr: r, fc: c, tr: tr, tc: tc, ep: ep})
	}

	tr := r + dir
	if !onBoard(tr, c) {
		return
	}
	if g.Board[tr][c] == Empty {
		add(tr, c, false)
		if r == startRow && g.Board[tr+dir][c] == Empty {
			add(tr+dir, c, false)
		}
	}
	for _, tc := range []int{c - 1, c + 1} {
		if !onBoard(tr, tc) {
			continue
		}
		if g.Board[tr][tc].is(!side) {
			add(tr, tc, false)
		} else if g.Board[tr][tc] == Empty && r == epRow && tc == g.epFile && g.Board[r][tc] == pieceOf('P', !side) {
			add(tr, tc, true)
		}
	}
}

// castlingMoves checks rights, the rook, an empty path, and that the king is
// not in check and does not pass through an attacked square. Landing in check
// is left to legalMoves.
func (g *game) castlingMoves(side color) []move {
	row := homeRow(side)
	if g.Board[row][4] != pieceOf('K', side) || g.isInCheck(side) {
		return nil
	}
	rook := pieceOf('R', side)
	var moves []move
	if g.castling[castleIdx(side, true)] && g.Board[row][7] == rook &&
		g.Board[row][5] == Empty && g.Board[row][6] == Empty && !g.attacked(row, 5, !side) {
		moves = append(moves, move{fr: row, fc: 4, tr: row, tc: 6, castle: true})
	}
	if g.castling[castleIdx(side, false)] && g.Board[row][0] == rook &&
		g.Board[row][1] == Empty && g.Board[row][2] == Empty && g.Board[row][3] == Empty && !g.attacked(row, 3, !side) {
		moves = append(moves, move{fr: row, fc: 4, tr: row, tc: 2, castle: true})
	}
	return moves
}

// leavesKingInCheck reports whether playing m would leave side's king in check.
func (g *game) leavesKingInCheck(m move, side color) bool {
	next := *g
	next.applyMove(m)
	return next.isInCheck(side)
}

func (g *game) legalMoves(side color) []move {
	var legal []move
	for _, m := range g.pseudoMoves(side) {
		if !g.leavesKingInCheck(m, side) {
			legal = append(legal, m)
		}
	}
	return legal
}

func (g *game) hasLegalMoves(side color) bool {
	return len(g.legalMoves(side)) > 0
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func renderBoard(g *game, skipR1, skipC1, skipR2, skipC2 int, movingPiece Piece, mR, mC float64) []string {
	// Initialize buffer
	buf := make([][]ScreenCell, boardH)
	for i := range buf {
		buf[i] = make([]ScreenCell, boardW)
		for j := range buf[i] {
			buf[i][j] = ScreenCell{Ch: ' ', Fg: RGB{200, 200, 200}, Bg: RGB{0, 0, 0}}
		}
	}

	// Draw top/bottom labels
	for c := 0; c < 8; c++ {
		label := string(rune('a' + c))
		labelX := labelW + 1 + c*cellW + (cellW-len(label))/2
		for i, ch := range label {
			buf[0][labelX+i] = ScreenCell{Ch: ch, Fg: labelFg, Bg: RGB{0, 0, 0}}
			buf[boardH-1][labelX+i] = ScreenCell{Ch: ch, Fg: labelFg, Bg: RGB{0, 0, 0}}
		}
	}

	// Draw tiles and static pieces
	for r := 0; r < 8; r++ {
		// Draw rank labels
		rank := fmt.Sprintf("%d", 8-r)
		rankY := 1 + r*cellH + cellH/2
		for i, ch := range rank {
			buf[rankY][labelW-len(rank)+i] = ScreenCell{Ch: ch, Fg: labelFg, Bg: RGB{0, 0, 0}}
		}

		for c := 0; c < 8; c++ {
			tc := tileColor(r, c)
			startX := labelW + 1 + c*cellW
			startY := 1 + r*cellH

			for y := 0; y < cellH; y++ {
				for x := 0; x < cellW; x++ {
					buf[startY+y][startX+x].Bg = tc
				}
			}

			if (r == skipR1 && c == skipC1) || (r == skipR2 && c == skipC2) {
				continue
			}

			p := g.Board[r][c]
			if p != Empty {
				drawPiece(buf, p, float64(r), float64(c))
			}
		}
	}

	// Draw moving piece
	if movingPiece != Empty {
		drawPiece(buf, movingPiece, mR, mC)
	}

	// Convert buffer to lines
	lines := make([]string, boardH)
	for y := 0; y < boardH; y++ {
		var line strings.Builder
		var lastFg, lastBg RGB
		var lastBold bool
		first := true

		for x := 0; x < boardW; x++ {
			cell := buf[y][x]
			if first || cell.Fg != lastFg || cell.Bg != lastBg || cell.Bold != lastBold {
				line.WriteString(reset)
				line.WriteString(cell.Bg.bg())
				line.WriteString(cell.Fg.fg())
				if cell.Bold {
					line.WriteString(bold)
				}
				lastFg, lastBg, lastBold = cell.Fg, cell.Bg, cell.Bold
				first = false
			}
			line.WriteRune(cell.Ch)
		}
		line.WriteString(reset)
		lines[y] = line.String()
	}

	return lines
}

type ScreenCell struct {
	Ch   rune
	Fg   RGB
	Bg   RGB
	Bold bool
}

func drawPiece(buf [][]ScreenCell, p Piece, r, c float64) {
	design := pieceDesign(p)
	padX := (cellW - 7) / 2
	padY := (cellH - 3) / 2

	baseX := float64(labelW+1) + c*float64(cellW) + float64(padX)
	baseY := 1.0 + r*float64(cellH) + float64(padY)

	for pr := 0; pr < 3; pr++ {
		rowStr := design[pr]
		pc := pieceColor(p, pr)
		y := int(math.Round(baseY + float64(pr)))
		if y < 0 || y >= len(buf) {
			continue
		}

		for px, ch := range []rune(rowStr) {
			x := int(math.Round(baseX + float64(px)))
			if x < 0 || x >= len(buf[y]) {
				continue
			}
			if ch != ' ' {
				buf[y][x].Ch = ch
				buf[y][x].Fg = pc
				buf[y][x].Bold = true
			}
		}
	}
}

// endsInComment reports whether a movetext line that starts inside a {...}
// comment (or not) ends inside one. Braces after a ';' comment are literal.
func endsInComment(line string, inComment bool) bool {
	for _, ch := range line {
		switch {
		case inComment:
			inComment = ch != '}'
		case ch == '{':
			inComment = true
		case ch == ';':
			return false
		}
	}
	return inComment
}

type pgnGame struct {
	Tags  map[string]string
	Moves []string
}

func parsePGN(data string) []pgnGame {
	var games []pgnGame
	lines := strings.Split(data, "\n")
	var current pgnGame
	current.Tags = make(map[string]string)
	var moveLines []string
	inMoves := false
	inComment := false // inside a {...} comment, which can span lines

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%") { // '%' lines are PGN escapes
			continue
		}

		if strings.HasPrefix(line, "[") && !inComment {
			if inMoves {
				current.Moves = parseMoves(strings.Join(moveLines, "\n"))
				games = append(games, current)
				current = pgnGame{Tags: make(map[string]string)}
				moveLines = nil
				inMoves = false
			}
			line = strings.Trim(line, "[]")
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				key := parts[0]
				val := strings.Trim(parts[1], "\"")
				current.Tags[key] = val
			}
		} else {
			inMoves = true
			moveLines = append(moveLines, line)
			inComment = endsInComment(line, inComment)
		}
	}
	if inMoves {
		current.Moves = parseMoves(strings.Join(moveLines, "\n"))
		games = append(games, current)
	}
	return games
}

// parseMoves extracts the mainline SAN moves from PGN movetext. It drops
// comments ({...} and ; to end of line), variations (nested to any depth),
// move numbers, NAGs ($n), annotation marks (!, ?) and the game result.
func parseMoves(moveText string) []string {
	var moves []string
	var tok strings.Builder
	depth := 0

	flush := func() {
		t := tok.String()
		tok.Reset()
		if depth > 0 || t == "" || t[0] == '$' {
			return
		}
		if i := strings.LastIndexByte(t, '.'); i >= 0 { // "1.e4", "1...e5", "e.p."
			t = t[i+1:]
		}
		t = strings.TrimRight(t, "!?")
		switch t {
		case "", "1-0", "0-1", "1/2-1/2", "*":
			return
		}
		moves = append(moves, t)
	}

	for i := 0; i < len(moveText); i++ {
		switch ch := moveText[i]; ch {
		case '{':
			flush()
			if j := strings.IndexByte(moveText[i:], '}'); j >= 0 {
				i += j
			} else {
				i = len(moveText)
			}
		case ';':
			flush()
			if j := strings.IndexByte(moveText[i:], '\n'); j >= 0 {
				i += j
			} else {
				i = len(moveText)
			}
		case '(':
			flush()
			depth++
		case ')':
			flush()
			if depth > 0 {
				depth--
			}
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			tok.WriteByte(ch)
		}
	}
	flush()
	return moves
}

// posStatus is derived from the position; compute it once per position, not per frame.
type posStatus struct {
	inCheck, hasMoves bool
}

func (g *game) status() posStatus {
	return posStatus{inCheck: g.isInCheck(g.Turn), hasMoves: g.hasLegalMoves(g.Turn)}
}

// printFrame draws the board and side panel and returns the screen row
// reserved for the move prompt.
func printFrame(g *game, st posStatus, skipR1, skipC1, skipR2, skipC2 int, p Piece, r, c float64) int {
	boardLines := renderBoard(g, skipR1, skipC1, skipR2, skipC2, p, r, c)

	theme := themes[currentThemeIdx]
	var rightLines []string
	rightLines = append(rightLines, fmt.Sprintf("%s%s CHESS GAME %s", bold, theme.StartColor.bg()+whiteTop.fg(), reset))
	if len(g.Moves) > 0 {
		rightLines = append(rightLines, fmt.Sprintf(" [Step %d/%d]", g.HistoryIdx, len(g.Moves)))
	}
	if g.inVariation() {
		rightLines = append(rightLines, fmt.Sprintf("\x1b[38;2;255;200;80mYour variation\x1b[0m \x1b[2m(← to step %d resumes the PGN)\x1b[0m", g.branchIdx()))
	}
	rightLines = append(rightLines, "")

	if len(g.Tags) > 0 {
		white := g.Tags["White"]
		if elo := g.Tags["WhiteElo"]; elo != "" {
			white += " (" + elo + ")"
		}
		black := g.Tags["Black"]
		if elo := g.Tags["BlackElo"]; elo != "" {
			black += " (" + elo + ")"
		}
		rightLines = append(rightLines, fmt.Sprintf("%s%s vs %s %s", bold, white, black, reset))
		rightLines = append(rightLines, fmt.Sprintf("\x1b[2m%s | %s\x1b[0m", g.Tags["Event"], g.Tags["Date"]))
		rightLines = append(rightLines, fmt.Sprintf("\x1b[2m%s | %s\x1b[0m", g.Tags["ECO"], g.Tags["Result"]))
		rightLines = append(rightLines, "")
	}

	turnColor := whiteTop
	if g.Turn == black {
		turnColor = theme.EndColor
	}
	status := fmt.Sprintf("Turn: %s%s %s %s", bold, turnColor.bg()+RGB{0, 0, 0}.fg(), g.Turn, reset)
	if st.inCheck && st.hasMoves {
		status += " \x1b[38;2;255;80;80m[CHECK]\x1b[0m"
	}
	rightLines = append(rightLines, status)

	if g.Error != "" {
		rightLines = append(rightLines, fmt.Sprintf("\x1b[38;2;255;80;80mError: %s\x1b[0m", g.Error))
	} else {
		rightLines = append(rightLines, "")
	}

	hist := "History: "
	if len(g.History) > 0 {
		start := max(0, len(g.History)-3)
		hist += strings.Join(g.History[start:], " | ")
	} else {
		hist += "(empty)"
	}
	rightLines = append(rightLines, "\x1b[2m"+hist+"\x1b[0m")

	if !st.hasMoves && g.HistoryIdx == len(g.Moves) {
		rightLines = append(rightLines, "")
		if st.inCheck {
			rightLines = append(rightLines, fmt.Sprintf("%sCHECKMATE! %s wins!%s", bold+"\x1b[38;2;255;80;80m", (!g.Turn).String(), reset))
		} else {
			rightLines = append(rightLines, fmt.Sprintf("%sSTALEMATE! It's a draw.%s", bold+"\x1b[38;2;255;255;80m", reset))
		}
	}

	rightLines = append(rightLines, "")
	rightLines = append(rightLines, "[←/→] Prev/Next Move")
	rightLines = append(rightLines, "[Home/End] First/Last")
	rightLines = append(rightLines, "[F1-F5] Themes")
	rightLines = append(rightLines, "Type a move, [Enter] to play")
	rightLines = append(rightLines, "[Esc] Clear move")
	rightLines = append(rightLines, "[?] Help")
	rightLines = append(rightLines, "[Ctrl-D] Quit")
	rightLines = append(rightLines, "")
	rightLines = append(rightLines, "") // move prompt
	promptRow := len(rightLines)

	// Redraw from the top-left corner
	fmt.Print("\x1b[H")
	maxRows := max(len(boardLines), len(rightLines))
	for i := 0; i < maxRows; i++ {
		if i < len(boardLines) {
			fmt.Print(boardLines[i])
		} else {
			fmt.Print(strings.Repeat(" ", boardW))
		}

		if i < len(rightLines) {
			fmt.Print(gutter + rightLines[i])
		}
		fmt.Println("\x1b[K")
	}

	// Park the cursor on the prompt row so it doesn't hang at the bottom.
	fmt.Printf("\x1b[%d;%dH", promptRow, promptCol)
	return promptRow
}

// promptCol is the 1-based screen column where the side panel starts.
const promptCol = boardW + len(gutter) + 1

func animate(g *game, st posStatus, m move) {
	steps := 12
	duration := 180 * time.Millisecond
	p := g.Board[m.fr][m.fc]

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		// Ease-in curve
		et := t * t
		r := float64(m.fr) + float64(m.tr-m.fr)*et
		c := float64(m.fc) + float64(m.tc-m.fc)*et

		printFrame(g, st, m.fr, m.fc, m.tr, m.tc, p, r, c)
		time.Sleep(duration / time.Duration(steps))
	}
}

// splashW is the inner width of the splash window, in terminal cells.
const splashW = 60

// showSplash draws the title and help window over the board and waits for
// Esc. It returns false if the user quit (Q or EOF) instead.
func showSplash(stdin *bufio.Reader) bool {
	theme := themes[currentThemeIdx]
	border := lerp(theme.EndColor, whiteTop, 0.2)
	keyFg := lerp(theme.EndColor, whiteTop, 0.5)
	bg := RGB{18, 18, 24}

	// pad centers (or left-aligns) styled text whose visible width is w.
	pad := func(styled string, w int, center bool) string {
		left := 2
		if center {
			left = (splashW - w) / 2
		}
		return strings.Repeat(" ", left) + styled + bg.bg() + strings.Repeat(" ", splashW-left-w)
	}

	title := "♔  T E R M I N A L   C H E S S  ♚"
	n := utf8.RuneCountInString(title)
	var t strings.Builder
	t.WriteString(bold)
	i := 0
	for _, r := range title {
		t.WriteString(lerp(whiteTop, theme.EndColor, float64(i)/float64(n-1)).fg())
		t.WriteRune(r)
		i++
	}
	t.WriteString(reset + bg.bg())

	desc := []string{
		"A chess board in your terminal, with full move validation.",
		"Play moves yourself, or replay games from a PGN file.",
	}
	keys := [][2]string{
		{"← / →", "Previous / next move"},
		{"Home / End", "First / last move"},
		{"(type)", "A move in SAN: e4, Nf3, exd5, O-O, e8=Q"},
		{"Enter", "Play the typed move"},
		{"Backspace", "Erase a character"},
		{"Esc", "Clear the typed move / close this window"},
		{"F1-F5", "Switch color theme"},
		{"?", "Show this window again"},
		{"Ctrl-D", "Quit"},
	}

	lines := []string{
		pad("", 0, false),
		pad(t.String(), n, true),
		pad("", 0, false),
	}
	for _, d := range desc {
		lines = append(lines, pad(whiteBottom.fg()+d, utf8.RuneCountInString(d), true))
	}
	lines = append(lines, pad("", 0, false))
	lines = append(lines, pad(bold+border.fg()+"Keys"+reset+bg.bg(), 4, false))
	for _, k := range keys {
		text := fmt.Sprintf("  %s%-12s%s%s", bold+keyFg.fg(), k[0], reset+bg.bg()+whiteBottom.fg(), k[1])
		lines = append(lines, pad(text, 14+utf8.RuneCountInString(k[1]), false))
	}
	lines = append(lines, pad("", 0, false))
	hint := "Press Esc to close"
	lines = append(lines, pad("\x1b[2m"+whiteBottom.fg()+hint+reset+bg.bg(), len(hint), true))
	lines = append(lines, pad("", 0, false))

	// Center the window over the board.
	row := (boardH-len(lines)-2)/2 + 1
	col := (boardW-splashW-2)/2 + 1
	edge := bg.bg() + border.fg()
	fmt.Printf("\x1b[%d;%dH%s╭%s╮%s", row, col, edge, strings.Repeat("─", splashW), reset)
	for i, l := range lines {
		fmt.Printf("\x1b[%d;%dH%s│%s%s%s│%s", row+1+i, col, edge, reset+bg.bg()+whiteTop.fg(), l, border.fg(), reset)
	}
	fmt.Printf("\x1b[%d;%dH%s╰%s╯%s", row+1+len(lines), col, edge, strings.Repeat("─", splashW), reset)

	for {
		b, err := stdin.ReadByte()
		if err != nil {
			return false
		}
		switch b {
		case ctrlD:
			return false
		case '\x1b':
			// A lone Esc arrives by itself; arrow keys etc. arrive as one
			// burst, so drain and ignore those.
			if stdin.Buffered() == 0 {
				return true
			}
			stdin.Discard(stdin.Buffered())
		}
	}
}

const ctrlD = 0x04 // passed through as a byte in cbreak mode

// Keys decoded from escape sequences by readKey.
const (
	keyNone = iota
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyF1 // keyF1+n is F(n+1)
)

// readKey decodes the escape sequence whose leading Esc was just read:
// SS3 (ESC O x), CSI (ESC [ params final) and the Linux console's ESC [ [ x
// for F1-F5. Unknown sequences yield keyNone.
func readKey(stdin *bufio.Reader) int {
	b, _ := stdin.ReadByte()
	switch b {
	case 'O':
		b, _ = stdin.ReadByte()
		switch b {
		case 'H':
			return keyHome
		case 'F':
			return keyEnd
		case 'P', 'Q', 'R', 'S':
			return keyF1 + int(b-'P')
		}
	case '[':
		var params []byte
		for {
			b, _ = stdin.ReadByte()
			if (b < '0' || b > '9') && b != ';' {
				break
			}
			params = append(params, b)
		}
		// Ignore modifiers, e.g. "1;5" for Ctrl.
		n, _ := strconv.Atoi(strings.SplitN(string(params), ";", 2)[0])
		switch b {
		case 'D':
			return keyLeft
		case 'C':
			return keyRight
		case 'H':
			return keyHome
		case 'F':
			return keyEnd
		case '[':
			if b, _ = stdin.ReadByte(); b >= 'A' && b <= 'E' {
				return keyF1 + int(b-'A')
			}
		case '~':
			switch n {
			case 1, 7:
				return keyHome
			case 4, 8:
				return keyEnd
			case 11, 12, 13, 14, 15:
				return keyF1 + n - 11
			}
		}
	}
	return keyNone
}

// stty runs stty against the terminal on stdin (portable: no GNU-only -F).
func stty(args ...string) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
}

func setCbreak()       { stty("cbreak", "min", "1") }
func restoreTerminal() { stty("sane") }

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func main() {
	g := newGame()
	stdin := bufio.NewReader(os.Stdin)
	fmt.Print(clearScreen)

	if len(os.Args) > 1 {
		filePath := os.Args[1]
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}

		games := parsePGN(string(data))
		if len(games) == 0 {
			fmt.Println("No games found in PGN.")
			os.Exit(1)
		}

		selectedIdx := 0
		if len(games) > 1 {
			fmt.Printf("Found %d games. Select one (1-%d):\n", len(games), len(games))
			for i, g := range games {
				if i >= 20 { // Limit list
					fmt.Printf("... and %d more\n", len(games)-20)
					break
				}
				fmt.Printf("[%d] %s: %s vs %s (%s)\n", i+1, g.Tags["Event"], g.Tags["White"], g.Tags["Black"], g.Tags["Date"])
			}
			fmt.Print("Choice: ")
			line, _ := stdin.ReadString('\n')
			if choice, err := strconv.Atoi(strings.TrimSpace(line)); err == nil && choice >= 1 && choice <= len(games) {
				selectedIdx = choice - 1
			}
		}

		g.Moves = games[selectedIdx].Moves
		g.PGN = g.Moves
		g.Tags = games[selectedIdx].Tags
		g.Error = errText(g.goToMove(len(g.Moves)))
	}

	// Set terminal to cbreak mode to capture arrow keys, and restore it on
	// every exit path, including Ctrl-C.
	setCbreak()
	defer func() {
		fmt.Print(reset + clearScreen + wrapOn)
		restoreTerminal()
	}()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigs
		fmt.Print(reset + clearScreen + wrapOn)
		restoreTerminal()
		os.Exit(130)
	}()

	fmt.Print(clearScreen + wrapOff)

	navigate := func(idx int) { g.Error = errText(g.goToMove(idx)) }

	showHelp := true
	input := "" // move typed so far
	for {
		st := g.status()
		promptRow := printFrame(g, st, -1, -1, -1, -1, Empty, 0, 0)
		if showHelp {
			if !showSplash(stdin) {
				break
			}
			showHelp = false
			continue
		}

		// The move being typed is echoed on the prompt row; the cursor stays
		// at its end.
		fmt.Printf("\x1b[%d;%dH\x1b[K%sMove:%s %s", promptRow, promptCol, bold, reset, input)

		// Capture single key or start of escape sequence
		b, err := stdin.ReadByte()
		if err != nil || b == ctrlD { // stdin closed or quit
			break
		}
		if b == '?' {
			showHelp = true
			continue
		}

		if b == '\x1b' { // Escape sequence
			// A lone Esc arrives by itself; arrow and function keys arrive
			// as one burst.
			if stdin.Buffered() == 0 {
				input = ""
				continue
			}
			switch key := readKey(stdin); {
			case key == keyLeft:
				if g.HistoryIdx > 0 {
					navigate(g.HistoryIdx - 1)
				}
			case key == keyRight: // step forward incrementally
				if g.HistoryIdx < len(g.Moves) {
					notation := g.Moves[g.HistoryIdx]
					m, err := g.resolveMove(notation)
					if err != nil {
						g.Error = fmt.Sprintf("move %d (%s): %v", g.HistoryIdx+1, notation, err)
						break
					}
					animate(g, st, m)
					g.playMove(m, notation)
					g.HistoryIdx++
					g.Error = ""
				}
			case key == keyHome:
				navigate(0)
			case key == keyEnd:
				navigate(len(g.Moves))
			case key >= keyF1 && key < keyF1+len(themes):
				currentThemeIdx = key - keyF1
			}
			continue
		}

		switch {
		case b == 0x7f || b == '\b': // Backspace
			if input != "" {
				input = input[:len(input)-1]
			}
		case b > ' ' && b < 0x7f: // printable: part of the move
			input += string(b)
		case b == '\n' || b == '\r':
			if input == "" {
				continue
			}
			// Validate before touching g.Moves, so a typo doesn't drop the
			// rest of a PGN. A rejected move stays on the prompt to be fixed.
			m, err := g.resolveMove(input)
			if err != nil {
				g.Error = err.Error()
				continue
			}
			animate(g, st, m)
			g.playTyped(m, input)
			g.Error = ""
			input = ""
		}
	}
}
