1. the info panel is too narrow. the width appears to be fixed at a very small number, not changing as the number of panels grow. it should have the same size as all the other panels: 1/2, 1/3, 1/4 of the screen, etc.

2. ctrl + right/left/h/l should move the panel to the right and left.

3. when opening a new panel with space n, and then tab to the deck panel, the list of decks should populate immediately. its currently empty until you press enter. (it complains about the lack of search input, but then displays all the decks.)

4. pressing c from a card list creates a new deck with that card as commander. remove this functionality.

5. space 1-9 takes focuses panel N. remove this keymap. there will never be enough panels to justify it.

6. remove the ctrl+d/u to scroll half a page keymap.

7. ctrl+j/k should scroll through the information panel by paragraph (instead of scrolling), analogous to how it currently scrolls by category in statistics panel. the information panel sometimes has several paragraphs of rulings.

8. in the leader menu (pressing space), some of the space appears to be filled in with a background color. remove this color. it contrasts with my otherwise semi-transparent terminal everywhere else.

9. the hints in the hint bar can be written on fewer lines, with more hints on each line, extending further to the right. they should also be grouped better, with 'headers' to the left. like so
navigation: h/[arrow left] l/[arrow right] k/[arrow up] j/[arrow down] left right up down, gg G first last, o O sort, / filter, i search, esc clear then exit, space leader menu,  q quit scry
select: v V select one/all, y / p yank/put, w W save list as local deck and open, gv open version history
edit [edit deck]: a x u t add/remove/undo/tag, T tag with recent and add, c select as commander, e E cycle edit deck
info panel: s statistics, J/K up/down, ctrl+j/k up/down by paragraph,
etc.

10. the line showing the most recent change should be on its own line above the hints.

11. in the card information panel, the power/toughness is shown on the same line as the card type. it gets obscured when the panel gets narrower. the power/toughness should be on its own row.

12. i should be able to sort scryfall query by power and toughness.

13. i should be able to sort card lists by power and toughness. both power and toughness should be shown together (power/toughness) when sorted by either.

---

## What changed

All thirteen are done. Where a change had a subtlety worth keeping, it's noted.

| # | What it was | Where it was put right |
|---|---|---|
| 1 | information panel a fixed narrow width | `internal/ui/layout.go` |
| 2 | no way to move a panel without the leader | `internal/ui/keys.go` |
| 3 | tabbing to the decks target left the panel blank until enter | `internal/ui/keys.go`, `panel.go` |
| 4 | `c` conjured a whole deck up around a card | `internal/ui/edit.go` |
| 5 | `space 1-9` focused a panel by number | `internal/ui/keys.go`, `render.go`, `splash.go` |
| 6 | `ctrl+d`/`ctrl+u` scrolled half a page | `internal/ui/view.go`, `rulesview.go`, `keys.go` |
| 7 | `ctrl+j`/`ctrl+k` scrolled the info panel a line at a time | `internal/ui/render.go`, `keys.go` |
| 8 | the leader menu painted a background band | `internal/ui/render.go` |
| 9 | the hint bar was one long ungrouped run | `internal/ui/hints.go`, `render.go`, the views |
| 10 | the last result shared the keys' first line | `internal/ui/render.go` |
| 11 | power/toughness rode the end of the type line | `internal/ui/cardinfo.go` |
| 12 | (already worked — `ctrl+o` reaches power and toughness) | test in `find_test.go` |
| 13 | no card-list sort by power or toughness | `internal/ui/sort.go` |

### The information panel is a column now (1)

It used to be a quarter of the terminal, clamped to a prose-friendly band.
It's an ordinary column now: with one panel open it's half the screen, with
two it's a third, and so on — the same share every panel gets, held to the
same `minPanel` floor, dropped only when the terminal is too narrow for even
a single panel beside it.

### The decks preview (3)

Tabbing round to the decks target fills the body with your decks while the
bar stays open over them, so they appear without waiting for enter. It's a
*preview*: tab may replace it freely, but it must not wipe a real search or
deck reached by reopening the bar with `i`. A `previewing` flag on the panel
is what tells the two apart; enter or leaving the bar commits it.

### The hint bar is grouped, the notice is above it (9, 10)

The keys are grouped — navigation, select, edit, info panel — each line led
by what it is, which is what lets the bar put more on a line and read at a
glance. There is still one source (`hintGroups`), which the reference
flattens: each view declares its own groups and the workspace folds in the
keys it owns everywhere, so no view repeats them and none can name `esc`
twice. The editing keys are headed by the deck they change (`edit · Ghen`)
rather than trailing every key with it. The last thing you did sits on its
own line above the keys; the panel count stays out to the right of the first
key line.

### Paragraph scrolling (7)

`ctrl+j`/`ctrl+k` move the information panel a paragraph at a time — the same
idea as jumping a whole group in statistics, one level coarser than `K`/`J`.
Paragraphs are the blank-line-separated blocks the card panel already draws:
type, text, printing, and each ruling. The lines the jump counts and the
lines the panel draws come from one place (`infoContent`), so they can't
disagree about where a paragraph begins.

### Power and toughness (11, 12, 13)

They're on their own row in the card panel now, so a narrow panel gives up
the end of the type line before it gives up the `2/3`. Card lists sort by
either, biggest first, cards without them last; the column shows both
together whichever you sorted by. The Scryfall *query* sort already reached
power and toughness through `ctrl+o` — a test pins that it still does.

---

## A second pass: two more (14, 15)

| # | What it was | Where it was put right |
|---|---|---|
| 14 | the hint bar showed a panel index (`2/3`) | `internal/ui/render.go` |
| 15 | the hint bar was too tall — groups wrapped over several rows | `internal/ui/render.go`, `hints.go`, the views |

### The bar is one row per group now (14, 15)

The panel index is gone. Each group is a single row across the full width,
its labels terse — `up/down`, `add/remove`, `text history` — because the bar
is a reminder, not the manual. A key that would overrun its row is dropped
rather than wrapped; the full set, with its longer descriptions, is one `?`
away. The reference is grouped to match — one section per group — so it still
carries the group headings, which is where the editing keys name the deck
they change (`edit · Ghen`).

---

## And a third: the reference folds into the bar (16)

| # | What it was | Where it was put right |
|---|---|---|
| 16 | `?` opened a separate full-screen reference window | `internal/ui/render.go`, `keys.go`, `ui.go`, `splash.go` |

The hint bar rests at three keys — `space menu · ? keys · q quit` — the ones
that reach everything else. `?` grows it in place to the whole grouped keymap
and `?` again shrinks it back; it stays where you put it rather than closing
on the next key, so you can read it and act at once. The panels give up the
room while it's grown, the way they do for the leader menu.

The full-screen reference window is gone, and with it `viewKeys`,
`keySections`, `packSections`, `renderSections`, `interleave`, `hereTitle`
and the `keySection` type. There is now a single rendering of the keymap —
the bar — in two sizes, rather than a bar and a window that had to be kept in
step. `hintsExpanded` on the model is the toggle; `footerGroups` chooses the
resting three keys or the full set.
