# Here's the intended design of the program

The user interface is structured as a row of vertical panels. one permanent panel to the right showing information about the active panel (eg stats, git) or active row (eg card info, rule text).

There can be any number of additional panels. Each of them have their own search bar and contents, like several buffers. The idea is to be able to look at several saved lists, remote lists, scryfall searches, and rules while editing a deck.

one of the decks can be marked as the "editing" deck. that simply means that it is the deck that is the target of adding/removing/tagging cards from other lists. only local decks can be edited. but which one is the editing deck can be cycled with e/E.

opening the program with scry
a splash screen is shown

I want a button to open a new panel. then the new panel opens with a search bar. there, i can press tab to scroll through searching scryfall/local decks/moxfield
users/rules. the results open in a list in the panel, with its own header describing the content (instead of the current search bar acting as a header).

run scry from cli. the program opens with a splash screen. i press a keybind and its empty panel opens. in addition, the information panel opens, where the information from the higlighted item is shown (card information, rules, git diffs, deck information, stats)

both scryfall query results and card lists contain lists of cards.

## keymap in general keymap

ctrl+f: open new scryfall search panel, focus on search bar
ctrl+l: open new decks search panel, focus on search bar
ctrl+r: open new rules search panel, focus on search bar

ctrl+s: open global statistics panel

left/right/h/l: cycle active panels (not information panel), focus on list (unless empty, then search bar)

e/E: cycle through local lists as editing. changing which list is referenced by the card moving/tagging commands a/x/t/T/c in the lists

w: save editable deck
q: quit
esc: quit if there are no panels.

### decks panel

ctrl+l to open a list search panel.

there you see a list of

- local lists,
- remote lists that point to moxfield, and
- moxfield usernames.

the information in the list is compact.

name type legalDeck num-cards/decks last-modified
name-local-legal-deck   L *100 3m  
name-local-illegal-deck L   143 3m  
name-remote-legal-deck  R* 100 1y
moxfield-username       U    10

as you scroll through the local/remotes/users you see basic information about the selected one in the right information panel. age, author (if from moxfield), number of cards, whether legal, whether remote, git history.

up/down/k/j to go up and down in list
/ to filter
i to go to search bar where you search moxfield usernames and moxfield list url. these are added to the list as remotes and moxfield usernames
o/O to cycle sort
n to create new local list.
x to delete local list (checking for confirmation), remove moxfield user, remove remote link.
r to rename local list or remote link
c to copy deck if local, create local copy if remote. if creating local copy from remote, and if the local copy already exists, it syncs it (overwrites and logs the changes in git).
g to see list of git versions of the highlighted (local) list. (if available)
enter to open list in this panel. if moxfield user is highlighted, open list of the user's decks
enter+shift to open list in new panel. if moxfield user is highlighted, open list of the user's decks in new panel

esc to (if viewing git versions, or if viewing moxfield user decks,) returns to initial list of decks view. remove filters, delete panel if there are no filters.

### scryfall search panel

ctrl+s to open a scryfall search panel.

up/down: query history
enter: run query, go to results list
tab/shift+tab: cycle scryfall sort search

### list of cards panels

there are four types of list of cards:

- local editable list (max one)
- local lists
- remote lists
- scryfall search results

i: search bar (for new scryfall search, or for new local/remote list search)
/: filter
o/O: cycle sort order.

v: select/deselect card
V: select/deselect all shown in list

a/x: add/remove selected cards to/from editable list.
t: tag selected cards (the cards that are in the editable list become tagged.)
T: tag selected cards with most recent tag and put them in the editable list. (if active list is not editable list)

c: select card as commander in editable list. open new deck panel if no editable deck is open, with commander as the card and deck name  

r: rename list. (only local lists)
l: save as local list and open as local list (only remote lists and search results)

s: open statistics panel for this list and filter cards by property/tags

esc: remove filters, delete panel if there are no filters.

the cards are shown on one row each. the potentially large number of panels means that the contents need to be compacted gracefully when space becomes limited:

the list shows two columns. the name of the card, and the column by which the list is being sorted (o/O).

the mana cost is written in the format 3BG

when space is short, first the type line is initialized:
Legendary Creature - Elf Faerie Noble -> LC-EFN

then if the space is short again, the name should be initialized, ending with a period to mark the abbreviation:
Dwynen, Gilt-Leaf Daen -> D,GLD.
Miara, Thorn of the Glade -> M,TotG.

#### editable panel

there is one list marked as editable. all operations revolve around editing that one. it can be changed with e/E. only local lists are editable.

there can be only one editable list at a time. a/x/t/T/c modifies the editable list.

cards in non-editable list are marked if they're also in the editable list
cards in the editable list are marked if they're also in the non-editable list

### rules panel

ctrl+r: open rules panel.

In this panel, the comprehensive rules are searched. the search gives the list of the paragraphs that contain a match to the search query. the list shows the paragraph numbers and the title of the rule. the full rules are shown in the information panel as i scroll through.

if the panel is opened while higlighting a card in a list, the rules panel opens with the list of (below) that are invoked by the highlighted card.

- abilities
- actions
- zones
- glossary terms

if there is no highlighted card, it opens an empty rules panel, with the search bar focused

up/down/k/j to go up and down in list
/: filter
i: search bar
o/O: cycle sort order.

### information panel

the information panel shows the more detailed information about whatever is highlighted among the lists. card info, deck info, rules, list statistics

up+shift/down+shift/K/J to go up and down in information panel

up+ctrl/down+ctrl/k+ctrl/j+ctrl to scroll view and down in information panel

#### statistics panel

s to open statistics panel for the active list
ctrl+s to open statistics panel for all lists collectively

up+shift/down+shift/K/J to go up and down in statistics panel

the statistics panel shows horizontal histograms of the card properties (user tags, card type, color, mana value, rarity) in the collection of cards (either just active list or all visible lists). each row corresponds to a property, and the width of the bar corresponds to the number of cards with the property.

the histograms all have the same scale.

going through the rows on the statistics page filters the card list to those cards that match that property. The histograms change to reflect the subset of cards that are now visible.

when showing and filtering all lists collectively. only some lists will have tags, but may contain cards that are tagged in a different list. those cards that are tagged are shown in the other filtered lists.
