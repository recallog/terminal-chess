package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestCastlingBug(t *testing.T) {
	g := newGame()

	// Move White King and back
	// e4, e5, Ke2, Ke7, Ke1, Ke8
	moves := []string{"e4", "e5", "Ke2", "Ke7", "Ke1", "Ke8"}
	for _, m := range moves {
		if err := g.executeMove(m); err != nil {
			t.Fatalf("failed to execute move %s: %v", m, err)
		}
		g.Turn = !g.Turn
	}

	// Now try to castle O-O for white
	// It should fail because the king has moved.
	err := g.executeMove("O-O")
	if err == nil {
		t.Errorf("expected error when castling after moving king, but got none")
	}
}

func TestCheckLegality(t *testing.T) {
	g := newGame()

	// Set up a pin: White king at e1, White bishop at e2, Black rook at e8
	g.Board = [8][8]Piece{}
	g.Board[7][4] = WhiteKing
	g.Board[6][4] = WhiteBishop
	g.Board[0][4] = BlackRook
	g.Turn = white

	// Try to move the pinned bishop
	err := g.executeMove("Bd3")
	if err == nil {
		t.Errorf("expected error when moving pinned piece, but got none")
	}

	// Try to move king into check
	err = g.executeMove("Ke2")
	if err == nil {
		t.Errorf("expected error when moving king into check, but got none")
	}
}

// loadFEN sets up a position from the first four FEN fields (placement, side,
// castling, en passant).
func loadFEN(t *testing.T, fen string) *game {
	t.Helper()
	f := strings.Fields(fen)
	if len(f) < 4 {
		t.Fatalf("bad FEN %q", fen)
	}
	g := newGame()
	g.Board = [8][8]Piece{}
	for r, rank := range strings.Split(f[0], "/") {
		c := 0
		for _, ch := range rank {
			if ch >= '1' && ch <= '8' {
				c += int(ch - '0')
				continue
			}
			side := color(ch >= 'A' && ch <= 'Z')
			g.Board[r][c] = pieceOf(byte(unicode.ToUpper(ch)), side)
			c++
		}
	}
	g.Turn = f[1] == "w"
	for i, k := range "KQkq" {
		g.castling[i] = strings.ContainsRune(f[2], k)
	}
	g.epFile = -1
	if f[3] != "-" {
		g.epFile = int(f[3][0] - 'a')
	}
	return g
}

func playAll(t *testing.T, g *game, moves ...string) {
	t.Helper()
	for _, m := range moves {
		if err := g.play(m); err != nil {
			t.Fatalf("move %s: %v", m, err)
		}
	}
}

func perft(g *game, depth int) int {
	if depth == 0 {
		return 1
	}
	n := 0
	for _, m := range g.legalMoves(g.Turn) {
		next := *g
		next.applyMove(m)
		next.Turn = !next.Turn
		n += perft(&next, depth-1)
	}
	return n
}

// Reference counts from https://www.chessprogramming.org/Perft_Results
func TestPerft(t *testing.T) {
	cases := []struct {
		name   string
		fen    string
		counts []int
	}{
		{"start", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -", []int{20, 400, 8902}},
		{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq -", []int{48, 2039, 97862}},
		{"position3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - -", []int{14, 191, 2812, 43238}},
		{"position4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq -", []int{6, 264, 9467}},
		{"position5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ -", []int{44, 1486, 62379}},
	}
	for _, tc := range cases {
		g := loadFEN(t, tc.fen)
		for d, want := range tc.counts {
			if got := perft(g, d+1); got != want {
				t.Errorf("%s depth %d: got %d, want %d", tc.name, d+1, got, want)
			}
		}
	}
}

func TestEnPassantRequiresFifthRank(t *testing.T) {
	g := newGame()
	playAll(t, g, "e4", "d5")
	if err := g.executeMove("cxd3"); err == nil {
		t.Fatal("cxd3 from c2 accepted as en passant")
	}
	if g.Board[6][3] != WhitePawn {
		t.Fatal("d2 pawn was removed")
	}
}

func TestEnPassantCapture(t *testing.T) {
	g := newGame()
	playAll(t, g, "e4", "a6", "e5", "d5", "exd6")
	if g.Board[3][3] != Empty || g.Board[2][3] != WhitePawn {
		t.Fatal("en passant did not remove the d5 pawn")
	}
}

func TestCastlingLostWhenRookCaptured(t *testing.T) {
	g := loadFEN(t, "4k3/8/2b5/8/8/8/8/4K2R b K -")
	playAll(t, g, "Bxh1")
	if err := g.executeMove("O-O"); err == nil {
		t.Fatal("castled after the h1 rook was captured")
	}
}

func TestCastlingMovesRook(t *testing.T) {
	g := loadFEN(t, "r3k3/8/8/8/8/8/8/4K2R w Kq -")
	playAll(t, g, "O-O", "0-0-0")
	if g.Board[7][6] != WhiteKing || g.Board[7][5] != WhiteRook || g.Board[0][2] != BlackKing || g.Board[0][3] != BlackRook {
		t.Fatal("castling placed king or rook wrongly")
	}
}

func TestPromotion(t *testing.T) {
	g := loadFEN(t, "7k/P7/8/8/8/8/8/4K3 w - -")
	for _, bad := range []string{"a8", "a8=K", "a8=x", "Ke2=Q"} {
		if err := g.executeMove(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if err := g.executeMove("a8=N"); err != nil || g.Board[0][0] != WhiteKnight {
		t.Fatalf("a8=N: err=%v piece=%s", err, g.Board[0][0].glyph())
	}
	g = loadFEN(t, "7k/P7/8/8/8/8/8/4K3 w - -")
	if err := g.executeMove("a8Q+"); err != nil || g.Board[0][0] != WhiteQueen {
		t.Fatalf("a8Q+: err=%v", err)
	}
}

func TestHistoryNumbering(t *testing.T) {
	g := newGame()
	playAll(t, g, "e4", "e5", "Nf3")
	want := []string{"1.e4", "e5", "2.Nf3"}
	if strings.Join(g.History, " ") != strings.Join(want, " ") {
		t.Fatalf("history %q, want %q", g.History, want)
	}
}

func TestGoToMoveReportsBadMove(t *testing.T) {
	g := newGame()
	g.Moves = []string{"e4", "e5", "Ke3", "Nc6"}
	err := g.goToMove(len(g.Moves))
	if err == nil || !strings.Contains(err.Error(), "move 3") {
		t.Fatalf("got %v, want error for move 3", err)
	}
	if g.HistoryIdx != 2 || g.Turn != white {
		t.Fatalf("stopped at idx %d turn %s, want 2 White", g.HistoryIdx, g.Turn)
	}
}

func TestVariationResumesPGN(t *testing.T) {
	g := newGame()
	g.PGN = []string{"e4", "e5", "Nf3", "Nc6", "Bb5"}
	g.Moves = g.PGN
	if err := g.goToMove(2); err != nil {
		t.Fatal(err)
	}

	// Branch at step 2 with 2.d4 and continue the variation.
	for _, san := range []string{"d4", "exd4"} {
		m, err := g.resolveMove(san)
		if err != nil {
			t.Fatal(err)
		}
		g.playTyped(m, san)
	}
	if !g.inVariation() || g.branchIdx() != 2 {
		t.Fatalf("inVariation %v branchIdx %d, want true 2", g.inVariation(), g.branchIdx())
	}
	if got := strings.Join(g.PGN, " "); got != "e4 e5 Nf3 Nc6 Bb5" {
		t.Fatalf("PGN overwritten: %q", got)
	}

	// Inside the variation, the variation is kept.
	if err := g.goToMove(3); err != nil {
		t.Fatal(err)
	}
	if !g.inVariation() {
		t.Fatal("variation dropped while still inside it")
	}

	// Back at the branch point, the PGN is resumed.
	if err := g.goToMove(2); err != nil {
		t.Fatal(err)
	}
	if g.inVariation() || len(g.Moves) != 5 {
		t.Fatalf("moves %q, want the PGN", g.Moves)
	}
	if err := g.goToMove(len(g.Moves)); err != nil {
		t.Fatal(err)
	}
	if g.Board[3][1] != WhiteBishop { // b5
		t.Fatal("PGN not replayed to the end")
	}
}

func TestCheckNotFromCastlingKing(t *testing.T) {
	// Kings on e8 and g8 are not adjacent; Black's castling rights must not
	// count as an attack on g8.
	g := loadFEN(t, "4k1K1/8/8/8/8/8/8/7r w k -")
	if g.isInCheck(white) {
		t.Fatal("white reported in check by black's castling move")
	}
}

func TestParseMoves(t *testing.T) {
	text := "1. e4! {best by test} e5 (1... c5 (1... e6) 2. Nf3) 2. Nf3?! $1 Nc6 ; comment\n3. Bb5 a6 1-0"
	got := strings.Join(parseMoves(text), " ")
	if want := "e4 e5 Nf3 Nc6 Bb5 a6"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Malformed input must terminate.
	parseMoves("e4 } { ) ( e5")
}

func TestAmbiguousMove(t *testing.T) {
	g := loadFEN(t, "4k3/8/8/8/8/8/8/R4RK1 w - -")
	if err := g.executeMove("Rd1"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("Rd1: got %v, want ambiguous", err)
	}
	if err := g.executeMove("Rad1"); err != nil {
		t.Fatalf("Rad1: %v", err)
	}
}

func TestBundledPGNs(t *testing.T) {
	files, err := filepath.Glob("pgn/*.pgn")
	if err != nil || len(files) == 0 {
		t.Fatalf("no PGN files found: %v", err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, pg := range parsePGN(string(data)) {
			g := newGame()
			g.Moves = pg.Moves
			if err := g.goToMove(len(g.Moves)); err != nil {
				t.Errorf("%s game %d: %v", f, i+1, err)
			}
		}
	}
}

func TestParsePGNBracketInComment(t *testing.T) {
	// A comment line starting with '[' must not be read as a tag starting a
	// new game.
	data := "[Event \"A\"]\n\n1. e4 e5 {a\n[pgndiagram] long comment} 2. Nf3 *\n\n[Event \"B\"]\n\n1. d4 *\n"
	games := parsePGN(data)
	if len(games) != 2 {
		t.Fatalf("got %d games, want 2", len(games))
	}
	if got := strings.Join(games[0].Moves, " "); got != "e4 e5 Nf3" {
		t.Fatalf("game 1 moves %q, want \"e4 e5 Nf3\"", got)
	}
}
