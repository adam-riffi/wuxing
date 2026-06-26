package lineage

import "github.com/google/uuid"

// Minter generates unique lineage ids. The id generator is injectable so tests
// can make ids deterministic; production uses random UUIDs.
type Minter struct {
	newID func() string
}

// NewMinter returns a Minter that mints random UUID v4 ids.
func NewMinter() *Minter {
	return &Minter{newID: uuid.NewString}
}

// NewMinterFunc returns a Minter that mints ids from gen — for deterministic tests.
func NewMinterFunc(gen func() string) *Minter {
	return &Minter{newID: gen}
}

// SequenceID mints a new sequence id.
func (m *Minter) SequenceID() SequenceID { return SequenceID(m.newID()) }

// RunID mints a new run id.
func (m *Minter) RunID() RunID { return RunID(m.newID()) }

// SessionID mints a new session id.
func (m *Minter) SessionID() SessionID { return SessionID(m.newID()) }

// CallID mints a new call id.
func (m *Minter) CallID() CallID { return CallID(m.newID()) }

// ToolFunctionID mints a new tool_function id.
func (m *Minter) ToolFunctionID() ToolFunctionID { return ToolFunctionID(m.newID()) }
