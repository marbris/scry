package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Browsing someone's decks on Moxfield, so importing one doesn't mean
// finding it in a browser first and pasting the URL back.
//
// There is no public endpoint for "this user's decks"; the deck search takes
// an authorUserNames filter instead, which comes to the same thing. Only
// public decks are ever returned — a private or unlisted deck isn't visible
// to anyone not signed in, including us.

const (
	moxSearchURL = "https://api2.moxfield.com/v2/decks/search" +
		"?pageNumber=%d&pageSize=%d&sortType=updated&sortDirection=descending" +
		"&authorUserNames=%s"

	// One screen's worth and then some; a page beyond this is a scroll
	// nobody makes looking for a deck they already know the name of.
	moxUserPageSize = 60
)

// moxUserDeck is one deck in someone's list. Enough to choose by without
// fetching any of them.
type moxUserDeck struct {
	Name      string   `json:"name"`
	Format    string   `json:"format"`
	PublicID  string   `json:"publicId"`
	PublicURL string   `json:"publicUrl"`
	Cards     int      `json:"mainboardCount"`
	Updated   string   `json:"lastUpdatedAtUtc"`
	Colors    []string `json:"colorIdentity"`
	CreatedBy struct {
		UserName string `json:"userName"`
	} `json:"createdByUser"`
}

// age is how long ago the deck last changed, in the shape git uses.
func (d moxUserDeck) age() string {
	t, err := time.Parse(time.RFC3339, d.Updated)
	if err != nil {
		return ""
	}
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days < 1:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 30:
		return fmt.Sprintf("%d days ago", days)
	case days < 365:
		return plural(fmt.Sprintf("%d month", days/30), days/30) + " ago"
	}
	return plural(fmt.Sprintf("%d year", days/365), days/365) + " ago"
}

type moxUserDecksMsg struct {
	user  string
	decks []moxUserDeck
	err   error
}

func fetchMoxUserDecksCmd(user string) tea.Cmd {
	return func() tea.Msg {
		name, decks, err := fetchMoxUserDecks(user)
		return moxUserDecksMsg{user: name, decks: decks, err: err}
	}
}

// fetchMoxUserDecks lists someone's public decks, newest change first. The
// name it comes back with is Moxfield's own spelling of it, which is what a
// pasted profile URL or a differently-cased guess should end up showing.
func fetchMoxUserDecks(user string) (string, []moxUserDeck, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return "", nil, fmt.Errorf("no user name")
	}
	// A pasted profile URL is a reasonable thing to hand this.
	if u, ok := moxfieldUserName(user); ok {
		user = u
	}

	body, err := doGet(fmt.Sprintf(moxSearchURL, 1, moxUserPageSize, url.QueryEscape(user)))
	if err != nil {
		return "", nil, err
	}

	var page struct {
		TotalResults int           `json:"totalResults"`
		Data         []moxUserDeck `json:"data"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return "", nil, fmt.Errorf("moxfield: %w", err)
	}

	// The search endpoint ignores a filter it doesn't understand and
	// answers with the whole site, so anything not actually by this person
	// is dropped rather than shown as theirs.
	var mine []moxUserDeck
	name := user
	for _, d := range page.Data {
		if strings.EqualFold(d.CreatedBy.UserName, user) {
			name = d.CreatedBy.UserName // Moxfield's own spelling
			mine = append(mine, d)
		}
	}
	if len(mine) == 0 {
		return "", nil, fmt.Errorf("no public decks for %q", user)
	}
	return name, mine, nil
}

// moxfieldUserName pulls the name out of a profile URL, so pasting one works
// as well as typing the name.
func moxfieldUserName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	i := strings.Index(strings.ToLower(s), "moxfield.com/users/")
	if i < 0 {
		return "", false
	}
	name := s[i+len("moxfield.com/users/"):]
	if cut := strings.IndexAny(name, "/?#"); cut >= 0 {
		name = name[:cut]
	}
	if name == "" {
		return "", false
	}
	return name, true
}
