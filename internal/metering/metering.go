// Package metering bridges the tools' fact emission to the storage detail
// tables. A single StoreMeter implements both connectors.Meter and ai.Meter, so
// the kernel can hand one meter to every tool and have crossings, mutations, and
// ai calls land in the fact store keyed by their lineage stamp.
package metering

import (
	"context"

	"github.com/adam-riffi/wuxing/internal/storage"
	"github.com/adam-riffi/wuxing/internal/storage/facts"
	"github.com/adam-riffi/wuxing/internal/tools/ai"
	"github.com/adam-riffi/wuxing/internal/tools/connectors"
)

// StoreMeter persists tool facts to the storage detail tables. Fact emission is
// best-effort observability on the side of the work, so write errors are
// swallowed rather than failing the tool call.
type StoreMeter struct {
	detail *facts.Detail
}

// NewStoreMeter returns a meter that writes to db's detail tables.
func NewStoreMeter(db *storage.DB) *StoreMeter {
	return &StoreMeter{detail: facts.NewDetail(db)}
}

// Crossing implements connectors.Meter.
func (m *StoreMeter) Crossing(c connectors.Crossing) {
	_ = m.detail.RecordCrossing(context.Background(), c.Stamp, "sqlite", c.Target, c.Direction, c.Rows, 0, 0)
}

// Mutation implements connectors.Meter.
func (m *StoreMeter) Mutation(x connectors.Mutation) {
	_ = m.detail.RecordMutation(context.Background(), x.Stamp, x.Target, x.RowsInserted, 0, 0)
}

// Call implements ai.Meter.
func (m *StoreMeter) Call(f ai.CallFact) {
	_ = m.detail.RecordAICall(context.Background(), f.Stamp, f.Model, f.Mode, f.TokensIn, f.TokensOut, f.Cost, f.TTFTMs, f.TurnCount)
}

// Ensure StoreMeter satisfies both tool meter interfaces.
var (
	_ connectors.Meter = (*StoreMeter)(nil)
	_ ai.Meter         = (*StoreMeter)(nil)
)
