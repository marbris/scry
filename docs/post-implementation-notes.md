# Post-implementation notes

Nineteen things found by using the program after the twelve phases landed.
All of them are fixed; this is kept as the record of what was wrong and where
it was put right, because several were the same mistake wearing different
clothes.

| # | What was wrong | Fixed in |
|---|---|---|
| 1 | mana cost not coloured by symbol in card lists | `147adbe` |
| 2 | sorting by colour didn't colour the name | `147adbe` |
| 3 | sorting by type didn't colour the name or type line | `147adbe` |
| 16 | split-card cost read `2U // {2R` — a stray brace | `147adbe` |
| 4 | `/` did nothing when browsing somebody's decks | `b689534` |
| 5 | a deck with no cards showed a blank, not `0` | `b689534` |
| 6 | the decks panel had no column for a deck's colours | `b689534` |
| 7 | opening a remote deck didn't follow it, so it never appeared as `R` | `b689534` |
| 8 | `?` — the "Here" section ignored which panel was active | `6b1d8bf` |
| 9 | the leader menu's `r` showed two question-mark glyphs | `6b1d8bf` |
| 10 | no leader hint for `s` or `c` | `6b1d8bf` |
| 11 | the leader menu pushed the panels off the top of the screen | `6b1d8bf` |
| 12 | the leader menu stayed up after `space s` | `6b1d8bf` |
| 13 | `gv` on a card in a deck showed the *deck's* history | `ce737a9` |
| 14 | `s` said "nothing to count" over a search still filling | `ce737a9` |
| 15 | sorting by colour didn't sink the lands | `ce737a9` |
| 17 | the editing deck followed the cursor instead of `e`/`E` | `ce737a9` |
| 18 | statistics didn't scroll when the selection went past the bottom | `ab0a5b5` |
| 19 | no way to move through statistics a group at a time | `ab0a5b5` |

Two more from the next session's use, one of them a note reopened:

| # | What was wrong | Fixed in |
|---|---|---|
| 14b | `s` on a **search** still said "nothing to count" | below |
| 20 | no way out of the printed-text history `gv` opens | below |

## What the last two changed

Note 14 was only half fixed. Deriving the bars each frame stopped them going
stale, which was the reported symptom on a deck; a search had never had any.
The bars sum quantities, a search result has none — it isn't in a deck and
nobody has chosen how many — so every row's base came to zero, and a row
nothing has ever matched is dropped. Every group was dropped, and the panel
said "nothing to count" over a screen full of cards. A card with no quantity
counts as one card. Both ways into a deck floor the quantity at one, so a
quantity below one can only mean an entry that was never in a deck.

`gv` was the only key into the printed-text history and there was no key out.
`esc` fell straight through to taking the panel apart, and moving to another
card left the history standing over it — showing "gv for how its text has
changed", which is the prompt to press the key you had just pressed. The view
belongs to the card it was opened on, so it lasts as long as the cursor stays
there: `esc` leaves it, and so does moving off. That is checked in one place,
after every keypress, rather than in each key that can move the cursor —
`j`, `k`, `gg`, `G`, `ctrl+d`, a filter narrowing the list out from under it
— because that list is exactly the one somebody adds to and forgets.

## What 18 and 19 changed

The statistics panel scrolls to wherever the highlighted category is rather
than keeping an offset of its own. The category *is* the position, so
deriving it can't drift out of step with it — which is exactly how `J` past
the bottom came to be moving a cursor you could no longer see.

`ctrl+k` and `ctrl+j` move a whole group — tags, types, colours, the curve.
They already scroll the information panel elsewhere, and this is the same
axis one level coarser, which is what `ctrl` means throughout. Five groups of
a dozen rows is a great deal of `J` to reach the curve.

## The patterns worth remembering

Three of these were one bug. A filter that matches everything looks exactly
like a filter that was never applied, so a view that forgot to implement the
filter interface failed silently — first in the rules panel, then in
somebody's decks, then in a deck's versions. It is an interface now, so a
view that can be narrowed says so in its own file.

Two were the value-receiver trap this project keeps meeting: a method on a
`Model` value mutates a copy, so the fix is always to derive rather than to
store and update.

Two more were width measured in runes over text carrying ANSI escapes, which
wraps a row and grows a panel. Measure with the escapes stripped.
