# scry

A terminal UI for looking up [Magic: The Gathering](https://magic.wizards.com/) cards, rules and wording history, built for Commander/EDH players who live in the terminal.

Card search comes from the [Scryfall API](https://scryfall.com/docs/api), the rules and glossary from the official [comprehensive rules](https://magic.wizards.com/en/rules), and the printed text of older printings from [MTGJSON](https://mtgjson.com/). *Scry* is a keyword action in its own right — rule 701.22, "look at the top card of your library" — which is roughly what this does.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)
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
- **Configurable sort** — cycle sort orders; every search returns up to 175 cards
- **Quick lookup mode** — pass a query that returns one card and get plain text output, no TUI
- **Built-in syntax reference** — press `?` for a comprehensive Scryfall syntax guide focused on Commander
- **Gruvbox dark theme**

## Install

### From source

Requires [Go 1.21+](https://go.dev/dl/).

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
$ scry maarika
Maarika, Brutal Gladiator  2BRG
Legendary Creature — Human Warrior
P/T: 7/4
Universes Within · rare · CMC 5
EDHREC Rank: #10333

Oracle Text
Maarika, Brutal Gladiator must be blocked if able.
As long as it's your turn, Maarika has indestructible.
Whenever Maarika deals damage to a creature, if that creature was dealt excess
damage this turn, that creature's controller sacrifices a noncreature, nonland
permanent.

Rulings
  none

Legalities
  ✘ standard
  ✔ legacy
  ✔ vintage
  ✔ commander
```

The text wraps to your terminal width, and the colour is dropped automatically when the output is piped or redirected.

### Rules browser

```bash
scry rules              # browse the comprehensive rules and glossary
scry rules flying       # open pre-filtered
scry rules update       # re-download the latest rules text
```

The rules text is cached in `~/.local/share/scry/comprules.txt` and downloaded on first use. (Upgrading from the old `scryfall-tui` name? The rules file is copied over from `~/.local/share/mtg-rules/` and the printed-text cache is moved from `~/.local/share/scryfall-tui/`, so nothing is re-downloaded.)

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
| `enter` | Run search, then move to the results |
| `tab` / `shift+tab` | Cycle sort order |
| `↑/↓` | Move through the results while typing |
| `ctrl+r` | Browse the comprehensive rules |
| `esc` | Back to the results (quits if there are none) |

### Results

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Navigate list |
| `/` | Fuzzy filter (searches name + oracle text) |
| `i` or `esc` | Edit the search query |
| `J/K` or `shift+↑/↓` | Scroll the panel |
| `r` | Rules for this card (press again for the card view) |
| `s` | Statistics for these results (press again for the card view) |
| `t` | Printed text history (press again for the card view) |
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

## Acknowledgments

- [Scryfall](https://scryfall.com/) for their incredible free API
- [Charm](https://charm.sh/) for the Go TUI ecosystem
- [EDHREC](https://edhrec.com/) for Commander rankings
- [MTGJSON](https://mtgjson.com/) for the printed text of every printing
- The [Tagger](https://tagger.scryfall.com/) community for oracle tags

## License

MIT
