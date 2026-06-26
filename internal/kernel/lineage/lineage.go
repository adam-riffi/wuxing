// Package lineage assigns the metadata wuxing stamps onto every event: the
// case-file ids (sequence → run → session → call → tool_function) and the order
// fields that record position within each parent.
//
// Order is explicit, never inferred from clocks — parallel steps share a
// timestamp and clocks have resolution limits, so "what triggered what" is
// recorded as integer rank, not time. The kernel is the only thing that sees the
// whole picture, so it mints these values from the outside; the observed service
// never authors its own lineage and so cannot forge it.
//
// A Stamp is an immutable value. The With* helpers derive a child stamp for the
// next level down, resetting the deeper levels so a stamp always describes one
// coherent position in the hierarchy.
package lineage

// SequenceID identifies a causal chain of runs linked by event triggers.
type SequenceID string

// RunID identifies one triggered execution — one service, end to end.
type RunID string

// SessionID identifies a unit of work within a run.
type SessionID string

// CallID identifies a tool invocation within a session.
type CallID string

// ToolFunctionID identifies an internal step within a call.
type ToolFunctionID string

// Stamp is the full lineage stamp carried on an event.
//
// Each order field is the position of a level within its parent, and is named
// after that parent (matching the spine tables in the data model):
//
//	SequenceOrder — this run's position within its sequence
//	RunOrder      — this session's position within its run
//	CallOrder     — this call's position within its session
//	StepOrder     — this tool_function's position within its call
//
// The sequence itself is the chain head and has no order field. Filtering by
// Sequence and ordering by (SequenceOrder, RunOrder, CallOrder, StepOrder)
// replays an entire cascade in causal order with no timestamp involved.
type Stamp struct {
	Sequence     SequenceID     `json:"sequence_id"`
	Run          RunID          `json:"run_id,omitempty"`
	Session      SessionID      `json:"session_id,omitempty"`
	Call         CallID         `json:"call_id,omitempty"`
	ToolFunction ToolFunctionID `json:"tool_function_id,omitempty"`

	SequenceOrder int `json:"sequence_order"`
	RunOrder      int `json:"run_order"`
	CallOrder     int `json:"call_order"`
	StepOrder     int `json:"step_order"`
}

// NewSequence returns a stamp at the head of a fresh causal chain.
func NewSequence(id SequenceID) Stamp {
	return Stamp{Sequence: id}
}

// WithRun derives a stamp for a new run at the given position within the
// sequence, clearing every deeper level.
func (s Stamp) WithRun(id RunID, order int) Stamp {
	s.Run, s.SequenceOrder = id, order
	s.Session, s.Call, s.ToolFunction = "", "", ""
	s.RunOrder, s.CallOrder, s.StepOrder = 0, 0, 0
	return s
}

// WithSession derives a stamp for a new session at the given position within the
// run, clearing every deeper level.
func (s Stamp) WithSession(id SessionID, order int) Stamp {
	s.Session, s.RunOrder = id, order
	s.Call, s.ToolFunction = "", ""
	s.CallOrder, s.StepOrder = 0, 0
	return s
}

// WithCall derives a stamp for a new call at the given position within the
// session, clearing every deeper level.
func (s Stamp) WithCall(id CallID, order int) Stamp {
	s.Call, s.CallOrder = id, order
	s.ToolFunction = ""
	s.StepOrder = 0
	return s
}

// WithStep derives a stamp for a new tool_function at the given position within
// the call.
func (s Stamp) WithStep(id ToolFunctionID, order int) Stamp {
	s.ToolFunction, s.StepOrder = id, order
	return s
}
