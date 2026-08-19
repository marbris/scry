# scry — implementation plan for the panel layout

Companion to `design-layout.md` (what the program should be) and `keymap.md`
(how it is driven). This one says how to get there from what exists today.

## Decisions taken

Resolving the ambiguities in the design doc:

| Question | Decision |
|---|---|
| Statistics panel | A **mode of the information panel**, not a panel in the row. `s`/`<space>s` switch the right panel to histograms; `K`/`J` walk the rows and filter the source list live. |
| Panel navigation | Bare `h`/`l`, Miller-column style. `<space>` is the prefix for everything that creates, closes or moves panels. See `keymap.md`. |
| "open in new panel" key | **`L`** — shift+enter is not transmittable by terminals. Generally, **a capital letter puts the result in a new panel**; `L` is the exception only because Enter has no shifted form. |
| Saving | **Explicit only.** `w` writes, `W` writes into a new panel. No autosave, no debounce; quitting with unsaved edits prompts. |
| Editing deck | **Derived, not cycled.** One local deck panel open → it is the target. Several → most recently focused. `e` pins an override. `gd` jumps to it. |
| Card transfers | `a`/`x` against the editing deck; `y`/`p` for arbitrary list-to-list moves, carrying quantity and tags. |
| Migration | **New UI shell, domain kept.** Write the workspace/panel layer from scratch; reuse the domain code, repackaged. |
| Card row columns | **Name + the sort column, two columns only.** Mana cost is what column 2 shows when sorted by mana value (the default). |
| Membership marks | **Editable deck vs every list.** Every card list marks cards in the editing deck; the editing deck marks cards present in any other visible list. |
| Deck legality | **Full check for local decks** — per-card legality from Scryfall plus deck size, singleton and commander colour identity. Remotes use Moxfield's own flag. |
| Startup | **Splash, then restore the workspace** — panel types, their queries, and which deck was being edited. |
| Versions | `gv` — git history on a deck row, printed-text history on a card row. Replaces the old `t` binding. |
| Theme | Semantic **roles**, not colour names, loaded from JSON. Gruvbox becomes one theme among several. |
| Code layout | Domain split into `internal/` packages; the UI stays one package. |

### Corrections to the design doc

- `### scryfall search panel` says `ctrl+s` opens it; the general keymap says
  `ctrl+f`. Both are superseded — panel creation moved to `<space>`.
- `enter+shift` becomes `L`.
- `l` no longer means "lists". `h j k l` are directional only, and the panel
  is renamed the **decks** panel (`<space>d`).
- `e`/`E` no longer cycles. `e` pins.
- `t` no longer shows printed-text history; that is `gv`. `t` is tag.
- `enter`/`L` are unbound in card lists — individual cards don't open.

### Assumptions taken without asking (easy to reverse)

- **`a`/`x`/`t`/`T`/`c`/`y`** with nothing selected act on the highlighted row.
- **`esc` cascade** in a list: clear selection → clear filter → pop sub-view →
  close panel. In a search bar, `esc` returns to the list.
- **Panel widths**: the info panel takes a fixed share (≈32 cols, or a third
  on narrow terminals); the rest splits equally among list panels down to a
  floor of ~14 cols, after which the row scrolls horizontally keeping the
  focused panel on screen.
- **Rules from a card** shows one list, grouped under `abilities` / `actions`
  / `zones` / `glossary` headers.
- **`o`/`O` sorts**: decks panel by name / last-modified / size / type;
  rules panel by rule number / relevance.

## Where the current code stands

The app today is a **screen state machine**: one `model` with a `state` enum,
a `backStack`, exactly two fixed card panes (`m.results`, `m.deckPane`), and a
right panel with modes. Focus is a three-value enum.

The design wants a **workspace of N sibling panels**, each with its own type,
search bar, list, filter, sort and sub-view stack. These are structurally
incompatible; the shell is rewritten, not refactored.

**Survives** (~4,900 lines): card model, Scryfall client, deck files, deck
git, card cache, Moxfield, rules parser, statistics rows, printed-text
history, sorting, filtering, oracle highlighting.

**Rewritten**: `model.go`, `results.go` (941 lines), `pane.go`, `keymap.go`,
`help.go`, and the view halves of `deckpicker.go`, `moxuserview.go`,
`deckhistory.go`, `rulesview.go`.

**Deleted**: `tui_test.go` (1,623 lines), `layout_test.go`, `statsnav_test.go`,
`keymap_test.go` — all assert the two-pane shell. Domain tests survive.

## Target layout

```
scry/
  main.go                  package main — arg parsing, entry points
  docs/
  internal/
    mtg/        card, faces, mana, colours, type lines
    scryfall/   search, rulings, collection, HTTP
    deck/       file format, git, card cache, legality
    moxfield/   deck import, user decks
    rules/      parser, keyword/glossary/type indexes, card matching
    prints/     MTGJSON printed-text history
    stats/      statistic rows and groups
    theme/      roles, built-in themes, config loading
    ui/         workspace, panel, views, delegates, layout, keymap
```

Dependency direction is one-way: `ui` → everything; `deck` → `scryfall` →
`mtg`; `rules`/`stats`/`prints` → `mtg`; `moxfield` → `deck`. No cycles.

**`ui` deliberately stays one package.** Panels hold views, views open panels,
the workspace holds panels — mutually referential by nature. Splitting it
would mean inventing interfaces to satisfy the compiler rather than to clarify
anything. Organise it with filenames instead.

Splitting means exporting whatever crosses a boundary: `deckFile` becomes
`deck.File`, `ScryfallCard` becomes `mtg.Card`, `deckEntry` becomes
`deck.Entry`. Mechanical, and it removes the stutter along the way.

## Theming

The obstacle is that the current names are **colour** names — `gruvOrange`,
`gruvBgLight` — referenced directly at 234 sites across 16 files. A palette
you can swap needs **role** names, so a theme file says what a thing is for
rather than what colour it happens to be.

Roles to define:

| Group | Roles |
|---|---|
| Surfaces | `Bg` `BgAlt` `Border` `BorderFocus` `BorderEditing` |
| Text | `Text` `TextBright` `TextDim` `TextMuted` |
| Semantic | `Accent` `Highlight` `Success` `Error` `Info` `Special` |
| Selection | `SelectionBg` `SelectionFg` `Marked` `Member` |
| Mana | `ManaW` `ManaU` `ManaB` `ManaR` `ManaG` `ManaC` `ManaMulti` |
| Rarity | `RarityCommon` `RarityUncommon` `RarityRare` `RarityMythic` |
| Charts | `BarFill` `BarEmpty` |
| Diffs | `DiffAdd` `DiffRemove` |

Configuration:

- `~/.config/scry/config.json` — `{"theme": "gruvbox"}`, honouring
  `XDG_CONFIG_HOME`.
- Files split across all four XDG directories: **config** for what you wrote,
  **data** for decks, **state** for session and query history, **cache** for
  everything re-downloadable. `paths.Migrate` moves what's left in the old
  single directory, on startup, without ever overwriting.
- `~/.config/scry/themes/<name>.json` — user themes.
- Built-ins embedded with `//go:embed`; gruvbox stays the default.
- **Partial themes fall back role by role** to the default, so a user file
  can be five lines that only change the accent.
- Values accept `#rrggbb` or an ANSI index `0`–`15`. The second is free from
  lipgloss and gives you "inherit the terminal's own palette" for nothing.
- `scry theme list` and `scry theme <name>` for switching without editing
  JSON.

**Do this early.** Roughly 60% of those 234 references live in files that are
being deleted anyway, so converting to roles now costs a fraction of what it
would cost later — and every new view is written against roles from day one
rather than being retrofitted.

## Phases

All twelve are done. Each compiled and ran on its own.

**1. Repackage. — done**, in four commits. ~4,700 lines now live in ten
packages under `internal/`; 6,978 lines of old UI remain in `package main`,
to be replaced from phase 3 on.

| Package | Lines | |
|---|---|---|
| `deck` | 2,165 | file format, git, card cache, resolution |
| `rules` | 762 | parser, indexes, card matching |
| `moxfield` | 531 | deck import, user decks |
| `prints` | 376 | MTGJSON printed-text history |
| `stats` | 350 | statistic rows and groups |
| `scryfall` | 179 | search, rulings, collection |
| `mtg` | 165 | the card, and card-type facts |
| `fetch` | 99 | the network, one User-Agent |
| `paths` | 41 | where files live |
| `theme` | 27 | the palette |

Three rules held throughout:

- **Domain packages return answers, not `tea.Cmd`s.** The UI is the only
  part with a main thread to stay off, so the wrapping lives there.
- **`compat.go` bridges the old names**, so the doomed UI compiled untouched
  rather than being rewritten on its way to deletion. It is 167 lines and
  shrinks to nothing as its clients go.
- **Tests moved with their code.** `tui_test.go` was asserting internals two
  packages away; `moxuser_test.go` turned out to be two files in one.

**2. Theme. — done.** Define the roles, convert the surviving files to them, add the
config file, embedded built-ins, per-role fallback, and the `scry theme`
commands. A second theme (a light one) to prove the fallback works.

**3. Workspace shell. — done**, behind `scry --panels` while it grows, so
the working app stays working. `internal/ui` is one package as planned.

Two consequences of a space leader, found by running it and kept because
both are vim's own answer: the leader is dead inside a search bar (that's
insert mode), and `esc` cascades — clear the query, leave the bar, close the
panel, and closing the last one lands on the splash rather than quitting.

The layout is a pure function with tests: panels cover the terminal exactly,
differ by at most a column, never shrink below a readable floor (the row
scrolls instead), and the focused panel is always on screen. `workspace`, `panel`, the layout arithmetic (equal
shares, minimum width, horizontal overflow), the focus ring on `h`/`l`, the
`<space>` prefix and its which-key popup, splash screen, empty information
panel, panel create/close/move/jump. No content in the panels yet.

**4. Card list view. — done.** The row delegate: name plus the sort column, and the
degradation ladder — type line to an initialism (`Legendary Creature - Elf
Faerie Noble` → `LC-EFN`), then the name (`Dwynen, Gilt-Leaf Daen` → `D,GLD.`).
Plus `o`/`O`, `/`, `v`/`V`, and the membership marks.

The list is its own rather than bubbles' — cheaper than fighting a keymap and
a filter mode that both want the same keys, and it holds `deck.Card`
directly, which is the merge with `cardItem` that phase 1 deferred to here.

**The ladder is per row, not per list.** A column ends up mixing full names
with shortened ones, and the trailing full stop is what says which is which —
`Sol Ring` is a name, `EA.` is an abbreviation. Shortening every row to match
the longest would cost the names that fit perfectly well and gain only
tidiness.

**5. Find panel. — done.** Search bar, `tab` target cycling, `up`/`down`
query history, `ctrl+o` query sort, results feeding a card list view.
`scry --panels <query>` opens straight onto a search.

Results are kept **in Scryfall's order**, not re-sorted on arrival: the query
asked for an order — EDHREC rank by default — and re-sorting would throw away
the answer to the question just asked. The panel calls it "scryfall order"
rather than "as found".

Panels carry an id, because a search in flight has to find its way back to
the panel that asked for it and the row can be reordered or closed
meanwhile. An answer to a closed panel, or to a query that panel has since
moved on from, is dropped.

**6. Decks panel. — done.** The unified list — local decks, remote links, Moxfield
usernames — with the compact columns. `i` to add a remote or a user,
`n`/`r`/`x`/`c`, `enter`/`L`, and the sub-views (`gv` versions, a user's
decks) on the panel's stack.

Panels hold a **stack of views**, not one — some are reached from inside
another, and esc pops back rather than closing. `cardList` and `deckList`
both implement the interface; the movement keys live once, on a shared
cursor.

Three corrections: a decks panel opens **on your decks**, not an empty search
bar (`i` reaches the bar, which follows rather than searches); `esc` no
longer empties a panel back to its bar — clear, pop, close, as the design
doc always said; and `g` is a prefix only, so `gg` and `gv` coexist.

**Every width is a display width.** A rune count overflows on wide
characters — an emoji is one rune and two columns — and a real deck name
proved it.

**7. Editing deck and transfers. — done.** The derived-target rule and `e` pinning,
`gd`, `a`/`x`, the `y`/`p` register carrying quantity and tags, `t`/`T`, `c`,
undo, `w`/`W`, and the unsaved-changes prompt on quit.

`a` counts up rather than refusing a card already there, which makes `a`/`x`
symmetric and removes the need for `+`/`-`. Undo keeps whole copies of the
deck rather than a diff — a diff that gets it wrong corrupts the deck rather
than merely failing — and an edit that changed nothing drops its own step.

`w` and `c` are different operations at different levels, and both are
needed:

| | acts on | produces |
|---|---|---|
| `c` in a decks panel | a **row** — one deck among many | another row: a local copy beside it, or a sync if it exists |
| `w` in a card list | the **list** in front of you | this panel becomes the local deck; `W` opens it in a new panel |

**8. Rules panel. — done.** Paragraph search results, the full rule in the
information panel, and card-scoped opening — grouped into abilities, actions,
ability words, zones, card types and glossary.

Views now say what the information panel should show; phase 9 fills in the
rest. Relevance ranks by how early and how tightly the terms appear —
"sacrifice a creature" used to lead with rule 101.4, because "a" is in every
rule. The rules bar quotes the same way the card filter does, so there is one
query syntax in the program.

**9. Information panel. — done.** Card, deck, rule and diff renderers;
`K`/`J` to move and `ctrl+k`/`ctrl+j` to scroll; `s`/`<space>s` statistics
with live cross-list filtering; `gv` for both kinds of version history.

Oracle highlighting is ported from the old screen onto theme roles. Rulings
arrive on a delay after the cursor settles. Statistics narrow the list they
count, so bars and rows can never disagree; every bar shares one scale.

Card printed-text history reads the sets already cached and **offers** the
rest — forty printings is forty multi-megabyte downloads. `y` is the
go-ahead, claimed before the view sees it because a card list takes `y` for
yank.

`g` is now genuinely a prefix: every list had been claiming it for "go to the
top" before the workspace saw it, so `gd` and `gv` could never be typed.

**10. Legality engine. — done.** Per-card legality plus deck size, singleton
and commander colour identity, with partners, basics and "any number" cards
handled. Feeds the decks panel's flag column and the panel you're editing in.

Runs from the **card cache alone**, so opening the decks panel checks a
directory of decks without touching the network. A deck whose cards aren't
all cached is reported unknown, never legal.

**11. Persistence and entry points. — done.** Workspace in `session.json`,
splash, and every CLI path rehomed onto the new shell — which is now simply
the shell. The two-pane screen is deleted: 25 files and 18 test files, with
`compat.go` among them, empty at last because there is no old UI to bridge.
Package `main` is four files.

The session keeps what will still be there tomorrow: a search as its query,
a deck of yours by slug, nothing borrowed and nothing empty. **The suite went
from 125 seconds to under one** — that was the old tests reaching the
network.

**12. Tests. — done.** Most landed alongside each phase, so this was a gap
pass guided by coverage. Four packages had none at all.

| | before | after |
|---|---|---|
| `mtg` | 0% | 100% |
| `scryfall` | 0% | 89% |
| `fetch` | 0% | 85% |
| `deck` | 70% | 84% |
| `ui` | 69% | 77% |
| `moxfield` | 18% | 43% |
| `prints` | 20% | 44% |

`scryfall.Search` needed an injectable URL to be testable at all — paging and
the partial-answer rule are the subtlest code here and had only been checked
by reading. What stays uncovered is HTTP plumbing over already-tested pieces,
and the platform branches in `paths` that can't run on Linux.

The pass found two bugs: `/` did nothing in a rules panel (a type switch with
no case, failing silently), and a tag with a space could be imported but
never typed.
