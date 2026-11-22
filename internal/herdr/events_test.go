package herdr

import (
	"bufio"
	"strings"
	"testing"
)

// Shapes as `hc events --json` prints them (hc 0.1.21, schema/README.md).
const stream = `{"channel":null,"seq":10,"terminals":[{"address":"t1","state":"live","created_ms":2,"status":{"app":"claude-code","state":"working"},"status_ms":100,"title":"a","labels":{"where":"s / 1"}},{"address":"t2","state":"live","created_ms":1,"parent":"t1","status":null,"title":""}]}
{"jsonrpc":"2.0","method":"heartbeat","params":{"seq":10}}
{"jsonrpc":"2.0","method":"event","params":{"seq":11,"type":"status","terminal":"t1","data":{"status":{"app":"claude-code","state":"done"},"records":{},"reported":true}}}
{"jsonrpc":"2.0","method":"event","params":{"seq":12,"type":"title","terminal":"t1","data":{"title":"b"}}}
{"jsonrpc":"2.0","method":"event","params":{"seq":13,"type":"status","terminal":"t1","data":{"status":{"app":"claude-code","state":"done","msg":"Run tests"}}}}
{"jsonrpc":"2.0","method":"missed","params":{"seq":20,"terminals":[{"address":"t1","state":"live","title":"c"}]}}
{"jsonrpc":"2.0","method":"event","params":{"seq":19,"type":"title","terminal":"t1","data":{"title":"stale"}}}
{"jsonrpc":"2.0","method":"event","params":{"seq":21,"type":"labels","terminal":"t1","data":{"labels":{"herdr.pinned":"1"}}}}
{"jsonrpc":"2.0","method":"event","params":{"seq":22,"type":"created","terminal":"t3","data":{"address":"t3","state":"live","parent":"t1"}}}
{"jsonrpc":"2.0","method":"event","params":{"seq":23,"type":"removed","terminal":"t3","data":{}}}
`

func TestReadEventsApply(t *testing.T) {
	ts := Terminals{}
	var got []string
	beats := 0
	err := readEvents(bufio.NewReader(strings.NewReader(stream)), func(ev Event) {
		changed, moved, err := ts.Apply(ev, 999)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ev.Type)
		switch ev.Seq {
		case 10:
			if len(ts) != 2 || ts["t2"].ParentAddr() != "t1" || ts["t1"].Labels["where"] != "s / 1" {
				t.Errorf("snapshot: %+v", ts)
			}
			if l := ts.Sorted(); l[0].Address != "t2" {
				t.Errorf("Sorted: oldest first, got %s", l[0].Address)
			}
		case 11:
			if tm := ts["t1"]; tm.StatusState() != "done" || tm.StatusMS != 999 || !changed || moved {
				t.Errorf("status: %+v changed=%v moved=%v", tm, changed, moved)
			}
		case 12:
			if ts["t1"].Title != "b" {
				t.Errorf("title: %q", ts["t1"].Title)
			}
			ts["t1"].StatusMS = 5
		case 13:
			// only the message changed: a change, but the status didn't start again
			if tm := ts["t1"]; tm.StatusMsg() != "Run tests" || tm.StatusMS != 5 || !changed || moved {
				t.Errorf("status msg: %+v changed=%v moved=%v", tm, changed, moved)
			}
		case 20:
			if len(ts) != 1 || ts["t1"].Title != "c" {
				t.Errorf("missed snapshot: %+v", ts)
			}
		case 21:
			if ts["t1"].Labels["herdr.pinned"] != "1" || !moved {
				t.Errorf("labels: %+v moved=%v", ts["t1"].Labels, moved)
			}
		case 22:
			if ts["t3"] == nil || !moved {
				t.Errorf("created: %+v", ts)
			}
		case 23:
			if ts["t3"] != nil || !moved {
				t.Errorf("removed: %+v", ts)
			}
		}
	}, func() { beats++ })
	if err == nil || err.Error() != "EOF" {
		t.Errorf("err = %v, want EOF", err)
	}
	want := "snapshot status title status snapshot labels created removed" // seq 19 dropped
	if g := strings.Join(got, " "); g != want {
		t.Errorf("events = %q, want %q", g, want)
	}
	if beats != 1 {
		t.Errorf("beats = %d", beats)
	}
}

func TestReadEventsError(t *testing.T) {
	in := `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"not_found"}}` + "\n"
	err := readEvents(bufio.NewReader(strings.NewReader(in)), func(Event) {}, func() {})
	if err == nil || err.Error() != "not_found" {
		t.Errorf("err = %v", err)
	}
}

func TestProgram(t *testing.T) {
	tm := Terminal{Argv: []string{`C:\x\pwsh.exe`}}
	if tm.Program() != `C:\x\pwsh.exe` {
		t.Error(tm.Program())
	}
	tm.Status = &TermStatus{State: "idle", App: "claude-code"}
	if tm.Program() != "claude-code" {
		t.Error(tm.Program())
	}
}
