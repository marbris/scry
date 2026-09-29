package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg" // Scryfall's pictures are JPEGs
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"scry/internal/browser"
	"scry/internal/fetch"
	"scry/internal/mtg"
	"scry/internal/paths"
	"scry/internal/prints"
)

// gx: the card as printed, in the information panel.
//
// A terminal that speaks kitty's graphics protocol — kitty, ghostty, WezTerm
// — can draw a picture in among the text, and there gx shows the printing
// right where the card's text was. Anywhere else, gx opens the picture in the
// browser: a card you can't see is worse than a card in another window.
//
// The picture is drawn with kitty's Unicode placeholders rather than placed
// at a screen position. The image is sent once, off to the side, as a
// "virtual placement"; the panel then draws it with ordinary characters —
// one special character a cell, coloured with the image's number — which the
// terminal replaces with the picture. They are text as far as everything
// else is concerned, so the picture moves, scrolls and is overdrawn exactly
// the way the text around it is, and Bubbletea's redrawing never has to know
// there is a picture at all.

// kittyImageID is the one image scry keeps in the terminal. It is carried in
// the placeholder's 256-colour foreground, so it has to be under 256; one
// picture is on screen at a time, so one number is all it needs.
const kittyImageID = 219

// imageDelay is how long the cursor rests on a card before its picture is
// asked for, so walking down a list with j isn't a download per card.
const imageDelay = 250 * time.Millisecond

// kittyGraphics reports whether the terminal can draw pictures. A variable,
// so the tests can decide.
var kittyGraphics = func() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty" {
		return true
	}
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "ghostty", "wezterm":
		return true
	}
	return false
}

// writeTerminal sends bytes to the terminal outside Bubbletea's drawing, for
// the image itself. One write, so it can't be split by a redraw going out at
// the same moment. A variable, so the tests can catch it.
var writeTerminal = func(b []byte) { os.Stdout.Write(b) }

type imgState int

const (
	imgFetching imgState = iota
	imgReady
	imgFailed
)

// cardImage is one card's picture, and how far along getting it is.
type cardImage struct {
	state    imgState
	printing mtg.Card
	png      []byte
	w, h     int // in pixels
	err      error
}

// kittyShown is what the terminal is holding under kittyImageID: which card,
// at what size in cells. The placeholders only draw once it's there.
type kittyShown struct {
	key        string
	cols, rows int
}

type imageMsg struct {
	key      string
	printing mtg.Card
	png      []byte
	w, h     int
	err      error
}

type imageTickMsg struct {
	key string
	seq int
}

// imageKey is what a card's picture is filed under: the card, not the
// printing, since which printing is shown is the answer, not the question.
func imageKey(c mtg.Card) string {
	if c.OracleID != "" {
		return c.OracleID
	}
	return strings.ToLower(c.Name)
}

// gxCard is gx on a list of cards: its picture in the information panel, or
// in the browser where the terminal can't draw one.
func (m *Model) gxCard(c mtg.Card) tea.Cmd {
	if c.Name == "" {
		return nil
	}
	if !kittyGraphics() {
		m.notice = "this terminal can't draw pictures — opening it in the browser"
		return openPrintingInBrowser(c)
	}
	m.info.mode = infoImage
	m.info.offset = 0
	return m.fetchImage(c)
}

// fetchImage asks for a card's picture, unless it has it or is asking.
func (m *Model) fetchImage(c mtg.Card) tea.Cmd {
	key := imageKey(c)
	if img, ok := m.images[key]; ok && img.state != imgFailed {
		return nil
	}
	m.images[key] = &cardImage{state: imgFetching}
	return func() tea.Msg {
		p, err := prints.Preferred(c)
		if err != nil {
			return imageMsg{key: key, err: err}
		}
		data, w, h, err := printingPNG(p)
		return imageMsg{key: key, printing: p, png: data, w: w, h: h, err: err}
	}
}

// imageHover follows the cursor while the picture is up: rest on a card and
// its picture is asked for.
func (m *Model) imageHover() tea.Cmd {
	if m.info.mode != infoImage {
		return nil
	}
	c := m.focusedCard()
	if c == nil {
		return nil
	}
	key := imageKey((*c))
	if _, ok := m.images[key]; ok {
		return nil
	}
	m.imageSeq++
	seq := m.imageSeq
	return tea.Tick(imageDelay, func(time.Time) tea.Msg { return imageTickMsg{key: key, seq: seq} })
}

func (m Model) handleImageTick(msg imageTickMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.imageSeq || m.info.mode != infoImage {
		return m, nil
	}
	c := m.focusedCard()
	if c == nil || imageKey((*c)) != msg.key {
		return m, nil
	}
	cmd := m.fetchImage((*c))
	return m, cmd
}

func (m Model) handleImage(msg imageMsg) (tea.Model, tea.Cmd) {
	img := m.images[msg.key]
	if img == nil {
		return m, nil
	}
	if msg.err != nil {
		img.state, img.err = imgFailed, msg.err
		return m, nil
	}
	img.state = imgReady
	img.printing, img.png, img.w, img.h = msg.printing, msg.png, msg.w, msg.h
	return m, nil
}

// printingPNG is a printing's picture as a PNG — the one format kitty takes
// without being told the pixel size — from the cache if it has been fetched
// before.
func printingPNG(p mtg.Card) ([]byte, int, int, error) {
	path := filepath.Join(paths.Cache(), "images", p.ID+".png")
	if data, err := os.ReadFile(path); err == nil {
		if cfg, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
			return data, cfg.Width, cfg.Height, nil
		}
	}

	raw, err := fetch.GetFile(p.Image("normal"))
	if err != nil {
		return nil, 0, 0, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	// The cache is a convenience: failing to keep a copy loses nothing.
	if os.MkdirAll(filepath.Dir(path), 0755) == nil {
		os.WriteFile(path, buf.Bytes(), 0644)
	}
	b := img.Bounds()
	return buf.Bytes(), b.Dx(), b.Dy(), nil
}

// openPrintingInBrowser finds the printing gx would show, and opens its
// picture in the browser.
func openPrintingInBrowser(c mtg.Card) tea.Cmd {
	return func() tea.Msg {
		p, err := prints.Preferred(c)
		if err != nil {
			return noticeMsg{err: err}
		}
		if err := browser.Open(p.Image("large")); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "opened " + p.Name + " (" + strings.ToUpper(p.Set) + ") in the browser"}
	}
}

// openInBrowser opens a link, reporting back only if it couldn't.
func openInBrowser(url, what string) tea.Cmd {
	return func() tea.Msg {
		if err := browser.Open(url); err != nil {
			return noticeMsg{err: err}
		}
		return noticeMsg{text: "opened " + what + " in the browser"}
	}
}

// ── Drawing ─────────────────────────────────────────────────────

// imageFit is the size in cells a card's picture takes in a space: the
// full width if the height allows, otherwise the full height. A cell is
// about twice as tall as it is wide.
func imageFit(w, h, cols, rows int) (int, int) {
	if w <= 0 || h <= 0 || cols <= 0 || rows <= 0 {
		return 0, 0
	}
	fitRows := int(float64(cols)*float64(h)/float64(w)/2 + 0.5)
	if fitRows <= rows {
		return cols, maxInt(fitRows, 1)
	}
	fitCols := int(float64(rows)*float64(w)*2/float64(h) + 0.5)
	return maxInt(minInt(fitCols, cols), 1), rows
}

// syncImage keeps the terminal holding the picture the panel wants to draw,
// at the size it wants to draw it: sent when it changes, and taken back when
// the panel stops showing pictures.
func (m Model) syncImage() (Model, tea.Cmd) {
	want := kittyShown{}
	var img *cardImage
	if m.info.mode == infoImage {
		if c := m.focusedCard(); c != nil {
			if i, ok := m.images[imageKey((*c))]; ok && i.state == imgReady {
				if inner, room, _, ok := m.infoSpan(); ok {
					cols, rows := imageFit(i.w, i.h, inner, room-imageCaptionRows)
					if cols > 0 && rows > 0 && rows <= len(placeholderDiacritics) {
						want = kittyShown{imageKey((*c)), cols, rows}
						img = i
					}
				}
			}
		}
	}
	if want == m.kitty {
		return m, nil
	}
	m.kitty = want
	if img == nil {
		if !kittyGraphics() {
			return m, nil
		}
		return m, func() tea.Msg { writeTerminal([]byte(kittyDelete())); return nil }
	}
	payload := kittyDelete() + kittyTransmit(img.png, want.cols, want.rows)
	return m, func() tea.Msg { writeTerminal([]byte(payload)); return nil }
}

// imageCaptionRows is the room under the picture for which printing it is.
const imageCaptionRows = 2

func kittyDelete() string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", kittyImageID)
}

// kittyTransmit sends a PNG as a virtual placement of cols×rows cells, in
// the chunks the protocol asks for. q=2 keeps the terminal from answering,
// which would otherwise arrive as keypresses.
func kittyTransmit(data []byte, cols, rows int) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	const chunk = 4096
	for i := 0; i < len(enc); i += chunk {
		end := minInt(i+chunk, len(enc))
		more := 1
		if end == len(enc) {
			more = 0
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=T,U=1,f=100,q=2,i=%d,c=%d,r=%d,m=%d;%s\x1b\\",
				kittyImageID, cols, rows, more, enc[i:end])
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, enc[i:end])
		}
	}
	return b.String()
}

// placeholderRows are the lines that draw the picture: each row's first cell
// names its row and column with diacritics, and the rest inherit from the
// cell to their left, a column further along.
func placeholderRows(cols, rows int) []string {
	out := make([]string, rows)
	cell := string(rune(0x10EEEE))
	for r := 0; r < rows; r++ {
		var b strings.Builder
		fmt.Fprintf(&b, "\x1b[38;5;%dm", kittyImageID)
		b.WriteString(cell)
		b.WriteRune(placeholderDiacritics[r])
		b.WriteRune(placeholderDiacritics[0])
		b.WriteString(strings.Repeat(cell, cols-1))
		b.WriteString("\x1b[39m")
		out[r] = b.String()
	}
	return out
}

// infoImageLines is the information panel with the picture up.
func (m Model) infoImageLines(width int) []string {
	c := m.focusedCard()
	if c == nil {
		return []string{mutedLine("no card here", width)}
	}
	img, ok := m.images[imageKey((*c))]
	switch {
	case !ok || img.state == imgFetching:
		return []string{mutedLine("fetching "+c.Name+"…", width)}
	case img.state == imgFailed:
		return []string{mutedLine("no picture: "+img.err.Error(), width)}
	}
	if m.kitty.key != imageKey((*c)) {
		return []string{mutedLine("drawing…", width)}
	}
	lines := placeholderRows(m.kitty.cols, m.kitty.rows)
	for i := range lines {
		lines[i] += strings.Repeat(" ", maxInt(width-m.kitty.cols, 0))
	}
	p := img.printing
	caption := p.SetName + " (" + strings.ToUpper(p.Set) + ")"
	if len(p.ReleasedAt) >= 4 {
		caption += " · " + p.ReleasedAt[:4]
	}
	return append(lines, "", mutedLine(caption, width))
}
