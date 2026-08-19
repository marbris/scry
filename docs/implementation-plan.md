# scry — implementation plan for the panel layout

Companion to `scry_design_layout.md`. That doc says what the program should
be; this one says how to get there from what exists today.

## Decisions taken (2026-08-18)

Resolving the ambiguities in the design doc:

| Question | Decision |
|---|---|
| Statistics panel | A **mode of the information panel**, not a panel in the row. `s`/`ctrl+s` switch the right panel to histograms; `K`/`J` walk the rows and filter the source list live. |
| "open in new panel" key | **`L`** (shift+enter is not transmittable by terminals; ctrl+enter needs the Kitty protocol). `enter` opens in place, `L` opens to the right. |
| Saving | **Explicit only.** `w` writes the editable deck and makes the git commit. No autosave, no debounce. Quitting with unsaved edits prompts. |
| Migration | **New UI shell, domain kept.** Write the workspace/panel layer from scratch; reuse the ~4,900 lines of domain code unchanged. |
| Card row columns | **Name + the sort column, two columns only.** Mana cost is what column 2 shows when sorted by mana value (the default). |
| Membership marks | **Editable deck vs every list.** Every card list marks cards that are in the editing deck; the editing deck marks cards present in any other visible list. |
| Deck legality | **Full check for local decks** — per-card legality from Scryfall plus deck size, singleton and commander colour identity. Remotes use Moxfield's own flag. |
| Startup | **Splash, then restore the workspace** — panel types, their queries, and which deck was being edited. |

### Doc corrections these imply

- `### scryfall search panel` says `ctrl+s` opens it; the general keymap says
  `ctrl+f`. **`ctrl+f` is correct** — `ctrl+s` is global statistics.
- `enter+shift` becomes `L` throughout.

### Assumptions taken without asking (easy to reverse)

- **`e/E`** cycles among the local-deck panels *currently open*, not every
  deck on disk.
- **`a`/`x`/`t`/`T`/`c`** with nothing selected act on the highlighted row.
- **`esc` cascade** in a list: clear selection → clear filter → pop sub-view
  → close panel. In a search bar, `esc` returns to the list.
- **Panel widths**: the info panel takes a fixed share (≈32 cols, or a third
  on narrow terminals); the rest is split equally among list panels down to a
  floor of ~14 cols, after which the row scrolls horizontally keeping the
  focused panel on screen.
- **Rules from a card** shows one flat list, grouped under `abilities` /
  `actions` / `zones` / `glossary` headers.
- **`o/O` sorts**: decks panel by name / last-modified / size / type;
  rules panel by rule number / relevance.

### One keymap conflict still open

`tab` is asked to do two things: cycle the **search target** (scryfall /
decks / moxfield / rules) in a new panel's bar, and cycle the **scryfall
query sort** in a scryfall panel's bar. Both apply with the search bar
focused.

Proposal: `tab`/`shift+tab` **always** cycles search target — it is the
uniform behaviour and the doc's headline. The scryfall query sort (a
different thing from the on-screen `o/O` sort) moves to **`ctrl+o`** in the
search bar. Change if you'd rather it went elsewhere.

## Where the current code stands

The app today is a **screen state machine**: one `model` with a `state` enum
(`stateResults`, `stateRules`, `stateDecks`, `stateMoxUser`,
`stateDeckHistory`, `stateHelp`, `stateKeys`), a `backStack`, exactly two
fixed card panes (`m.results`, `m.deckPane`), and a right panel with modes.
Focus is a three-value enum.

The design wants a **workspace of N sibling panels**, each with its own type,
search bar, list, filter, sort and sub-view stack. These are structurally
incompatible; the shell is rewritten, not refactored.

**Kept as-is** (~4,900 lines): `card.go`, `cardcache.go`, `cardsort.go`,
`deckfile.go`, `deckgit.go`, `deck.go`, `moxfield.go`, `moxuser.go`,
`rules.go`, `search.go`, `filter.go`, `highlight.go`, `history.go`,
`stats.go` (row-building half), `searchhistory.go`, `util.go`, `theme.go`,
`legacy.go`.

**Rewritten**: `model.go`, `results.go` (941 lines), `pane.go`, `keymap.go`,
`help.go`, and the view halves of `deckpicker.go`, `moxuserview.go`,
`deckhistory.go`, `rulesview.go`.

**Deleted**: `tui_test.go` (1,623 lines), `layout_test.go`,
`statsnav_test.go`, `keymap_test.go` — all assert the two-pane shell. Domain
tests (`deckfile`, `deckgit`, `cardcache`, `cardsort`, `moxuser`, `filter`,
`decktag`, `deckedit`) survive.

Everything stays in `package main` in one directory. Splitting into
`internal/` would mean exporting most of the domain layer for no benefit at
this size.

## Architecture

```go
// workspace.go — the row of panels and what has focus
type workspace struct {
    panels  []*panel
    focused int  // -1 while the splash is up
    editing int  // index of the editing-deck panel, -1 if none
    info    infoPanel
    scrollX int  // first visible panel when the row overflows
}

// panel.go — one vertical panel: a search bar over a stack of views
type panel struct {
    search     textinput.Model
    searchOpen bool     // the bar is replaced by a header once results land
    stack      []view   // deck list → git log, or → moxfield user's decks
}

// view.go — what a panel is currently showing
type view interface {
    Update(tea.Msg, *appCtx) (view, tea.Cmd)
    Header(width int) string          // replaces the search bar
    List() *list.Model
    Info(width int) string            // what the info panel shows for the row
    Keys() []binding                  // feeds the help screen too
}
```

Views: `scryfallView`, `deckListView` (local + remote + moxfield users),
`cardListView` (with an `editable` flag), `moxUserDecksView`, `gitLogView`,
`rulesView`.

The info panel asks the focused panel's top view for `Info()`, unless it is
in statistics mode, in which case it renders histograms over that view's
cards (or every visible list, for `ctrl+s`).

Key routing, outermost first: global → panel → top view → the bubbles list.

## Phases

Each phase should compile and run. Ordered so the thing is usable as early
as possible.

**1. Workspace shell.** `workspace`, `panel`, the layout arithmetic (equal
shares, minimum width, horizontal overflow), focus ring on `h`/`l`, splash
screen, empty info panel, `ctrl+f`/`ctrl+l`/`ctrl+r` opening empty typed
panels, `esc`/`q`. No content in the panels yet.

**2. Card list view.** The row delegate rewrite: name + sort column, and the
degradation ladder — type line to an initialism
(`Legendary Creature - Elf Faerie Noble` → `LC-EFN`), then the name
(`Dwynen, Gilt-Leaf Daen` → `D,GLD.`). Plus `o`/`O`, `/`, `v`/`V`, and the
membership marks.

**3. Scryfall panel.** Search bar, `up`/`down` query history, query sort,
results feeding a `cardListView`.

**4. Decks panel.** The unified list — local decks, remote links, moxfield
usernames — with the compact columns from the doc. `i` to add a remote or a
user, `n`/`x`/`r`/`c`/`g`/`enter`/`L`, and the sub-views (`gitLogView`,
`moxUserDecksView`) on the panel's stack.

**5. Editing deck.** `e`/`E` cycling, `a`/`x`/`t`/`T`/`c`, `w` to save and
commit, undo, and the unsaved-changes prompt on quit.

**6. Rules panel.** Paragraph search results, full rule in the info panel,
and the card-scoped opening (abilities / actions / zones / glossary).

**7. Info panel and statistics.** Card, deck, rule and diff renderers;
`K`/`J` to move and `ctrl+j`/`ctrl+k` to scroll; `s`/`ctrl+s` statistics
with live cross-list filtering.

**8. Legality engine.** Per-card legality plus deck size, singleton and
commander colour identity. Feeds the decks panel's flag column.

**9. Persistence and entry points.** Workspace in `session.json`, splash,
and the CLI paths (`scry <query>`, `scry deck …`, `scry rules …`) rehomed
onto the new shell.

**10. Tests.** Rebuild the TUI tests against the workspace: layout
arithmetic and the abbreviation ladder, the focus ring, the `esc` cascade,
panel open/close, and stats filtering across lists.
