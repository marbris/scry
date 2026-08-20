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
| 18 | statistics didn't scroll when the selection went past the bottom | below |
| 19 | no way to move through statistics a group at a time | below |

## What the last two changed

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
