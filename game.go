package horsepaste

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"
)

const wordsPerGame = 25

type Team int

const (
	Neutral Team = iota
	Red
	Blue
	Black
)

func (t Team) String() string {
	switch t {
	case Red:
		return "red"
	case Blue:
		return "blue"
	case Black:
		return "black"
	default:
		return "neutral"
	}
}

func (t Team) Other() Team {
	if t == Red {
		return Blue
	}
	if t == Blue {
		return Red
	}
	return t
}

func (t *Team) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	switch s {
	case "red":
		*t = Red
	case "blue":
		*t = Blue
	case "black":
		*t = Black
	default:
		*t = Neutral
	}
	return nil
}

func (t Team) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t Team) Repeat(n int) []Team {
	s := make([]Team, n)
	for i := 0; i < n; i++ {
		s[i] = t
	}
	return s
}

// GameState encapsulates enough data to reconstruct
// a Game's state. It's used to recreate games after
// a process restart.
type GameState struct {
	Seed      int64    `json:"seed"`
	PermIndex int      `json:"perm_index"`
	Round     int      `json:"round"`
	Revealed  []bool   `json:"revealed"`
	WordSet   []string `json:"word_set"`
}

func (gs GameState) anyRevealed() bool {
	var revealed bool
	for _, r := range gs.Revealed {
		revealed = revealed || r
	}
	return revealed
}

func randomState(words []string) GameState {
	return GameState{
		Seed:      rand.Int63(),
		PermIndex: 0,
		Round:     0,
		Revealed:  make([]bool, wordsPerGame),
		WordSet:   words,
	}
}

// nextGameState returns a new GameState for the next game.
func nextGameState(state GameState) GameState {
	state.PermIndex = state.PermIndex + wordsPerGame
	if state.PermIndex+wordsPerGame >= len(state.WordSet) {
		state.Seed = rand.Int63()
		state.PermIndex = 0
	}
	state.Revealed = make([]bool, wordsPerGame)
	state.Round = 0
	return state
}

type Game struct {
	GameState
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	StartingTeam   Team      `json:"starting_team"`
	WinningTeam    *Team     `json:"winning_team,omitempty"`
	Words          []string  `json:"words"`
	Layout         []Team    `json:"layout"`
	RoundStartedAt time.Time `json:"round_started_at,omitempty"`
	GameOptions
}

type GameOptions struct {
	TimerDurationMS int64 `json:"timer_duration_ms,omitempty"`
	EnforceTimer    bool  `json:"enforce_timer,omitempty"`
	HardMode        bool  `json:"hard_mode,omitempty"`
}

func (g *Game) StateID() string {
	return fmt.Sprintf("%019d", g.UpdatedAt.UnixNano())
}

func (g *Game) checkWinningCondition() {
	if g.WinningTeam != nil {
		return
	}
	var redRemaining, blueRemaining bool
	for i, t := range g.Layout {
		if g.Revealed[i] {
			continue
		}
		switch t {
		case Red:
			redRemaining = true
		case Blue:
			blueRemaining = true
		}
	}
	if !redRemaining {
		winners := Red
		g.WinningTeam = &winners
	}
	if !blueRemaining {
		winners := Blue
		g.WinningTeam = &winners
	}
}

func (g *Game) NextTurn(currentTurn int) bool {
	if g.WinningTeam != nil {
		return false
	}
	// TODO: remove currentTurn != 0 once we can be sure all
	// clients are running up-to-date versions of the frontend.
	if g.Round != currentTurn && currentTurn != 0 {
		return false
	}
	g.UpdatedAt = time.Now()
	g.Round++
	g.RoundStartedAt = time.Now()
	return true
}

func (g *Game) Guess(idx int) error {
	if idx > len(g.Layout) || idx < 0 {
		return fmt.Errorf("index %d is invalid", idx)
	}
	if g.Revealed[idx] {
		return errors.New("cell has already been revealed")
	}
	g.UpdatedAt = time.Now()
	g.Revealed[idx] = true

	if g.Layout[idx] == Black {
		winners := g.currentTeam().Other()
		g.WinningTeam = &winners
		return nil
	}

	g.checkWinningCondition()
	if g.Layout[idx] != g.currentTeam() {
		g.Round = g.Round + 1
		g.RoundStartedAt = time.Now()
	}
	return nil
}

func (g *Game) currentTeam() Team {
	if g.Round%2 == 0 {
		return g.StartingTeam
	}
	return g.StartingTeam.Other()
}

func newGame(id string, state GameState, opts GameOptions, neighbors map[string][]string) *Game {
	// consistent randomness across games with the same seed
	seedRnd := rand.New(rand.NewSource(state.Seed))
	// distinct randomness across games with same seed
	randRnd := rand.New(rand.NewSource(state.Seed * int64(state.PermIndex+1)))

	game := &Game{
		ID:             id,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		StartingTeam:   Team(randRnd.Intn(2)) + Red,
		Words:          make([]string, 0, wordsPerGame),
		Layout:         make([]Team, 0, wordsPerGame),
		GameState:      state,
		RoundStartedAt: time.Now(),
		GameOptions:    opts,
	}

	// In hard mode, pick a random assassin word and fill the board with its
	// nearest semantic neighbors. The chosen word is forced onto the Black
	// tile below. Fall back to the normal random board if a suitable cluster
	// can't be built (e.g. missing neighbor data).
	var blackWord string
	var clustered []string
	hardMode := opts.HardMode
	if hardMode {
		// Use randRnd so paginated "next game"s with the same seed but a
		// different PermIndex produce different boards.
		blackWord, clustered, hardMode = hardModeWords(randRnd, state.WordSet, neighbors)
	}

	if !hardMode {
		// Pick the next `wordsPerGame` words from the
		// randomly generated permutation
		perm := seedRnd.Perm(len(state.WordSet))
		permIndex := state.PermIndex
		for _, i := range perm[permIndex : permIndex+wordsPerGame] {
			w := state.WordSet[perm[i]]
			game.Words = append(game.Words, w)
		}
	}

	// Pick a random permutation of team assignments.
	var teamAssignments []Team
	teamAssignments = append(teamAssignments, Red.Repeat(8)...)
	teamAssignments = append(teamAssignments, Blue.Repeat(8)...)
	teamAssignments = append(teamAssignments, Neutral.Repeat(7)...)
	teamAssignments = append(teamAssignments, Black)
	teamAssignments = append(teamAssignments, game.StartingTeam)

	shuffleCount := randRnd.Intn(5) + 5
	for i := 0; i < shuffleCount; i++ {
		shuffle(randRnd, teamAssignments)
	}
	game.Layout = teamAssignments

	if hardMode {
		// Place the chosen assassin on the Black tile and the clustered
		// neighbors on every other tile, preserving the random role layout.
		game.Words = make([]string, wordsPerGame)
		var next int
		for i, t := range game.Layout {
			if t == Black {
				game.Words[i] = blackWord
				continue
			}
			game.Words[i] = clustered[next]
			next++
		}
	}
	return game
}

// hardModeWords picks a random assassin word from words and returns it along
// with wordsPerGame-1 of its nearest neighbors (also drawn from words). It
// samples loosely from a pool of the closest neighbors so boards vary across
// games. ok is false if no word has enough in-set neighbors to fill a board.
func hardModeWords(seedRnd *rand.Rand, words []string, neighbors map[string][]string) (black string, others []string, ok bool) {
	const needed = wordsPerGame - 1
	if len(neighbors) == 0 {
		return "", nil, false
	}

	inSet := make(map[string]bool, len(words))
	for _, w := range words {
		inSet[w] = true
	}

	// Candidate assassins are words with at least `needed` neighbors that are
	// also present in the current word set.
	var candidates []string
	filtered := make(map[string][]string, len(words))
	for _, w := range words {
		var ns []string
		for _, n := range neighbors[w] {
			if n != w && inSet[n] {
				ns = append(ns, n)
			}
		}
		filtered[w] = ns
		if len(ns) >= needed {
			candidates = append(candidates, w)
		}
	}
	if len(candidates) == 0 {
		return "", nil, false
	}

	black = candidates[seedRnd.Intn(len(candidates))]

	// Draw from a pool of the closest neighbors for variety. The pool is at
	// least `needed` large so we can always fill the board.
	pool := filtered[black]
	poolSize := needed * 2
	if poolSize > len(pool) {
		poolSize = len(pool)
	}
	pool = pool[:poolSize]

	perm := seedRnd.Perm(len(pool))
	chosen := make([]string, 0, needed)
	for _, i := range perm[:needed] {
		chosen = append(chosen, pool[i])
	}
	return black, chosen, true
}

func shuffle(rnd *rand.Rand, teamAssignments []Team) {
	for i := range teamAssignments {
		j := rnd.Intn(i + 1)
		teamAssignments[i], teamAssignments[j] = teamAssignments[j], teamAssignments[i]
	}
}
