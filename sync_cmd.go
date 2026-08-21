package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"scry/internal/deck"
	"scry/internal/theme"
)

// `scry sync` — mirroring your decks to a git remote you own.
//
// It is plain git and nothing else: the decks directory is already a git
// repository, and syncing is fetch + merge + push to a remote you point it at.
// That keeps it host-agnostic — GitHub, Codeberg, GitLab, a box in your
// cupboard — and free of any dependency beyond the git that versions the decks
// already. scry can't create the remote for you (only a host's own UI or CLI
// does that), so setup is: make one empty private repo, connect it, sync.
//
// Setting it up is a terminal job; the recurring sync is also on the s key in
// the decks panel. Both ends call deck.Sync — this file is only the words.

const syncUsage = `Usage:
  scry sync                 Pull and push your decks
  scry sync remote <url>    Connect a git remote you own
  scry sync status          Show what's connected, and what's ahead or behind
  scry sync off             Disconnect

Your decks directory is one git repository, so syncing is git pull + push of
your whole collection to a remote you own. Make one empty, private repo on any
host — GitHub, Codeberg, GitLab, self-hosted — then connect it. Inside the app,
s in the decks panel syncs.`

func runSync(args []string) {
	if len(args) == 0 {
		doSync()
		return
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Println(syncUsage)
	case "remote", "connect", "init", "url":
		syncConnect(args[1:])
	case "status":
		syncStatus()
	case "off", "disconnect", "remove":
		syncOff()
	default:
		fmt.Println(syncUsage)
		os.Exit(1)
	}
}

// setupGuide is what to do when nothing is connected yet. It leads with the one
// step scry can't do for you — creating the repo — and insists it be private,
// since these are your decks.
func setupGuide() {
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)
	fmt.Println("To sync, create one empty, private repository on any git host")
	fmt.Println("(GitHub, Codeberg, GitLab, self-hosted), then connect it:")
	fmt.Println()
	fmt.Println("  scry sync remote <url>")
	fmt.Println("  scry sync")
	fmt.Println()
	fmt.Println(dim.Render("e.g. scry sync remote git@codeberg.org:you/scry-decks.git"))
	fmt.Println(dim.Render("scry pushes your whole collection; s in the decks panel syncs too."))
}

// doSync is the bare `scry sync`: the same thing the s key does in the app.
func doSync() {
	if !deck.SyncConfigured() {
		setupGuide()
		os.Exit(1)
	}
	fmt.Println("Syncing…")
	res, err := deck.Sync()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Println(res.Summary())
}

// syncConnect points scry at a remote. With no URL it explains how to get one,
// since "how do I set this up" is the likeliest reason to type it bare.
func syncConnect(args []string) {
	if len(args) < 1 {
		setupGuide()
		return
	}
	url := strings.Join(args, " ")
	if err := deck.Connect(url, ""); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Printf("Connected to %s\n", url)
	fmt.Println(lipgloss.NewStyle().Foreground(theme.TextMuted).
		Render("Now run `scry sync` to push your decks."))
}

func syncStatus() {
	url, branch, ok := deck.SyncRemote()
	if !ok {
		fmt.Println("Not connected.")
		fmt.Println()
		setupGuide()
		return
	}

	head := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextMuted)
	fmt.Printf("%s %s\n", head.Render("remote"), url)
	fmt.Printf("%s %s\n", head.Render("branch"), branch)

	fmt.Println(dim.Render("checking the remote…"))
	st, err := deck.Status()
	if err != nil {
		fmt.Println(dim.Render("couldn't reach it: " + err.Error()))
		return
	}
	switch {
	case !st.HasRemoteBranch:
		fmt.Println(dim.Render("nothing pushed yet — run `scry sync`"))
	case st.Ahead == 0 && st.Behind == 0:
		fmt.Println(dim.Render("in sync"))
	default:
		fmt.Printf("%d to push, %d to pull — run `scry sync`\n", st.Ahead, st.Behind)
	}
}

func syncOff() {
	if !deck.SyncConfigured() {
		fmt.Println("Syncing wasn't set up.")
		return
	}
	if err := deck.Disconnect(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Println("Disconnected. Your decks and their history are untouched.")
}
