package config

// This file fixes the package's exported API so that other packages can build
// against it. Delete it once everything is implemented in its own file.

// File names in the support folder.
const (
	ConfigFile = "config.toml"
	BackupFile = "config.toml.bak"
	StateFile  = "state.json"
	BadState   = "state.json.bad"
	ParkedFile = "parked.json" // Parked windows, for restoring after a crash.
)

// Dir returns Bench's support folder: $BENCH_HOME if it's set, otherwise
// "Bench" in os.UserConfigDir() (~/Library/Application Support/Bench on a
// Mac). It doesn't create it.
func Dir() (string, error) { panic("not implemented") }

// Default returns the default config: the one in the spec, with three tabs
// (Comms, Code, Research) and the Brave picture-in-picture rule.
func Default() *Config { panic("not implemented") }

// DefaultTOML returns the default config as Bench writes it on first run,
// with a comment on each setting.
func DefaultTOML() []byte { panic("not implemented") }

// Parse reads config.toml. Missing settings and shortcuts take their default
// values; missing tabs don't (a file with no [[tab]] is a problem). Syntax
// errors and unknown fields are returned as problems (with the tab and field
// where possible), as are validation problems; err is only for failures that
// aren't the file's fault. If there are problems, cfg is nil.
func Parse(data []byte) (cfg *Config, problems []Problem, err error) { panic("not implemented") }

// Load reads and parses a config file. If the file doesn't exist, it writes
// DefaultTOML there (creating the folder) and returns the default config.
func Load(path string) (cfg *Config, problems []Problem, err error) { panic("not implemented") }

// Validate checks a config and returns every problem: an unknown preset or
// small-screen setting; more panes than the preset has; sizes that don't
// match the preset's top-level split; an item with none, or more than one, of
// app, web, vscode, and terminal; fields that don't apply to an item's kind;
// a web address that isn't https; an empty or duplicate tab name; a rule with
// no app, with both or neither of float and tab, or naming a tab that doesn't
// exist; an unknown new_windows or follow_focus value; a non-positive
// compact_below; a negative gap; a non-positive launch_timeout; and
// shortcuts that don't parse.
func (c *Config) Validate() []Problem { panic("not implemented") }

// Format writes a config as TOML in the layout of the default file: comments
// on settings, [shortcuts], each [[tab]] with its [[tab.pane]] entries and
// inline item tables, then the [[rule]] entries. Parse(Format(c)) returns a
// config equal to c.
func Format(c *Config) []byte { panic("not implemented") }

// Save validates a config and writes it to path atomically, first copying
// any existing file to config.toml.bak in the same folder. It refuses to
// save a config with problems.
func Save(path string, c *Config) error { panic("not implemented") }

// Clone returns a deep copy of the config.
func (c *Config) Clone() *Config { panic("not implemented") }

// TabIndex returns the index of the tab with the given name, or -1.
func (c *Config) TabIndex(name string) int { panic("not implemented") }

// LoadState reads state.json and checks it against the config: it must
// parse, have version StateVersion, name the config's tabs in the same order,
// and have valid trees whose items all exist in Items. If it doesn't, the file
// is renamed to state.json.bad and LoadState returns (nil, nil). A missing
// file also returns (nil, nil).
func LoadState(path string, cfg *Config) (*State, error) { panic("not implemented") }

// SaveState writes state.json atomically.
func SaveState(path string, s *State) error { panic("not implemented") }
