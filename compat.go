package main

import (
	"scry/internal/deck"
	"scry/internal/fetch"
	"scry/internal/moxfield"
	"scry/internal/mtg"
	"scry/internal/paths"
	"scry/internal/rules"
	"scry/internal/scryfall"
	"scry/internal/stats"
	"scry/internal/theme"
	"scry/internal/ui"
)

// The bridge between the old UI and the packages the domain now lives in.
//
// Most of this file's clients — results.go, model.go, pane.go and the four
// view files — are being replaced by the panel workspace. Rewriting their
// references to the new package names would be work spent on code with a
// known expiry date, so instead the old names go on meaning what they always
// meant, and this file shrinks as the files above it are deleted. When it is
// empty, the migration is done.

// ── Cards ───────────────────────────────────────────────────────

type (
	ScryfallCard     = mtg.Card
	CardFace         = mtg.Face
	Ruling           = mtg.Ruling
	ScryfallResponse = mtg.SearchResponse
	RulingsResponse  = mtg.RulingsResponse
)

var (
	primaryType    = mtg.PrimaryType
	isLand         = mtg.IsLand
	typePrecedence = mtg.TypePrecedence
	deckSections   = mtg.Sections
)

// ── Rules ───────────────────────────────────────────────────────

type (
	RulesData     = rules.Data
	Rule          = rules.Rule
	GlossaryEntry = rules.GlossaryEntry
	Keyword       = rules.Keyword
	RuleMatch     = rules.RuleMatch
	matchKind     = rules.MatchKind
)

const (
	matchKeyword  = rules.MatchKeyword
	matchGlossary = rules.MatchGlossary
	matchType     = rules.MatchType
)

var (
	parseRules    = rules.Parse
	loadRules     = rules.Load
	downloadRules = rules.Download
	rulesFilePath = rules.FilePath
)

// ── Searching ───────────────────────────────────────────────────

var (
	sortOptions      = scryfall.SortOptions
	fetchIdentifiers = scryfall.Identifiers
	fetchCollection  = scryfall.Collection
	getRulings       = scryfall.Rulings
)

// ── Network ─────────────────────────────────────────────────────

type notFoundError = fetch.NotFound

const userAgent = fetch.UserAgent

var (
	doGet  = fetch.Get
	doPost = fetch.Post
	hostOf = fetch.Host
)

// ── Decks ───────────────────────────────────────────────────────

type (
	deckFile   = deck.File
	deckEntry  = deck.Entry
	deckInfo   = deck.Info
	deckCard   = deck.Card
	deckCommit = deck.Commit
	deckChange = deck.Change

	unresolvedError = deck.UnresolvedError
)

var (
	readDeck                = deck.Read
	writeDeck               = deck.Write
	deleteDeck              = deck.Delete
	deleteDeckCommitted     = deck.DeleteCommitted
	listDecks               = deck.List
	deckExists              = deck.Exists
	newDeck                 = deck.New
	decksDir                = deck.Dir
	deckFilePath            = deck.Path
	parseDeckFile           = deck.ParseFile
	resolveEntries          = deck.Resolve
	deckFileFrom            = deck.FileFrom
	openLocalDeck           = deck.Open
	slugify                 = deck.Slugify
	gitAvailable            = deck.GitAvailable
	deckHistory             = deck.History
	deckDiff                = deck.Diff
	deckAt                  = deck.At
	restoreDeck             = deck.Restore
	saveDeckVersioned       = deck.SaveVersioned
	diffDecks               = deck.DiffDecks
	deckHasUncommittedEdits = deck.HasUncommittedEdits
	deckRepoPath            = deck.RepoPath
	applyTagEdits           = deck.ApplyTagEdits
)

const (
	defaultFormat   = deck.DefaultFormat
	gitNotInstalled = deck.GitNotInstalled
)

// ── Moxfield ────────────────────────────────────────────────────

type (
	moxDeck     = moxfield.Deck
	moxUserDeck = moxfield.UserDeck
)

var (
	moxfieldURLID    = moxfield.URLID
	deckRef          = moxfield.Ref
	fetchMoxfield    = moxfield.Fetch
	importMoxfield   = moxfield.Import
	loadDeck         = moxfield.Load
	moxfieldUserName = moxfield.UserName
	moxDeckToFile    = moxfield.ToFile
)

// ── Statistics ──────────────────────────────────────────────────

type (
	statRow   = stats.Row
	statGroup = stats.Group
)

var statGroups = stats.Groups

// ── Files ───────────────────────────────────────────────────────

var dataDir = paths.Data

// ── Query history ───────────────────────────────────────────────

var (
	loadQueryHistory = ui.LoadQueryHistory
	saveQueryHistory = ui.SaveQueryHistory
	rememberQuery    = ui.RememberQuery
)

// ── Colours ─────────────────────────────────────────────────────

var (
	gruvBg      = theme.Bg
	gruvBgLight = theme.BgAlt
	gruvFg      = theme.Fg
	gruvFgDim   = theme.FgDim
	gruvWhite   = theme.White
	gruvGray    = theme.Gray
	gruvRed     = theme.Red
	gruvGreen   = theme.Green
	gruvYellow  = theme.Yellow
	gruvBlue    = theme.Blue
	gruvPurple  = theme.Purple
	gruvAqua    = theme.Aqua
	gruvOrange  = theme.Orange
)
