package viinput

// Mode represents the current vi-style input state.
type Mode int

const (
	// ModeInsert accepts normal text entry and cursor editing keys.
	ModeInsert Mode = iota
	// ModeNormal accepts vi-style commands.
	ModeNormal
)

// Model is the state container for the vi-style input component.
type Model struct {
	Prompt    string
	runes     []rune
	cursor    int
	mode      Mode
	focused   bool
	pending   rune
	history   [][]rune
	wantsExit bool
}

// New returns a Model initialized for insert mode.
func New() Model {
	return Model{mode: ModeInsert}
}
