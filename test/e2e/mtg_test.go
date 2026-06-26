package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/adam-riffi/wuxing/internal/contracts/cfg"
	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/interpreter"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/kernel/triggers"
	"github.com/adam-riffi/wuxing/internal/tools/ai"
	"github.com/adam-riffi/wuxing/internal/tools/connectors"
)

// --- test doubles -----------------------------------------------------------

type connRec struct {
	crossings []connectors.Crossing
	mutations []connectors.Mutation
}

func (r *connRec) Crossing(c connectors.Crossing) { r.crossings = append(r.crossings, c) }
func (r *connRec) Mutation(m connectors.Mutation) { r.mutations = append(r.mutations, m) }

type aiRec struct{ facts []ai.CallFact }

func (r *aiRec) Call(f ai.CallFact) { r.facts = append(r.facts, f) }

// orderedAI returns canned responses in order: the mtg checker fact first, then
// the notifier's composed message.
type orderedAI struct {
	responses []string
	i         int
}

func (o *orderedAI) Infer(_ context.Context, _ ai.Request) (ai.Result, error) {
	out := o.responses[o.i]
	o.i++
	return ai.Result{Output: json.RawMessage(out), Model: "fake", Cost: 0.001, TokensIn: 5, TokensOut: 7}, nil
}

// --- the worked example -----------------------------------------------------

// TestMTGNewSetNotifier wires the whole substrate: an external trigger opens a
// sequence, the interpreter drives the mtg workflow (ai check -> branch ->
// connector write), the emitted fact fires the notifier via triggers inheriting
// the sequence, and ai composes. It asserts one sequence_id across the cascade
// plus the tool facts. Real containers await the Docker engine; this exercises
// the Go substrate end to end.
func TestMTGNewSetNotifier(t *testing.T) {
	ctx := context.Background()

	// Domain database with the cards table the connector writes to.
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "mtg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE cards (name TEXT, set_code TEXT)`); err != nil {
		t.Fatal(err)
	}

	connMeter := &connRec{}
	aiMeter := &aiRec{}
	aiBackend := &orderedAI{responses: []string{
		`{"new_set":true,"rows":[{"name":"Sol Ring","set_code":"C21"}]}`,
		`"A new MTG set just dropped: C21"`,
	}}

	b := bus.New()
	if err := b.Register("connectors", connectors.New(db, []connectors.Grant{{Database: "mtg", Table: "cards"}}, connMeter).Handler()); err != nil {
		t.Fatal(err)
	}
	if err := b.Register("ai", ai.New(aiBackend, aiMeter).Handler()); err != nil {
		t.Fatal(err)
	}

	minter := lineage.NewMinter()
	interp := interpreter.New(b, cfg.DefaultVocabulary(), minter)

	var fired []triggers.Run
	trig := triggers.New(minter, func(r triggers.Run) { fired = append(fired, r) })
	trig.RegisterEvent("notifier", "mtg-db updated")

	mtg := &cfg.Service{
		Name: "mtg",
		Workflow: []cfg.Step{
			{ID: "check", Tool: "ai", Operation: "infer", With: map[string]any{"prompt": "check scryfall"}, Branch: []cfg.Branch{{When: "new_set == true", Goto: "write"}}},
			// The checker emits the rows; the write step declares its target.
			{ID: "write", Tool: "connectors", Operation: "write", With: map[string]any{"target": "mtg.cards"}},
		},
		Successors: []cfg.Successor{{Service: "notifier", Topic: "mtg-db updated", When: "new_set == true"}},
	}

	// External (cron) trigger opens a NEW sequence for the mtg run.
	mtgRun := trig.FireExternal("mtg", triggers.KindCron)
	mtgStamp := mtgRun.Stamp.WithSession(minter.SessionID(), 0)

	fact, err := interp.Run(ctx, mtg, mtgStamp)
	if err != nil {
		t.Fatalf("mtg run: %v", err)
	}
	if fact["new_set"] != true || fact["rows_inserted"] != float64(1) {
		t.Errorf("mtg emitted fact: %+v", fact)
	}

	// The connector wrote the card and minted both facts.
	var cards int
	_ = db.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&cards)
	if cards != 1 {
		t.Errorf("cards written: got %d want 1", cards)
	}
	if len(connMeter.crossings) != 1 || connMeter.crossings[0].Direction != "out" {
		t.Errorf("connector crossing: %+v", connMeter.crossings)
	}
	if len(connMeter.mutations) != 1 || connMeter.mutations[0].RowsInserted != 1 {
		t.Errorf("connector mutation: %+v", connMeter.mutations)
	}

	// The emitted fact fires the notifier successor.
	succ, err := interp.Successors(mtg, fact)
	if err != nil {
		t.Fatal(err)
	}
	if len(succ) != 1 || succ[0].Service != "notifier" {
		t.Fatalf("successors: %+v", succ)
	}

	// Triggers fire the notifier inheriting the sequence (the event carries it).
	notifierRuns := trig.OnEvent(succ[0].Topic, mtgStamp)
	if len(notifierRuns) != 1 {
		t.Fatalf("notifier runs: %d", len(notifierRuns))
	}
	nrun := notifierRuns[0]

	// THE cascade invariant: one sequence_id across both runs, order incremented.
	if nrun.Stamp.Sequence != mtgStamp.Sequence {
		t.Errorf("sequence not inherited: mtg %q vs notifier %q", mtgStamp.Sequence, nrun.Stamp.Sequence)
	}
	if nrun.Stamp.SequenceOrder != mtgStamp.SequenceOrder+1 {
		t.Errorf("notifier sequence_order: got %d want %d", nrun.Stamp.SequenceOrder, mtgStamp.SequenceOrder+1)
	}
	if nrun.Stamp.Run == mtgStamp.Run {
		t.Error("notifier must get a fresh run id")
	}

	// Run the notifier (ai compose).
	notifier := &cfg.Service{
		Name:     "notifier",
		Workflow: []cfg.Step{{ID: "compose", Tool: "ai", Operation: "infer"}},
	}
	notifStamp := nrun.Stamp.WithSession(minter.SessionID(), 0)
	if _, err := interp.Run(ctx, notifier, notifStamp); err != nil {
		t.Fatalf("notifier run: %v", err)
	}

	// Both ai calls recorded cost at incur-time.
	if len(aiMeter.facts) != 2 {
		t.Errorf("ai cost facts: got %d want 2", len(aiMeter.facts))
	}
	for _, f := range aiMeter.facts {
		if f.Cost <= 0 {
			t.Errorf("ai cost not recorded: %+v", f)
		}
	}
}
