# scry — navigation scheme

## The organising rule

**Bare keys act on the contents of the focused panel. `<space>` acts on the
panels themselves.**

Consistency lives inside a key space, not across the whole program — the same
way vim has `f`, `ctrl+f`, `gf` and `zf` all meaning different things without
anyone calling it inconsistent. The prefix carries the domain; the letter
carries the verb within it. So `r` (rename a deck) and `<space>r` (open a
rules panel) coexist without ambiguity.

Four concept letters, used consistently in both spaces:

**`d`** decks · **`s`** statistics · **`r`** rules · **`f`** find

`l` is deliberately not one of them. `h j k l` are directional and nothing
else — the panel formerly called "lists" is the **decks** panel.

## Spaces

| Space | Scope | Examples |
|---|---|---|
| bare | the row / the list in the focused panel | `j` `/` `a` `t` `s` |
| `<space>` | the workspace: create, close, move, focus panels | `<space>f` `<space>c` |
| `g` | jump to | `gg` `gd` |
| shift | the information panel | `K` `J` |

`,` stays bound as an alias for `<space>`, since the leader popup is already
built around it.

**The leader does nothing inside a search bar**, where space is a space —
the bar is insert mode, and vim's leader doesn't work there either. A fresh
panel opens with its bar focused, so opening a second empty panel means
finishing or abandoning the first. An empty panel is a question you haven't
answered.

## Panels — `<space>`

| Key | Action |
|---|---|
| `<space>f` | new **find** panel (scryfall search) |
| `<space>d` | new **decks** panel |
| `<space>r` | new **rules** panel |
| `<space>n` | new blank panel — `tab` picks its target |
| `<space>s` | global **statistics** (every visible list) |
| `<space>c` | close this panel |
| `<space>o` | close every other panel ("only") |
| `<space>h` `<space>l` | move this panel left / right in the row |
| `<space>1`…`<space>9` | focus panel N |
| `<space>?` | key reference |

Every `<space>` press raises the which-key popup (`leaderBar`), so none of
this has to be memorised.

## Moving around — bare

| Key | Action |
|---|---|
| `h` `l` | previous / next panel |
| `j` `k` | previous / next row |
| `g` `G` | first / last row |
| `ctrl+d` `ctrl+u` | half a page |
| `gg` `G` | first / last row |
| `ctrl+d` `ctrl+u` | half page |
| `gd` | jump to the editing deck panel |
| `gv` | **versions** of the highlighted row — see below |
| `K` `J` | move the selection in the **information** panel |
| `ctrl+k` `ctrl+j` | scroll the information panel |

`h`/`l` for panels follows the Miller-column convention (ranger, lf, nnn) —
directional rather than ordinal, so you aim rather than count. Card lists are
one-dimensional, so `h`/`l` have nothing else to do.

**Note:** bubbles' list widget binds `h` `l` `f` `b` `d` `u` to paging by
default. The panel workspace doesn't use it — the card list is its own, which
is cheaper than fighting a keymap and a filter mode that both want the same
keys.

`n`/`N` are gone. With a filter that hides what doesn't match, "next match"
and "next row" are the same key, and that key is `j`.

## In a card list — bare

| Key | Action |
|---|---|
| `/` | filter, narrowing as you type · `esc` abandons it |
| `o` `O` | cycle sort forward / back |
| `v` `V` | select row / select all shown |
| `a` `x` | add / remove selected cards to the editing deck |
| `y` `p` | yank selected cards · put them in this list |
| `t` `T` | tag · tag with the last tag and add |
| `c` | set as commander |
| `s` | statistics for this list |
| `gv` | printed-text history of this card |
| `w` | write this list — see **Writing a list** |
| `W` | write it, and open the result in a new panel |
| `esc` | clear selection → clear filter → empty the panel → close it |
| | in a search bar: clear the query → leave the bar → close the panel |

`enter` and `L` are unbound here: individual cards don't open, they're shown
in the information panel as the cursor moves.

## In a decks panel — bare

| Key | Action |
|---|---|
| `i` | search bar: add a moxfield user or deck URL |
| `/` `o` `O` | filter, sort |
| `n` `r` `x` `c` | new · rename · delete · copy/sync |
| `gv` | git versions of this deck |
| `enter` `L` | open here / in a new panel |

## The editing deck

Derived, not cycled.

- Exactly one local deck panel open → it **is** the editing deck. No key, no
  concept to learn.
- Several open → the most recently focused one.
- `e` **pins** the focused panel as the target, overriding the automatic
  rule. `e` again unpins.
- The editing panel carries a distinct border colour so the target is never
  a guess. `gd` jumps to it from anywhere.

Cycling a selector (`e`/`E` walking a list of candidates) is the un-vim part
of the original design: vim's grammar is *go to the thing, then act on it*.
This keeps that grammar and makes the concept invisible in the common case.

Later, optionally: `y`/`p` to yank cards in one list and put them in another,
for ad-hoc moves that don't involve the editing deck at all.

## Versions — `gv`

One key for "how did this thing change over time", in both places the
program has a thing with a history:

- on a **deck** row — its git versions
- on a **card** row — how its printed oracle text changed across printings

This replaces the old `t` binding, which now means tag. `g` is a pure prefix
(`gg`, `gd`, `gv`) — it never acts on its own, which is what lets `gg` and
`gv` coexist.

Two keys rather than one because bare keys belong to what you press
constantly. Tagging and statistics earn one; "what did this used to say" is a
lookup, and vim charges two keys for `gJ` for the same reason.

## Yank and put — `y` / `p`

`y` yanks the selected cards, or the highlighted one if nothing is selected,
and clears the selection. `p` puts them into the focused list.

- The register carries **entries, not names** — yanking from a deck takes
  quantity and tags with it; yanking from search results takes just the card.
  Putting into a deck merges rather than duplicating.
- `p` requires a local deck, like everything else that writes.
- The count sits in the status line while the register is full.

`a` is the shorthand: `y` followed by `p` into the editing deck, in one key.
The general mechanism and the fast path for the common case, which is also
why the editing deck carries less weight than it first appears — ad-hoc
transfers between two lists never need it.

## Writing a list — `w`

`w` means *write this list*, and what that produces depends on what the list
is. Vim's `:w` versus `:w <name>`, exactly:

| The focused list is | `w` does |
|---|---|
| a local deck | saves it and commits |
| a scryfall search, or a remote deck | prompts for a name, creates a local deck, and the panel becomes that deck |

`W` does the same but opens the result in a **new** panel, leaving the search
where it was. That is the general rule: **a capital letter puts the result in
a new panel.** `L` is the one exception, and only because Enter has no
shifted form.

The name prompt defaults to something useful — the remote deck's title, or
the query string for search results.

### `w` and `c` are not the same operation

Each acts on its own level, and they produce different things:

| | acts on | produces |
|---|---|---|
| `c` (decks panel) | a **row** — one deck among many | another **row**: a local copy beside it, or a sync if the copy exists |
| `w` (card list) | the **list** — the cards in front of you | another **list**: this panel becomes the local deck, or `W` opens it in a new one |

`c` copies an item within a collection. `w` writes the thing you are looking
at. The decks panel never stops being a decks panel; the card panel stops
being remote.

## Search bars

| Key | Action |
|---|---|
| `tab` `shift+tab` | cycle search target: scryfall / decks / rules |
| `up` `down` | query history |
| `ctrl+o` | cycle the scryfall query sort (which 175 come back) |
| `enter` | run, and move to the results |
| `esc` | back to the list |

`tab` means one thing everywhere: change what this bar searches. The scryfall
*query* sort is a property of the query, not of the list on screen, so it
lives with the bar rather than on `o`/`O`.

## Global

| Key | Action |
|---|---|
| `w` `W` | write the focused list (here / in a new panel) |
| `q` | quit (prompts if the editing deck has unsaved edits) |
| `esc` | at the last panel with nothing to clear, quit |
| `?` | keys for the focused panel |

Help is scope-declared: `?` shows the bindings for the panel you're in, not a
global table. Every panel type declares its own set, and one hint line at the
bottom shows what applies right now.

## Prior art this draws on

- **ranger / lf / nnn** — Miller columns, `h`/`l` between columns, preview
  pane pinned right. Your information panel is that preview pane.
- **lazygit** — multiple panels with per-panel context keymaps and a help
  overlay scoped to the focused panel.
- **magit** — transient menus: grouped popups beat memorising flat bindings.
  `leaderBar` is a small version and can grow submenus without redesign.
- **tmux** — prefix discipline: everything about panes lives behind the
  prefix, nothing about panes lives outside it.
