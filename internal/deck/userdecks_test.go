package deck

import (
	"testing"
	"time"
)

func TestUserDecksRoundTrip(t *testing.T) {
	isolate(t)

	if _, ok := CachedUserDecks(LoadUserDecks(), "MarBri"); ok {
		t.Fatal("a cache out of nowhere")
	}
	if err := SaveUserDecks("MarBri", []UserDeck{{Name: "Elf Ball", ID: "a", Cards: 100}}); err != nil {
		t.Fatal(err)
	}
	// Names are matched however they're cased.
	got, ok := CachedUserDecks(LoadUserDecks(), "marbri")
	if !ok || len(got.Decks) != 1 || got.Decks[0].Name != "Elf Ball" {
		t.Fatalf("got %+v", got)
	}
	if got.Stale() {
		t.Error("a list fetched just now is stale")
	}
	if !(UserDeckList{Fetched: time.Now().Add(-48 * time.Hour)}).Stale() {
		t.Error("a two-day-old list isn't stale")
	}

	if err := ForgetUserDecks("MARBRI"); err != nil {
		t.Fatal(err)
	}
	if _, ok := CachedUserDecks(LoadUserDecks(), "MarBri"); ok {
		t.Error("forgetting kept the list")
	}
}
