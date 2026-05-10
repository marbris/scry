# scryfall-tui

A terminal UI for searching [Magic: The Gathering](https://magic.wizards.com/) cards via the [Scryfall API](https://scryfall.com/docs/api), built for Commander/EDH players who live in the terminal.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)
![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)

## Features

- **Full Scryfall syntax** — search with the same query language you use on scryfall.com
- **Fuzzy filtering** — narrow down results by name or oracle text
- **Live preview** — card details shown alongside the list as you browse
- **Responsive layout** — horizontal split on wide terminals, vertical on narrow (tiling WM friendly)
- **Color-coded mana symbols** — WUBRG rendered in appropriate colors
- **Card details** — oracle text, P/T, legalities, rulings, EDHREC rank
- **Configurable sort & result count** — cycle sort orders, adjust max results (up to 175)
- **Quick lookup mode** — pass a query that returns one card and get plain text output, no TUI
- **Built-in syntax reference** — press `?` for a comprehensive Scryfall syntax guide focused on Commander
- **Gruvbox dark theme**

## Install

### From source

Requires [Go 1.21+](https://go.dev/dl/).

```bash
git clone https://github.com/marbris/scryfall-tui.git
cd scryfall-tui
go build -o scryfall-tui .
```

Then put it on your PATH however you prefer:

```bash
# Symlink (recommended — rebuilds are picked up automatically)
ln -s "$(pwd)/scryfall-tui" ~/.local/bin/scryfall-tui

# Or move it
mv scryfall-tui ~/.local/bin/

# Or use go install
go install .
# (binary goes to ~/go/bin/)
```

### Arch Linux

```bash
sudo pacman -S go
git clone https://github.com/marbris/scryfall-tui.git
cd scryfall-tui
go build -o scryfall-tui .
ln -s "$(pwd)/scryfall-tui" ~/.local/bin/scryfall-tui
```

Make sure `~/.local/bin` is in your `$PATH`.

## Usage

### Interactive mode

```bash
scryfall-tui
```

Opens the search screen. Type a Scryfall query, hit enter, browse results.

### Direct query

```bash
scryfall-tui "t:dragon c:R cmc<=5"
```

Opens directly to results for the given query.

### Quick lookup

If your query returns exactly one card, the details are printed to stdout without opening the TUI:

```bash
$ scryfall-tui '!"Lightning Bolt"'
Lightning Bolt  R
Instant
Murders at Karlov Manor · uncommon · CMC 1
EDHREC Rank: #40

Oracle Text:
Lightning Bolt deals 3 damage to any target.

Legalities:
  ✔ modern
  ✔ legacy
  ✔ vintage
  ✔ commander
  ✔ pauper
```

### Example queries

```bash
# Gruul commanders
scryfall-tui "id<=RG is:commander"

# Cheap removal in Commander
scryfall-tui "otag:removal f:commander cmc<=3"

# Board wipes under 5 mana
scryfall-tui "otag:boardwipe cmc<=5 f:commander"

# Partner commanders in Orzhov
scryfall-tui "is:partner id<=WB"

# Red dragons sorted by EDHREC rank
scryfall-tui "t:dragon c:R"
```

## Key Bindings

### Search Screen

| Key | Action |
|---|---|
| `enter` | Run search |
| `tab` / `shift+tab` | Cycle sort order |
| `ctrl+up` / `ctrl+down` | Adjust max results (25–175) |
| `?` | Scryfall syntax reference |
| `esc` | Quit |

### Results

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Navigate list |
| `/` | Fuzzy filter (searches name + oracle text) |
| `enter` | Open full card detail |
| `esc` | Back to search |

### Detail View

| Key | Action |
|---|---|
| `↑/↓` or `j/k` | Scroll |
| `esc` or `q` | Back to results |

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

## Configuration

There's nothing to configure — it works out of the box. Sorting defaults to EDHREC rank. The layout automatically switches between horizontal (wide terminal) and vertical (narrow terminal) at 120 columns.

## How It Works

The app uses the [Elm architecture](https://guide.elm-lang.org/architecture/) via Bubbletea:

```
User input → Update(msg) → new state → View() → render
                ↑                                    │
                └────────────────────────────────────┘
```

All card data comes from the [Scryfall REST API](https://scryfall.com/docs/api). No API key required. The app respects Scryfall's required headers and rate expectations.

## Dependencies

- [bubbletea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [bubbles](https://github.com/charmbracelet/bubbles) — text input, list components
- [lipgloss](https://github.com/charmbracelet/lipgloss) — terminal styling and layout
- [Scryfall API](https://scryfall.com/docs/api) — card data

## Acknowledgments

- [Scryfall](https://scryfall.com/) for their incredible free API
- [Charm](https://charm.sh/) for the Go TUI ecosystem
- [EDHREC](https://edhrec.com/) for Commander rankings
- The [Tagger](https://tagger.scryfall.com/) community for oracle tags

## License

MIT
