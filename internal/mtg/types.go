package mtg

import "strings"

// How a decklist is grouped, and which single type a card with several of
// them counts as. Both the deck list and the statistics panel sort by this,
// so it lives here rather than in either of them.

// Sections is the order a decklist is grouped in: the command zone first,
// then spells roughly in the order they get cast, then lands.
var Sections = []string{
	"Commander", "Creature", "Planeswalker", "Battle",
	"Instant", "Sorcery", "Artifact", "Enchantment", "Land", "Other",
}

// TypePrecedence decides the one section a card with several types lands in.
// Creature wins over everything, so an Artifact Creature is a creature; Land
// comes before Artifact and Enchantment, so an artifact land is a land.
var TypePrecedence = []string{
	"Creature", "Planeswalker", "Battle", "Land",
	"Instant", "Sorcery", "Artifact", "Enchantment",
}

// PrimaryType is the one type a card is filed under.
func PrimaryType(typeLine string) string {
	// Modal double-faced cards join their halves with "//"; the front face
	// is the one that decides where the card is listed.
	if i := strings.Index(typeLine, "//"); i >= 0 {
		typeLine = typeLine[:i]
	}
	for _, t := range TypePrecedence {
		if strings.Contains(typeLine, t) {
			return t
		}
	}
	return "Other"
}

// IsLand is asked often enough, by the curve and the colour spread both, to
// be worth a name.
func IsLand(c Card) bool { return PrimaryType(c.TypeLine) == "Land" }
