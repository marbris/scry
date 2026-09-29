package ui

import (
	"os"
	"strings"
	"testing"

	"ttr/internal/keymap"
)

// rebind puts a keys.json in force for one test.
func rebind(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(keymap.Path(), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Remove(keymap.Path())
		keymap.Reset()
	})
	if err := keymap.Load(); err != nil {
		t.Fatal(err)
	}
}

// offered is the key text the hints show for a label, or "" if none does.
func offered(m Model, label string) string {
	for _, g := range m.hintGroups() {
		for _, k := range g.keys {
			if k[1] == label {
				return k[0]
			}
		}
	}
	return ""
}

func TestARebindingMovesTheKeyAndItsHint(t *testing.T) {
	rebind(t, `{"cards": {"sort1.next": ["z"], "sort1.prev": ["Z"]}}`)

	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	m = drive(m, ".")
	if got := m.ws.current().cardsView().order; got != sortArrival {
		t.Errorf(". still sorts after being rebound: %v", got)
	}
	m = drive(m, "z")
	if got := m.ws.current().cardsView().order; got != sortMana {
		t.Errorf("z moved to %v, want mana value", got)
	}
	if got := offered(m, "sort 1 & 2"); got != "z Z , <" {
		t.Errorf("the hint says %q, want %q", got, "z Z , <")
	}
}

func TestAReboundLeaderIsTheLeaderEverywhere(t *testing.T) {
	// The fixture opens its panel with space, so it is built first.
	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	rebind(t, `{"global": {"leader": [";"]}}`)

	if m = drive(m, "space"); m.leader {
		t.Error("space is still the leader")
	}
	if m = drive(m, ";"); !m.leader {
		t.Fatal("; did not raise the menu")
	}
	// The menu ? draws along the bottom is led by the leader in force.
	if got := stripANSI(strings.Join(m.leaderReference(200), " ")); !strings.HasPrefix(got, ";:") {
		t.Errorf("the menu's reference reads %q, want it led by ;", got)
	}
}

func TestAnUnboundKeyLeavesNoHint(t *testing.T) {
	rebind(t, `{"cards": {"tag": []}}`)

	m := withCards(sized(140, 30), "f", sample(), sortArrival)
	for _, g := range m.hintGroups() {
		for _, k := range g.keys {
			if k[0] == "" {
				t.Errorf("%q is offered with no key", k[1])
			}
			if k[1] == "tag" {
				t.Errorf("tag is offered as %q after being unbound", k[0])
			}
		}
	}
}
