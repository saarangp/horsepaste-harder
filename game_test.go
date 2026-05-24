package horsepaste

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/jbowens/dictionary"
)

var testWords []string

func init() {
	d, err := dictionary.Load("assets/original.txt")
	if err != nil {
		panic(err)
	}
	testWords = d.Words()
}

func BenchmarkGameMarshal(b *testing.B) {
	b.StopTimer()
	d, err := dictionary.Load("assets/original.txt")
	if err != nil {
		b.Fatal(err)
	}
	g := newGame("foo", GameState{
		Seed:     1,
		Round:    0,
		Revealed: make([]bool, 25),
		WordSet:  d.Words(),
	}, GameOptions{}, nil)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err = json.Marshal(g)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestGameShuffle(t *testing.T) {
	gamesWithoutRepeats := len(testWords)/25 - 1

	initialState := randomState(testWords)
	currState := initialState

	m := map[string]int{}
	for i := 0; i < gamesWithoutRepeats; i++ {
		g := newGame("foo", currState, GameOptions{}, nil)
		for _, w := range g.Words {
			if prevI, ok := m[w]; ok {
				t.Errorf("Word %q appeared twice, once in game %d and once in game %d.", w, prevI, i)
			}
			m[w] = i
		}
		currState = nextGameState(currState)
	}
}

func TestHardModeBoard(t *testing.T) {
	// A small synthetic neighbor table: APPLE is surrounded by fruit, and
	// every fruit is also a neighbor of the others so they're valid fillers.
	fruit := []string{"APPLE", "BANANA", "CHERRY", "GRAPE", "LEMON", "MANGO",
		"ORANGE", "PEACH", "PEAR", "PLUM"}
	words := append([]string{}, fruit...)
	for i := 0; i < 20; i++ {
		words = append(words, "FILLER"+string(rune('A'+i)))
	}

	neighbors := map[string][]string{}
	for _, f := range fruit {
		var ns []string
		for _, other := range fruit {
			if other != f {
				ns = append(ns, other)
			}
		}
		neighbors[f] = ns
	}

	// Not enough fruit to fill a 25-tile board, so a board can't be clustered.
	if _, _, ok := hardModeWords(rand.New(rand.NewSource(1)), words, neighbors); ok {
		t.Fatal("expected hardModeWords to fail without enough in-set neighbors")
	}

	// Give every word a large neighbor list drawn from the whole set so any
	// word can anchor a full board.
	full := map[string][]string{}
	for _, w := range words {
		var ns []string
		for _, other := range words {
			if other != w {
				ns = append(ns, other)
			}
		}
		full[w] = ns
	}

	state := randomState(words)
	state.Seed = 42
	g := newGame("foo", state, GameOptions{HardMode: true}, full)

	if len(g.Words) != wordsPerGame {
		t.Fatalf("expected %d words, got %d", wordsPerGame, len(g.Words))
	}
	seen := map[string]bool{}
	for _, w := range g.Words {
		if seen[w] {
			t.Errorf("duplicate word on board: %q", w)
		}
		seen[w] = true
	}

	// The assassin word must be one of the board words.
	var blackWord string
	for i, team := range g.Layout {
		if team == Black {
			blackWord = g.Words[i]
		}
	}
	if !seen[blackWord] {
		t.Errorf("assassin word %q not on the board", blackWord)
	}

	// Determinism: same seed and inputs must reproduce the same board.
	g2 := newGame("foo", state, GameOptions{HardMode: true}, full)
	for i := range g.Words {
		if g.Words[i] != g2.Words[i] || g.Layout[i] != g2.Layout[i] {
			t.Fatalf("hard mode board not deterministic at index %d", i)
		}
	}
}
