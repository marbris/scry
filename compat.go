package main

import (
	"scry/internal/fetch"
	"scry/internal/mtg"
	"scry/internal/paths"
	"scry/internal/theme"
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

// ── Network ─────────────────────────────────────────────────────

type notFoundError = fetch.NotFound

const userAgent = fetch.UserAgent

var (
	doGet  = fetch.Get
	doPost = fetch.Post
	hostOf = fetch.Host
)

// ── Files ───────────────────────────────────────────────────────

var dataDir = paths.Data

// ── Colours ─────────────────────────────────────────────────────

var (
	gruvBg      = theme.Bg
	gruvBgLight = theme.BgLight
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
