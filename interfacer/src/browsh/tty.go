package browsh

import (
	"fmt"
	"os"
	"encoding/json"

	"github.com/gdamore/tcell"
	"github.com/go-errors/errors"
)

var (
	screen tcell.Screen
	uiHeight = 2
	// IsMonochromeMode decides whether to render the TTY in full colour or monochrome
	IsMonochromeMode = false
)

func setupTcell() {
	var err error
	if err = screen.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	screen.EnableMouse()
	screen.Clear()
}

func sendTtySize() {
	x, y := screen.Size()
	sendMessageToWebExtension(fmt.Sprintf("/tty_size,%d,%d", x, y))
}

// This is basically a proxy that listens to STDIN and forwards all relevant input
// from the user to the webextension. So keyboard, mouse, terminal resizes, etc.
func readStdin() {
	for {
		ev := screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventKey:
			handleUserKeyPress(ev)
		case *tcell.EventResize:
			handleTTYResize()
		case *tcell.EventMouse:
			handleMouseEvent(ev)
		}
	}
}

func handleUserKeyPress(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyCtrlQ:
		quitBrowsh()
	case tcell.KeyCtrlL:
		urlBarFocusToggle()
	}
	if (ev.Rune() == 'm' && ev.Modifiers() == 4) {
		toggleMonochromeMode()
	}
	forwardKeyPress(ev)
	if activeInputBox != nil {
		handleInputBoxInput(ev)
	} else {
		handleScrolling(ev) // TODO: shouldn't you be able to still use mouse scrolling?
	}
}

func quitBrowsh() {
	if !*isUseExistingFirefox {
		quitFirefox()
	}
	Shutdown(errors.New("normal"))
}

func urlBarFocusToggle() {
	if urlInputBox.isActive {
		activeInputBox = nil
		urlInputBox.isActive = false
	} else {
		activeInputBox = &urlInputBox
		urlInputBox.isActive = true
		urlInputBox.text = ""
	}
}

func toggleMonochromeMode() {
	IsMonochromeMode = !IsMonochromeMode
}

func forwardKeyPress(ev *tcell.EventKey) {
	eventMap := map[string]interface{}{
		"key":  int(ev.Key()),
		"char": string(ev.Rune()),
		"mod":  int(ev.Modifiers()),
	}
	marshalled, _ := json.Marshal(eventMap)
	sendMessageToWebExtension("/stdin," + string(marshalled))
}

func handleScrolling(ev *tcell.EventKey) {
	yScrollOriginal := CurrentTab.frame.yScroll
	_, height := screen.Size()
	height -= uiHeight
	if ev.Key() == tcell.KeyUp {
		CurrentTab.frame.yScroll -= 2
	}
	if ev.Key() == tcell.KeyDown {
		CurrentTab.frame.yScroll += 2
	}
	if ev.Key() == tcell.KeyPgUp {
		CurrentTab.frame.yScroll -= height
	}
	if ev.Key() == tcell.KeyPgDn {
		CurrentTab.frame.yScroll += height
	}
	CurrentTab.frame.limitScroll(height)
	sendMessageToWebExtension(
		fmt.Sprintf(
			"/tab_command,/scroll_status,%d,%d",
			CurrentTab.frame.xScroll,
			CurrentTab.frame.yScroll * 2))
	if (CurrentTab.frame.yScroll != yScrollOriginal) {
		renderCurrentTabWindow()
	}
}

func handleMouseEvent(ev *tcell.EventMouse) {
	x, y := ev.Position()
	button := ev.Buttons()
	eventMap := map[string]interface{}{
		"button":    int(button),
		"mouse_x":   int(x + CurrentTab.frame.xScroll),
		"mouse_y":   int(y - uiHeight + CurrentTab.frame.yScroll),
		"modifiers": int(ev.Modifiers()),
	}
	marshalled, _ := json.Marshal(eventMap)
	sendMessageToWebExtension("/stdin," + string(marshalled))
}

func handleTTYResize() {
	width, _ := screen.Size()
	// TODO: How does this work with wide UTF8 chars?
	urlInputBox.Width = width - len(urlBarControls)
	screen.Sync()
	sendTtySize()
}

// Tcell uses a buffer to collect screen updates on, it only actually sends
// ANSI rendering commands to the terminal when we tell it to. And even then it
// will try to minimise rendering commands by only rendering parts of the terminal
// that have changed.
func renderCurrentTabWindow() {
	var currentCell cell
	var styling = tcell.StyleDefault
	var runeChars []rune
	width, height := screen.Size()
	if CurrentTab.frame.cells == nil { return }
	for y := 0; y < height - uiHeight; y++ {
		for x := 0; x < width; x++ {
			currentCell = getCell(x, y)
			runeChars = currentCell.character
			// TODO: do this is in isCharacterTransparent()
			if (len(runeChars) == 0) { continue }
			if IsMonochromeMode {
				styling = styling.Foreground(tcell.ColorWhite)
				styling = styling.Background(tcell.ColorBlack)
				if runeChars[0] == '▄' {
					runeChars[0] = ' '
				}
			} else {
				styling = styling.Foreground(currentCell.fgColour)
				styling = styling.Background(currentCell.bgColour)
			}
			screen.SetCell(x, y + uiHeight, styling, runeChars[0])
		}
	}
	screen.Show()
}

func getCell(x, y int) cell {
	var currentCell cell
	var ok bool
	frame := &CurrentTab.frame
	index := ((y + frame.yScroll) * frame.totalWidth) + ((x + frame.xScroll))
	if currentCell, ok = frame.cells.load(index); !ok {
		fgColour, bgColour := getHatchedCellColours(x)
		currentCell = cell{
			fgColour: fgColour,
			bgColour: bgColour,
			character: []rune("▄"),
		}
	}
	return currentCell
}

func getHatchedCellColours(x int) (tcell.Color, tcell.Color) {
	var bgColour, fgColour tcell.Color
	if (x % 2 == 0) {
		bgColour = tcell.NewHexColor(0xa9a9a9)
		fgColour = tcell.NewHexColor(0x797979)
	} else {
		bgColour = tcell.NewHexColor(0x797979)
		fgColour = tcell.NewHexColor(0xa9a9a9)
	}
	return fgColour, bgColour
}
