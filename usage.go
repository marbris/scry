package main

import "fmt"

const cliUsage = `scry — Magic: The Gathering cards, rules and decks in the terminal

Usage:
  scry                      Come back to the panels you left
  scry <query>              Run a Scryfall query; one result prints to stdout
  scry <moxfield url>       Look at a deck on Moxfield
  scry deck …               Your decks — see ` + "`scry deck`" + `
  scry sync …               Mirror your decks to a git remote — see ` + "`scry sync`" + `
  scry rules …              The comprehensive rules — see ` + "`scry rules`" + `
  scry theme …              Colours — see ` + "`scry theme`" + `
  scry -v, --version        Print the version
  scry -h, --help           This

Queries use Scryfall's own syntax:
  scry 't:creature c:rw cmc<=3'
  scry 'o:"draw a card" f:commander'

The app is a row of panels. space opens the menu, ? shows the keys.

Files follow the XDG directories: decks in the data directory, settings and
themes in the config directory, session state in the state directory, and
everything re-downloadable in the cache. Decks are files in a git repository
— SCRY_DECKS_DIR moves them somewhere else.`

func printUsage() { fmt.Println(cliUsage) }
