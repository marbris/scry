package ui

// The information panel, pinned to the right.
//
// It is never focused. K and J move within it and ctrl+k / ctrl+j scroll it,
// from wherever you happen to be — which is what keeps it a panel you read
// rather than a place you have to go and come back from. The cost is four
// keys in the shift and control spaces; the saving is a whole mode.
//
// In statistics the same four keys walk the breakdown: a category at a time,
// then a group at a time. Same keys, same axis, one level coarser — which is
// what ctrl means everywhere else here too.

type infoMode int

const (
	// infoCard follows the cursor: whatever is highlighted, described.
	infoCard infoMode = iota
	// infoStats is the histograms, which double as a filter for the list
	// they were counted from.
	infoStats
	// infoVersions is gv: a deck's git history, or a card's printed text
	// through the years.
	infoVersions
)

func (i infoMode) String() string {
	switch i {
	case infoStats:
		return "statistics"
	case infoVersions:
		return "versions"
	}
	return "card"
}

type infoPanel struct {
	mode infoMode
	// cursor is the row within the panel — a statistics category, a
	// revision — and offset is how far the view is scrolled.
	cursor int
	offset int
}

// toggle switches to a mode, or back to the card if already there. One key
// in and out beats two keys that both mean "show me statistics".
func (p *infoPanel) toggle(mode infoMode) {
	if p.mode == mode {
		p.mode = infoCard
	} else {
		p.mode = mode
	}
	p.cursor, p.offset = 0, 0
}

func (p *infoPanel) move(delta int) {
	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (p *infoPanel) scroll(delta int) {
	p.offset += delta
	if p.offset < 0 {
		p.offset = 0
	}
}
