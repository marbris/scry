# scry

A terminal UI for looking up [Magic: The Gathering](https://magic.wizards.com/) cards, rules and wording history, built for Commander/EDH players who live in the terminal.

Card search comes from the [Scryfall API](https://scryfall.com/docs/api), the rules and glossary from the official [comprehensive rules](https://magic.wizards.com/en/rules), and the printed text of older printings from [MTGJSON](https://mtgjson.com/). *Scry* is a keyword action in its own right — rule 701.22, "look at the top card of your library" — which is roughly what this does.

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)
![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)

## Features

- **Full Scryfall syntax** — search with the same query language you use on scryfall.com
- **Fuzzy filtering** — narrow down results by name or oracle text
- **One screen** — search bar, result list and card panel together; no separate search page to pass through
- **Live preview** — card details shown alongside the list as you browse
- **Responsive layout** — horizontal split on wide terminals, vertical on narrow (tiling WM friendly)
- **Color-coded mana symbols** — WUBRG rendered in appropriate colors
- **Card details** — oracle text, P/T, legalities, rulings, EDHREC rank, all in one panel
- **Rulings as you browse** — Scryfall rulings load for whichever card is under the cursor, debounced so scrolling past a card costs nothing
- **Oracle text highlighting** — mana and tap symbols, keyword abilities, keyword actions, ability words, reminder text, loyalty costs and P/T modifiers, all coloured
- **Printed text history** — press `t` to see how a card's wording changed across its printings, from Alpha to today (via MTGJSON, since Scryfall only serves current oracle text)
- **Comprehensive rules built in** — press `r` to swap the card panel for the rules its text invokes, and browse the full rulebook and glossary (merged in from `mtg-rules`)
- **Statistics you can filter by** — `s` breaks the results down by colour, rarity, mana value and type; `J/K` walks those categories, narrows the list to whichever one you're on, and re-cuts every histogram for what's left
- **Configurable sort** — cycle sort orders; every search returns up to 175 cards
- **Moxfield decks** — paste a public deck URL (or `scry deck <id>`) to browse someone's list, with every card behaving like a search result; save decks by name and reopen them with `scry deck <name>`
- **Deck tags in statistics** — the author's own card tags come down with the deck and get their own breakdown
- **Both faces of double-faced cards** — transforming and modal cards show each side's cost, type, P/T and rules text
- **Quick lookup mode** — pass a query that returns one card and get plain text output, no TUI
- **Built-in syntax reference** — press `?` for a comprehensive Scryfall syntax guide focused on Commander
- **Gruvbox dark theme**

## Screenshots

Search results with the card panel alongside — oracle text highlighted, rulings loaded for whichever card is under the cursor.

![Results and card panel](screenshots/results.png)

`r` swaps the panel for the comprehensive rules the card's text invokes — keyword abilities, keyword actions, ability words and the glossary entries behind them.

![Rules for the selected card](screenshots/rules.png)

A public Moxfield deck opened by URL — the deck reads in decklist order, and its cards behave like any other result.

![A Moxfield deck](screenshots/moxfield_deck.png)

`s` breaks the list down by the author's tags, type, colour, mana value and rarity.

![Statistics for a deck](screenshots/statistics.png)

`J/K` walks those categories and narrows the cards to whichever one it's on — here the deck's removal, with the breakdown re-cut for just those sixteen cards. Categories the filter has emptied hold their places at zero, and the bars stay scaled to the whole deck.

![Filtering by a statistics category](screenshots/filter_stats.png)

`t` shows how the card's printed wording changed across its printings, one entry per distinct wording.

![Printed text history](screenshots/card_text_history.png)

`enter` opens the full rules browser on the rules that card matched, with the rule text and its cross-references on the right.

![Rules browser](screenshots/browse_rules.png)

A query that returns exactly one card prints straight to stdout — same highlighting, no TUI.

![Quick lookup](screenshots/quick_lookup.png)

## Install

### From source

Requires [Go 1.26+](https://go.dev/dl/) — that's what `go.mod` asks for.

```bash
git clone https://github.com/marbris/scry.git
cd scry
go build -o scry .
```

Then put it on your PATH however you prefer:

```bash
# Symlink (recommended — rebuilds are picked up automatically)
ln -s "$(pwd)/scry" ~/.local/bin/scry

# Or move it
mv scry ~/.local/bin/

# Or use go install
go install .
# (binary goes to ~/go/bin/)
```

### Arch Linux

```bash
sudo pacman -S go
git clone https://github.com/marbris/scry.git
cd scry
go build -o scry .
ln -s "$(pwd)/scry" ~/.local/bin/scry
```

Make sure `~/.local/bin` is in your `$PATH`.

### macOS

Download the latest binary from [Releases](https://github.com/marbris/scry/releases):

- Apple Silicon (M1/M2/M3): `scry-darwin-arm64`
- Intel Mac: `scry-darwin-amd64`

```bash
chmod +x scry-darwin-arm64
xattr -d com.apple.quarantine scry-darwin-arm64
mv scry-darwin-arm64 /usr/local/bin/scry
```

Or build from source:

```bash
brew install go
git clone https://github.com/marbris/scry.git
cd scry
go build -o scry .
mv scry /usr/local/bin/
```

### Windows

Download `scry-windows-amd64.exe` from [Releases](https://github.com/marbris/scry/releases) and add it to your PATH.

### Linux (other distros)

Download the latest binary from [Releases](https://github.com/marbris/scry/releases):

- x86_64: `scry-linux-amd64`
- ARM64: `scry-linux-arm64`

```bash
chmod +x scry-linux-amd64
mv scry-linux-amd64 ~/.local/bin/scry
```

## Usage

### Interactive mode

```bash
scry
```

Opens on the results screen with the search bar focused. Type a Scryfall query, hit enter, browse results. `i` or `esc` puts you back in the search bar.

### Direct query

```bash
scry "t:dragon c:R cmc<=5"
```

Runs the query straight away — the search bar shows it at the top while the results load.

### Quick lookup

If your query returns exactly one card, the details are printed to stdout without opening the TUI — with the same syntax highlighting the TUI uses, including the card's rulings:

```
$ scry ghen
Ghen, Arcanum Weaver  RWB
Legendary Creature — Human Wizard
P/T: 2/3
Commander Legends · rare · CMC 3
EDHREC Rank: #9208

Oracle Text
{R}{W}{B}, {T}, Sacrifice an enchantment: Return target enchantment card from
your graveyard to the battlefield.

Rulings (2)
  • Because targets are chosen before costs are paid (such as the cost of
    sacrificing an enchantment), Ghen's ability can't target the enchantment
    you intend to sacrifice to activate its ability.
  • An Aura put onto the battlefield this way doesn't target anything (so it
    could be attached to an opponent's permanent with hexproof, for example),
    but the Aura's enchant ability restricts what it can be attached to. If
    the Aura can't legally be attached to anything, it remains in your
    graveyard.

Legalities
  ✘ standard
  ✘ pioneer
  ✘ modern
  ✔ legacy
  ✔ vintage
  ✔ commander
  ✘ pauper
```

The text wraps to your terminal width, and the colour is dropped automatically when the output is piped or redirected.

### Moxfield decks

Any public [Moxfield](https://moxfield.com/) deck can be opened as a result list:

```bash
scry deck j-0aJlxuOUm9FnKRvJcfZw            # the id out of the deck's URL
scry https://moxfield.com/decks/j-0aJlxuOUm9FnKRvJcfZw
```

or paste the URL into the search bar instead of a query.

The deck reads in decklist order — commanders, then creatures, spells and lands, alphabetically within each — with repeat copies shown as `30x Mountain`. Everything the results view does works on a deck's cards: `r` for the rules a card invokes, `s` for the deck's curve and colour spread, `t` for printed text history, `/` to filter, `enter` to browse matched rules.

```
⌕ https://moxfield.com/decks/j-0aJlxuOUm9FnKRvJcfZw
Deck     Winota: Snowball Stax  by ComedIan  · commander
Cards    100 cards · 98 unique
──────────────────────────────────────────────────────────────
▸ Winota, Joiner of Forces        2RW     Legendary Creature — …
  Ainok Strike Leader             1W      Creature — Dog Warrior
  Alexios, Deimos of Kosmos       3R      Legendary Creature — …
```

Only the command zone and mainboard are loaded; sideboards and maybeboards are skipped. Moxfield stores a Scryfall id per card, so the deck request only supplies ids and quantities — the card data itself comes from Scryfall, which is why rulings, highlighting and rules matching all work unchanged. A 100-card deck is one Moxfield request plus two Scryfall lookups.

Private and unlisted decks aren't accessible; the deck has to be public.

#### Saving decks

Press `w` on a deck to save it under a name made from its title, or name it yourself from the shell:

```bash
scry deck save ghen https://moxfield.com/decks/pdxwlkCOVkSQB2-6FYBvog
scry deck ghen          # open it again
scry deck list          # what's saved
scry deck rm ghen       # forget it
```

Saved decks live in `~/.local/share/scry/decks.json` and hold only the name and address — the deck itself is re-fetched each time, so an edited deck comes back current rather than stale.

#### Card tags

If the deck's author tagged their cards on Moxfield — `Ramp`, `Removal`, `Protection` — those tags come down with the deck and get their own breakdown in the statistics panel, counted by copies and commonest first:

```
Tags
  Land          ██████████████████████████████████████████ 37
  Aura          ████████████████ 12
  ETB/LTB       ██████████ 8
  Ramp          ██████████ 8
  Own           █████████ 7
```

Tags are per deck and set by whoever built it, so an untagged deck simply doesn't show the section.

### Statistics as a filter

`s` swaps the card panel for a breakdown of everything in the list — colour, rarity, mana value, type, and the author's tags if it's a tagged deck. The breakdown is a list in its own right: `J/K` walks it, and the cards narrow to whichever category the cursor is on.

```
Cards    100 cards · 86 unique  ▸ Tags: Aura
──────────────────────────────────────────────────────────────
▸ Chime of Night        1B    Enchantment — Aura  │  Tags
  Darksteel Mutation    1W    Enchantment — Aura  │    Land   ████████████████ 37
  Despondency           1B    Enchantment — Aura  │  ▸ Aura   ██████ 12
```

The histograms redraw for whichever cards the selected category leaves on screen: land on **White** and every bar describes the white cards — their curve, their rarities, their types. Walking further re-cuts the same view again.

Categories the filter has emptied keep their places and read zero rather than disappearing, so nothing shifts under the cursor and you can always walk back out. Bars stay scaled to the unfiltered set, so a small category reads as small rather than refilling the panel. `ctrl+d` / `ctrl+u` scroll a long breakdown without moving the selected category; `esc` clears the filter, and the header shows which one is active until you do.

```
Statistics (30 cards) · Color: White      Statistics (12 cards) · Tags: Aura

Color                                     Type
▸ White       ██████████████████ 30         Creature      0
  Black       ██ 2                          Instant       0
  Red         █ 1                           Sorcery       0
  Colorless    0                            Artifact      0
  Multi       ██ 2                          Enchantment ██████████ 12
                                            Land          0
```

### Rules browser

```bash
scry rules              # browse the comprehensive rules and glossary
scry rules flying       # open pre-filtered
scry rules update       # re-download the latest rules text
```

The rules text is cached in `~/.local/share/scry/comprules.txt` and downloaded on first use.

### Example queries

```bash
# Gruul commanders
scry "id<=RG is:commander"

# Cheap removal in Commander
scry "otag:removal f:commander cmc<=3"

# Board wipes under 5 mana
scry "otag:boardwipe cmc<=5 f:commander"

# Partner commanders in Orzhov
scry "is:partner id<=WB"

# Red dragons sorted by EDHREC rank
scry "t:dragon c:R"
```

## Key Bindings

### Search Bar

| Key | Action |
|---|---|
| `enter` | Run search — or load the deck, if the text is a Moxfield URL |
| `tab` / `shift+tab` | Cycle sort order |
| `↑/↓` | Move through the results while typing |
| `ctrl+r` | Browse the comprehensive rules |
| `esc` | Back to the results (quits if there are none) |

### Results

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Navigate list |
| `/` | Fuzzy filter (searches name + oracle text) |
| `i` | Edit the search query |
| `esc` | Clear the filter — or quit, if there isn't one |
| `J/K` or `shift+↑/↓` | Scroll the panel; in statistics, walk the categories |
| `ctrl+d` / `ctrl+u` | Scroll the panel half a screen, leaving the selection put |
| `r` | Rules for this card (press again for the card view) |
| `s` | Statistics for these results (press again for the card view) |
| `t` | Printed text history (press again for the card view) |
| `w` | Save the current deck (decks only) |
| `enter` | Browse the rules this card's text matched |
| `ctrl+r` | Browse all comprehensive rules |
| `?` | Scryfall syntax reference |

### Rules Browser

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Navigate rules |
| `/` | Search rules and glossary |
| `g` | Toggle rules / glossary |
| `J/K` | Scroll the rule text |
| `esc` or `q` | Back |

### Syntax Help

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Scroll |
| `d` / `u` | Page down / up |
| `esc`, `q`, or `?` | Back to search |

### Global

| Key | Action |
|---|---|
| `ctrl+c` | Quit |

## Printed text history

Scryfall serves only a card's *current* oracle text — its `printed_text` field is populated for non-English cards only, and the API carries no oracle revision history. The wording as actually printed comes from [MTGJSON](https://mtgjson.com/)'s `originalText`, so `t` pulls from there.

MTGJSON publishes whole sets rather than single cards, so a heavily reprinted card means one file per set it appeared in. Nothing is downloaded until you press `t`, and the panel tells you how many sets it needs before fetching anything:

```
Text History · Serra Angel

45 printings across 45 sets.

28 sets aren't cached yet. MTGJSON only publishes whole sets, so
reading this card's old wording means downloading them.

t: download and show
```

Press `t` again to go ahead. Each set is distilled to a name → text map in `~/.local/share/scry/originals/` (a few KB each) and never fetched twice — 60 sets comes to about 1.5 MB on disk. The result lists one entry per *distinct wording*, not per printing:

```
Text History · Serra Angel
22 wordings · 45 printings

Limited Edition Alpha · 1993
Flying
Does not tap when attacking.

Revised Edition · 1994
Flying
Attacking does not cause Serra Angel to tap.

Seventh Edition · 2001
Flying
Attacking doesn't cause Serra Angel to tap.
```

## Configuration

There's nothing to configure — it works out of the box. Sorting defaults to EDHREC rank, and every search returns up to 175 cards.

The layout adapts to the terminal width: at 120 columns and up the panel sits beside the list, below that it sits underneath. The panel shows the card by default; `r` and `s` swap it for the rules or the statistics, and pressing the same key again brings the card back.

## How It Works

The app uses the [Elm architecture](https://guide.elm-lang.org/architecture/) via Bubbletea:

```
User input → Update(msg) → new state → View() → render
                ↑                                    │
                └────────────────────────────────────┘
```

All card data comes from the [Scryfall REST API](https://scryfall.com/docs/api). No API key required. The app respects Scryfall's required headers and rate expectations — rulings are fetched only after the cursor has rested on a card for 100 ms, and each card's rulings are fetched at most once per session.

Keyword highlighting and the rules panel are driven by the official comprehensive rules rather than a hardcoded word list: keyword abilities come from rule 702's sub-headings, keyword actions from 701's, and ability words from the list inside rule 207.2c. A new set's keywords show up as soon as `scry rules update` pulls a newer rules file.

## Dependencies

- [bubbletea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [bubbles](https://github.com/charmbracelet/bubbles) — text input, list components
- [lipgloss](https://github.com/charmbracelet/lipgloss) — terminal styling and layout
- [Scryfall API](https://scryfall.com/docs/api) — card data
- [Magic Comprehensive Rules](https://magic.wizards.com/en/rules) — rules text and glossary
- [MTGJSON](https://mtgjson.com/) — per-printing printed text
- [Moxfield](https://moxfield.com/) — public deck lists

## Acknowledgments

- [Scryfall](https://scryfall.com/) for their incredible free API
- [Charm](https://charm.sh/) for the Go TUI ecosystem
- [EDHREC](https://edhrec.com/) for Commander rankings
- [MTGJSON](https://mtgjson.com/) for the printed text of every printing
- The [Tagger](https://tagger.scryfall.com/) community for oracle tags

## License

MIT
