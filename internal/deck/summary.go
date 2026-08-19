package deck

import (
	"os"
	"time"
)

// Summary is a deck as the picker lists it: enough to draw a row without
// resolving a single card name against Scryfall.
//
// Deliberately cheap. Opening the decks panel reads every deck file, and a
// panel that had to reach the network to tell you what you own would be
// unusable on a train.
type Summary struct {
	Slug     string
	Name     string
	Format   string
	Total    int // cards, counting quantities
	Unique   int // distinct cards
	Modified time.Time
	// Source is the Moxfield deck this was imported from, if any. A local
	// deck with a source can be synced against it again.
	Source string
	// Broken is set when the file wouldn't parse. The row still appears —
	// silently omitting a deck you know you have is worse than showing it
	// with a warning.
	Broken bool
}

// Summaries lists every deck on disk, in no particular order.
func Summaries() ([]Summary, error) {
	slugs, err := List()
	if err != nil {
		return nil, err
	}

	out := make([]Summary, 0, len(slugs))
	for _, slug := range slugs {
		s := Summary{Slug: slug, Name: slug}
		if info, err := os.Stat(Path(slug)); err == nil {
			s.Modified = info.ModTime()
		}

		d, err := Read(slug)
		if err != nil {
			s.Broken = true
			out = append(out, s)
			continue
		}
		if d.Name != "" {
			s.Name = d.Name
		}
		s.Format = d.Format
		s.Source = d.Source
		s.Total, s.Unique = d.Counts()
		out = append(out, s)
	}
	return out, nil
}
