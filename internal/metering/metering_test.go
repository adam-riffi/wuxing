package metering

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
	"github.com/adam-riffi/wuxing/internal/storage"
	"github.com/adam-riffi/wuxing/internal/tools/ai"
	"github.com/adam-riffi/wuxing/internal/tools/connectors"
)

func factStore(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "wuxing.db"))
	if err != nil {
		t.Fatalf("open fact store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func stamp() lineage.Stamp {
	return lineage.NewSequence("seq").WithRun("run", 0).WithSession("sess", 0).WithCall("call", 0)
}

func count(t *testing.T, db *storage.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestStoreMeter_PersistsAllFactTypes(t *testing.T) {
	db := factStore(t)
	m := NewStoreMeter(db)
	st := stamp()

	m.Crossing(connectors.Crossing{Stamp: st, Target: "mtg.cards", Direction: "out", Rows: 3})
	m.Mutation(connectors.Mutation{Stamp: st, Target: "mtg.cards", RowsInserted: 3})
	m.Call(ai.CallFact{Stamp: st, Model: "gpt-x", Mode: "infer", Cost: 0.002, TurnCount: 1})

	if c := count(t, db, "connector_ft_crossing"); c != 1 {
		t.Errorf("crossing rows: got %d want 1", c)
	}
	if c := count(t, db, "wuxing_ft_data_mutation"); c != 1 {
		t.Errorf("mutation rows: got %d want 1", c)
	}
	if c := count(t, db, "ai_ft_call"); c != 1 {
		t.Errorf("ai_ft_call rows: got %d want 1", c)
	}

	// The lineage stamp linked the fact to the spine.
	var seq string
	_ = db.QueryRow(`SELECT sequence_id FROM connector_ft_crossing WHERE call_id = 'call'`).Scan(&seq)
	if seq != "seq" {
		t.Errorf("crossing not stamped with the sequence: %q", seq)
	}
}

func TestStoreMeter_ConnectorWritePersistsFacts(t *testing.T) {
	factDB := factStore(t)

	domain, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "mtg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = domain.Close() }()
	if _, err := domain.Exec(`CREATE TABLE cards (name TEXT)`); err != nil {
		t.Fatal(err)
	}

	c := connectors.New(domain, []connectors.Grant{{Database: "mtg", Table: "cards"}}, NewStoreMeter(factDB))
	payload := json.RawMessage(`{"target":"mtg.cards","rows":[{"name":"Sol Ring"}]}`)
	ret := c.Handler()(context.Background(), bus.NewCall(stamp(), "connectors", "write", payload))
	if ret.Error != "" {
		t.Fatalf("write: %s", ret.Error)
	}

	// The write minted a crossing and a mutation in the fact store, stamped with
	// the call's lineage.
	if c := count(t, factDB, "connector_ft_crossing"); c != 1 {
		t.Errorf("crossing rows: got %d want 1", c)
	}
	var inserted int
	_ = factDB.QueryRow(`SELECT rows_inserted FROM wuxing_ft_data_mutation WHERE call_id = 'call'`).Scan(&inserted)
	if inserted != 1 {
		t.Errorf("mutation rows_inserted: got %d want 1", inserted)
	}
}
