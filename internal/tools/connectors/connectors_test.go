package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/adam-riffi/wuxing/internal/kernel/bus"
	"github.com/adam-riffi/wuxing/internal/kernel/lineage"
)

type recMeter struct {
	crossings []Crossing
	mutations []Mutation
}

func (m *recMeter) Crossing(c Crossing) { m.crossings = append(m.crossings, c) }
func (m *recMeter) Mutation(x Mutation) { m.mutations = append(m.mutations, x) }

func openDomainDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "mtg.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE cards (name TEXT, set_code TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func stamp() lineage.Stamp { return lineage.NewSequence("seq").WithRun("run", 0) }

func writeCall(target, rows string) bus.Envelope {
	payload := json.RawMessage(`{"target":"` + target + `","rows":` + rows + `}`)
	return bus.NewCall(stamp(), "connectors", "write", payload)
}

func TestConnector_WriteAllowed(t *testing.T) {
	db := openDomainDB(t)
	meter := &recMeter{}
	c := New(db, []Grant{{Database: "mtg", Table: "cards"}}, meter)
	h := c.Handler()

	ret := h(context.Background(), writeCall("mtg.cards", `[{"name":"Sol Ring","set_code":"C21"},{"name":"Counterspell","set_code":"MH2"}]`))
	if ret.Error != "" {
		t.Fatalf("write errored: %s", ret.Error)
	}
	var res map[string]int
	_ = json.Unmarshal(ret.Payload, &res)
	if res["rows_inserted"] != 2 {
		t.Errorf("rows_inserted: got %d want 2", res["rows_inserted"])
	}

	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&n)
	if n != 2 {
		t.Errorf("rows in table: got %d want 2", n)
	}

	if len(meter.crossings) != 1 || meter.crossings[0].Direction != "out" || meter.crossings[0].Rows != 2 {
		t.Errorf("crossing fact: %+v", meter.crossings)
	}
	if len(meter.mutations) != 1 || meter.mutations[0].RowsInserted != 2 {
		t.Errorf("mutation fact: %+v", meter.mutations)
	}
}

func TestConnector_WriteRefusedWithoutGrant(t *testing.T) {
	db := openDomainDB(t)
	meter := &recMeter{}
	// Grant is for mtg.cards; a write to mtg.users must be refused.
	c := New(db, []Grant{{Database: "mtg", Table: "cards"}}, meter)
	h := c.Handler()

	ret := h(context.Background(), writeCall("mtg.users", `[{"name":"x","set_code":"y"}]`))
	if ret.Error == "" {
		t.Fatal("write to an ungranted target should be refused")
	}
	if len(meter.crossings) != 0 || len(meter.mutations) != 0 {
		t.Error("a refused write must emit no facts and touch no rows")
	}
}

func TestConnector_ReadRoundTrip(t *testing.T) {
	db := openDomainDB(t)
	meter := &recMeter{}
	c := New(db, []Grant{{Database: "mtg", Table: "cards"}}, meter)
	h := c.Handler()

	_ = h(context.Background(), writeCall("mtg.cards", `[{"name":"Sol Ring","set_code":"C21"}]`))

	readPayload := json.RawMessage(`{"target":"mtg.cards"}`)
	ret := h(context.Background(), bus.NewCall(stamp(), "connectors", "read", readPayload))
	if ret.Error != "" {
		t.Fatalf("read errored: %s", ret.Error)
	}
	var res struct {
		Rows []map[string]any `json:"rows"`
	}
	_ = json.Unmarshal(ret.Payload, &res)
	if len(res.Rows) != 1 || res.Rows[0]["name"] != "Sol Ring" {
		t.Errorf("read rows: %+v", res.Rows)
	}
	if len(meter.crossings) == 0 || meter.crossings[len(meter.crossings)-1].Direction != "in" {
		t.Errorf("read should emit an inbound crossing: %+v", meter.crossings)
	}
}

func TestConnector_UnknownOperation(t *testing.T) {
	c := New(openDomainDB(t), nil, nil)
	ret := c.Handler()(context.Background(), bus.NewCall(stamp(), "connectors", "drop", nil))
	if ret.Error == "" {
		t.Error("unknown operation should error")
	}
}

func TestConnector_RegistersOnBus(t *testing.T) {
	db := openDomainDB(t)
	c := New(db, []Grant{{Database: "mtg", Table: "cards"}}, nil)
	b := bus.New()
	if err := b.Register("connectors", c.Handler()); err != nil {
		t.Fatal(err)
	}
	ret, err := b.Call(context.Background(), writeCall("mtg.cards", `[{"name":"a","set_code":"b"}]`))
	if err != nil || ret.Error != "" {
		t.Fatalf("end-to-end write over the bus failed: err=%v ret.Error=%s", err, ret.Error)
	}
}
