package moxfield

import (
	"scry/internal/fetch"

	"scry/internal/scryfall"

	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Browsing someone's decks on Moxfield, so importing one doesn't mean
// finding it in a browser first and pasting the URL back.
//
// There is no public endpoint for "this user's decks"; the deck search takes
// an authorUserNames filter instead, which comes to the same thing. Only
// public decks are ever returned — a private or unlisted deck isn't visible
// to anyone not signed in, including us.

const (
	// showIllegal is not optional. Without it the search answers with only
	// the decks that are legal in their format, which for anyone who builds
	// decks in the open is a small fraction of them: an account with 42
	// public decks came back with 11, and the 31 it left out were the ones
	// mid-build — 157 cards, or 3, or none yet. Those are precisely the
	// decks you'd open to work on.
	moxSearchURL = "https://api2.moxfield.com/v2/decks/search" +
		"?pageNumber=%d&pageSize=%d&sortType=updated&sortDirection=descending" +
		"&showIllegal=true&authorUserNames=%s"

	// Moxfield serves up to a hundred at a time.
	moxUserPageSize = 100

	// A ceiling on how much of a prolific account to pull down. Five pages
	// is five hundred decks, which is more than anyone will scroll.
	moxUserMaxPages = 5
)

// UserDeck is one deck in someone's list. Enough to choose by without
// fetching any of them.
type UserDeck struct {
	Name      string   `json:"name"`
	Format    string   `json:"format"`
	PublicID  string   `json:"publicId"`
	PublicURL string   `json:"publicUrl"`
	Cards     int      `json:"mainboardCount"`
	Legal     bool     `json:"isLegal"`
	Updated   string   `json:"lastUpdatedAtUtc"`
	Colors    []string `json:"colorIdentity"`
	CreatedBy struct {
		UserName string `json:"userName"`
	} `json:"createdByUser"`
}

// updatedAt is when the deck last changed. A deck Moxfield gave no date for
// sorts as though it were ancient, rather than as though it were touched at
// the epoch of everything else.
func (d UserDeck) UpdatedAt() time.Time {
	t, err := time.Parse(time.RFC3339, d.Updated)
	if err != nil {
		return time.Time{}
	}
	return t
}

// age is how long ago the deck last changed, in the shape git uses.
func (d UserDeck) Age() string {
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

func UserDecks(user string) (string, []UserDeck, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return "", nil, fmt.Errorf("no user name")
	}
	// A pasted profile URL is a reasonable thing to hand this.
	if u, ok := UserName(user); ok {
		user = u
	}

	var mine []UserDeck
	name := user

	for page := 1; page <= moxUserMaxPages; page++ {
		if page > 1 {
			time.Sleep(scryfall.PageDelay)
		}
		body, err := fetch.Get(fmt.Sprintf(moxSearchURL, page, moxUserPageSize, url.QueryEscape(user)))
		if err != nil {
			// A later page failing still leaves the earlier ones worth
			// showing.
			if len(mine) > 0 {
				break
			}
			return "", nil, err
		}

		var p struct {
			TotalPages int        `json:"totalPages"`
			Data       []UserDeck `json:"data"`
		}
		if err := json.Unmarshal(body, &p); err != nil {
			return "", nil, fmt.Errorf("moxfield: %w", err)
		}

		// The search endpoint ignores a filter it doesn't understand and
		// answers with the whole site, so anything not actually by this
		// person is dropped rather than shown as theirs.
		for _, d := range p.Data {
			if strings.EqualFold(d.CreatedBy.UserName, user) {
				name = d.CreatedBy.UserName // Moxfield's own spelling
				mine = append(mine, d)
			}
		}
		if len(p.Data) == 0 || page >= p.TotalPages {
			break
		}
	}

	if len(mine) == 0 {
		return "", nil, fmt.Errorf("no public decks for %q", user)
	}

	sortUserDecks(mine)
	return name, mine, nil
}

// sortUserDecks puts the finished decks first, then the rest, each newest
// first. Most of a builder's decks are half-built, and burying the ones that
// are done under thirty of them makes the list harder to use than it needs
// to be.
func sortUserDecks(decks []UserDeck) {
	sort.SliceStable(decks, func(i, j int) bool {
		if decks[i].Legal != decks[j].Legal {
			return decks[i].Legal
		}
		return decks[i].UpdatedAt().After(decks[j].UpdatedAt())
	})
}

// UserName pulls the name out of a profile URL, so pasting one works
// as well as typing the name.
func UserName(s string) (string, bool) {
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

// plural is a copy of the one in package deck. Six lines duplicated beats
// exporting a grammar helper from a package about decks, or inventing a
// package to hold it.
func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// User accepts either a profile URL or a bare username, since both are
// things people paste. A name is letters, digits and the few punctuation
// marks Moxfield allows — anything with a space or a slash in it was meant
// to be something else and saying so beats following a deck called "not a
// thing".
func User(s string) (string, bool) {
	if name, ok := UserName(s); ok {
		return name, true
	}

	s = strings.TrimSpace(s)
	if s == "" || len(s) > 50 {
		return "", false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return "", false
		}
	}
	return s, true
}
