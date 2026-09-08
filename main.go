//
// horse: https://github.com/if-not-nil/horse
// a better cd-ls
//

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"net/http" // todo: another library for filetypes
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/SerenaFontaine/kgp"
	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/mattn/go-runewidth"
	"github.com/mattn/go-sixel"
	"golang.org/x/image/draw"
	xterm "golang.org/x/term"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// the horse's mane
func main() {
	flag.BoolVar(&showPreview, "preview", true, "show a file preview on the right side")
	flag.BoolVar(&showPreview, "p", true, "alias for -preview")
	flag.Parse()

	loadConfig() // keybinding overrides, if any

	// probe for sixel before tcell grabs the terminal (it needs a raw-mode query)
	sixelOK = detectSixel()

	s, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("%+v", err)
	}
	screen = s
	if err := screen.Init(); err != nil {
		log.Fatalf("%+v", err)
	}
	width, height = screen.Size()

	// kitty images go to /dev/tty since stdout gets eval'd by the shell
	ttyFile, _ = os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	term := os.Getenv("TERM")
	kittyOK = ttyFile != nil && (term == "xterm-kitty" ||
		strings.Contains(term, "ghostty") ||
		os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" ||
		os.Getenv("WEZTERM_PANE") != "" ||
		// tmux hides the outer terminals env vars and rewrites TERM,
		// but forwards kitty graphics if `set -g allow-passthrough on` is set
		// everyone has this enabled already
		inTmux)
	if kittyOK {
		sixelOK = false // kitty is nicer, prefer it when we have both
	}

	screen.SetStyle(STYLE_BG)

	screen.Clear()

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err, "getpwd")
	}

	SwitchDir(cwd)
	Redraw()

	for {
		screen.Show()

		ev := screen.PollEvent()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			width, height = screen.Size()
			// force the image to be redrawn at the new size
			if KittyShown != "" {
				kittyClear()
				KittyShown = ""
			}
			SixelShown = "" // sixel is erased by the Sync repaint below
			screen.Sync()
			Redraw()
		case *tcell.EventKey:
			if ActivePrompt.IsActive {
				HandlePromptInput(ev)
				Redraw()
				continue
			}
			if Edit != editNone {
				HandleEditInput(ev)
				Redraw()
				continue
			}
			HandleKey(ev)
			Redraw()
		}
	}
}

// ok to tune
const (
	reservedRows        = 3                // rows of chrome (yk like chrome) (pwd line + margins) subtracted from height for the file list
	maxInputLength      = 100              // max chars in the search/filter box
	maxTextPreviewSize  = 50 * 1000        // skip syntax-highlighted preview above this big
	maxImagePreviewSize = 10 * 1000 * 1000 // skip image preview above this big
	previewReadLimit    = 10000            // bytes read for syntax highlighting
)

var (
	width                       = 20
	height                      = 20
	STYLE_BG                    = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	STYLE_DIR                   = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorDarkCyan)
	STYLE_DIR_SEL               = tcell.StyleDefault.Background(tcell.ColorDarkCyan).Foreground(tcell.ColorWhite)
	STYLE_FG                    = tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
	STYLE_MID                   = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorGrey)
	showPreview                 = false
	HL_STYLE      *chroma.Style = styles.Get("monokai")
	screen        tcell.Screen
	kittyOK       = false                   // terminal speaks the kitty graphics protocol
	sixelOK       = false                   // terminal speaks sixel (fallback when no kitty)
	inTmux        = os.Getenv("TMUX") != "" // /dev/tty is tmux's pty, not the real terminal
	ttyFile       *os.File                  // where we write kitty escapes (stdout is eval'd)

	Pwd       string
	Input     string
	Files     []os.DirEntry
	Results   []os.DirEntry
	Selected  int
	TopIndex  int
	listCache []string // cached CurrentList() cleared by invalidateList

	LastSel map[string]string // remembers the cursor per dir
	PrevDir string            // last dir we were in, for ~ toggle

	Edit     editKind   // which inline edit (rename/copy) is active, if any
	EditOrig string     // the file being renamed/copied
	EditBuf  LineEditor // the edited name/destination

	Selecting  bool            // multiselect mode is on
	Sel        map[string]bool // names marked in current dir
	LastMarked string          // most recently marked name for C-x to jump back to

	KittyShown string // path of the image currently drawn via kitty (for caching)
	SixelShown string // path of the image currently drawn via sixel (for caching)

	ActivePrompt Prompt
)

type editKind int

// go enums look do like this
const (
	editNone editKind = iota
	editRename
	editCopy
)

//////////////////
// key handling //
//////////////////

// HandleKey dispatches a key event in normal mode (no prompt or inline edit active).
// bindings live in keyBindings / runeBindings, which loadConfig can override
func HandleKey(ev *tcell.EventKey) {
	if ev.Key() == tcell.KeyRune {
		// most runes go to the search box; only bound runes (e.g. ~) are commands,
		// and only when not mid-search so filenames stay typeable
		if act, ok := runeBindings[ev.Rune()]; ok && Input == "" {
			runAction(act)
			return
		}
		doInput(ev.Rune())
		return
	}
	if act, ok := keyBindings[ev.Key()]; ok {
		runAction(act)
	}
}

func runAction(name string) {
	if fn := actions[name]; fn != nil {
		fn()
	}
}

//////////////////////
// keybinding config //
//////////////////////

// actions maps a config action name to what it does in normal mode
var actions = map[string]func(){
	"quit":        cancelOrQuit,
	"down":        func() { MoveCursor(1) },
	"up":          func() { MoveCursor(-1) },
	"select":      selectOrToggle,
	"cd":          quitOnPwd,
	"updir":       upDir,
	"delchar":     func() { backspace(false) },
	"delword":     func() { backspace(true) },
	"home":        toggleHome,
	"copypath":    copySelectedPath,
	"open":        openSelected,
	"delete":      promptDelete,
	"rename":      startRename,
	"copy":        startCopy,
	"multiselect": handleMultiSelect,
	"create":      promptCreate,
	"prevdir":     togglePrevDir,
}

// default bindings, all overridable through the config file
var keyBindings = map[tcell.Key]string{
	tcell.KeyCtrlS:      "copypath",
	tcell.KeyCtrlO:      "open",
	tcell.KeyCtrlD:      "delete",
	tcell.KeyCtrlR:      "rename",
	tcell.KeyCtrlY:      "copy",
	tcell.KeyCtrlX:      "multiselect",
	tcell.KeyCtrlA:      "create",
	tcell.KeyEscape:     "quit",
	tcell.KeyCtrlC:      "quit",
	tcell.KeyDown:       "down",
	tcell.KeyCtrlJ:      "down",
	tcell.KeyCtrlN:      "down",
	tcell.KeyUp:         "up",
	tcell.KeyCtrlK:      "up",
	tcell.KeyCtrlP:      "up",
	tcell.KeyTab:        "select",
	tcell.KeyCtrlL:      "select",
	tcell.KeyCtrlF:      "select",
	tcell.KeyEnter:      "cd",
	tcell.KeyCtrlH:      "updir", // same code as backspace; real backspace is KeyBackspace2
	tcell.KeyCtrlB:      "updir",
	tcell.KeyBackspace2: "delchar",
	tcell.KeyCtrlW:      "delword",
	tcell.KeyCtrlE:      "home",
}

var runeBindings = map[rune]string{
	'~': "prevdir",
}

// configPath resolves the config file: $HORSE_CONFIG, else %APPDATA%\horse\config on
// windows, else $XDG_CONFIG_HOME/horse/config, else ~/.config/horse/config
func configPath() string {
	if p := os.Getenv("HORSE_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "horse", "config")
		}
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "horse", "config")
}

// loadConfig applies keybinding overrides from the config file. each line is
// `action = key, key, ...`; blanks and #comments are skipped, unknown
// actions/keys are ignored, and a missing file just leaves the defaults
func loadConfig() {
	path := configPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rhs, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if _, ok := actions[name]; !ok {
			continue // unknown action, skip
		}
		// the config replaces the default keys for this action
		unbindAction(name)
		for _, tok := range strings.Split(rhs, ",") {
			tok = strings.TrimSpace(tok)
			if key, r, isRune, ok := parseKey(tok); ok {
				if isRune {
					runeBindings[r] = name
				} else {
					keyBindings[key] = name
				}
			}
		}
	}
}

// unbindAction removes every key currently mapped to name
func unbindAction(name string) {
	for k, a := range keyBindings {
		if a == name {
			delete(keyBindings, k)
		}
	}
	for r, a := range runeBindings {
		if a == name {
			delete(runeBindings, r)
		}
	}
}

// parseKey turns a config token (ctrl+x, tab, enter, esc, backspace, space,
// up/down/left/right, or a single character) into a tcell key or a rune
func parseKey(s string) (key tcell.Key, r rune, isRune, ok bool) {
	switch strings.ToLower(s) {
	case "up":
		return tcell.KeyUp, 0, false, true
	case "down":
		return tcell.KeyDown, 0, false, true
	case "left":
		return tcell.KeyLeft, 0, false, true
	case "right":
		return tcell.KeyRight, 0, false, true
	case "tab":
		return tcell.KeyTab, 0, false, true
	case "enter", "return":
		return tcell.KeyEnter, 0, false, true
	case "esc", "escape":
		return tcell.KeyEscape, 0, false, true
	case "backspace":
		return tcell.KeyBackspace2, 0, false, true
	case "space":
		return 0, ' ', true, true
	}
	l := strings.ToLower(s)
	if len(l) == 6 && strings.HasPrefix(l, "ctrl+") && l[5] >= 'a' && l[5] <= 'z' {
		return tcell.KeyCtrlA + tcell.Key(l[5]-'a'), 0, false, true
	}
	if rs := []rune(s); len(rs) == 1 {
		return 0, rs[0], true, true
	}
	return 0, 0, false, false
}

// copySelectedPath puts the selected entry's full path on the system clipboard
func copySelectedPath() {
	if p := SelectedPath(); p != "" {
		screen.SetClipboard([]byte(p))
	}
}

// openSelected launches the os's default opener, or runs the file directly if it's executable
func openSelected() {
	name, ok := currentName()
	if !ok {
		return
	}
	fullPath := path.Join(Pwd, name)
	os.Chdir(Pwd)

	stat, err := os.Stat(fullPath)
	isExec := err == nil && stat.Mode()&0o111 != 0

	go func(p string) {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "linux":
			if isExec {
				cmd = exec.Command(p)
			} else {
				cmd = exec.Command("xdg-open", p)
			}
		case "darwin":
			if isExec {
				cmd = exec.Command(p)
			} else {
				cmd = exec.Command("open", p)
			}
		case "windows":
			// start handles both docs and .exe; the "" is start's title arg
			cmd = exec.Command("cmd", "/c", "start", "", p)
		default:
			screen.Fini()
			fmt.Println("dont actually know how to open a file on your OS, pls submit an issue")
			os.Exit(0)
		}
		_ = cmd.Run()
	}(fullPath)
}

// ask for y/n confirmation, then remove selected entry
func promptDelete() {
	name, ok := currentName()
	if !ok {
		return
	}
	fullPath := path.Join(Pwd, name)

	OpenPrompt("delete "+name+"? (y/n): ", "", 0, func(input string) {
		if strings.ToLower(input) == "y" {
			os.RemoveAll(fullPath)
			SwitchDir(Pwd)
		}
	})
}

// begin inline rename of selected entry
func startRename() {
	name, ok := currentName()
	if !ok {
		return
	}
	Edit = editRename
	EditOrig = name
	EditBuf.SetText(name)
}

// begins inline copy of selected entry to new destination
func startCopy() {
	name, ok := currentName()
	if !ok {
		return
	}
	Edit = editCopy
	EditOrig = name
	EditBuf.SetText(name)

	// make room for the edit line below the source
	vh := height - reservedRows
	if Selected-TopIndex >= vh-1 {
		TopIndex++
	}
}

// enter selection mode on first press;
// on the second press it runs a bash command against everything that's been marked
func handleMultiSelect() {
	if !Selecting {
		name, ok := currentName()
		if !ok {
			return
		}
		Selecting = true
		Sel = map[string]bool{name: true}
		LastMarked = name
		MoveCursor(1)
		return
	}

	names := selectedNames()
	if len(names) == 0 {
		Selecting = false
		Sel = nil
		return
	}

	// second C-x will jump back to last marked entry before asking
	// what 2 run, so you see what youre working with
	jumpTo(LastMarked)

	token := braceList(names)

	// prefill " %" and park cursor behind the space,
	// so typing replaces selection placeholder
	OpenPrompt("bash (%=sel): ", " % ", 0, func(cmd string) {
		if strings.TrimSpace(cmd) == "" {
			return
		}
		final := cmd
		if strings.Contains(final, "%") {
			final = strings.ReplaceAll(final, "%", token)
		} else {
			final = final + " " + token
		}
		c := exec.Command("bash", "-c", final)
		c.Dir = Pwd
		_ = c.Run()
		Selecting = false
		Sel = nil
	})
}

// ask for new file/dir name (trailing "/" makes a dir),
// creating missing parent directories along the way
func promptCreate() {
	OpenPrompt("create: ", "", 0, func(name string) {
		if name == "" {
			return
		}
		fullPath := path.Join(Pwd, name)
		lastDir := fullPath
		if strings.HasSuffix(name, "/") {
			os.MkdirAll(fullPath, 0o755)
		} else {
			dir := filepath.Dir(fullPath)
			os.MkdirAll(dir, 0o755)
			lastDir = dir
			if f, err := os.Create(fullPath); err == nil {
				f.Close()
			}
		}
		SwitchDir(lastDir)
	})
}

// exit multi-select mode, or quit horse entirely
func cancelOrQuit() {
	if Selecting {
		Selecting = false
		Sel = nil
		return
	}
	kittyClear()
	screen.Fini()
	os.Exit(0)
}

// mark current entry in selection mode, or open/enter it otherwise
func selectOrToggle() {
	if Selecting {
		toggleSelect()
		return
	}
	if Select() != "" {
		quitOnSelect()
	}
}

// jump to $HOME, or to / if we're already there
func toggleHome() {
	homeDir, err := os.UserHomeDir()
	targetDir := homeDir
	if err != nil || path.Clean(Pwd) == path.Clean(homeDir) {
		targetDir = path.Clean("/")
	}
	SwitchDir(path.Clean(targetDir))
}

// quit horse and ask shell to open $EDITOR on selected file
func quitOnSelect() {
	kittyClear()
	screen.Fini()
	selectedPath := Select()
	if selectedPath == "" {
		os.Exit(0)
	}
	fmt.Printf("$EDITOR %s\n", escapePath(selectedPath))
	os.Exit(0)
}

// quit horse and ask shell to cd into current directory or selection
func quitOnPwd() {
	kittyClear()
	screen.Fini()
	var p string
	if Input == "" {
		p = Pwd
	} else {
		items := CurrentList()
		if len(items) > 0 && Selected < len(items) {
			p = filepath.Join(Pwd, items[Selected])
		} else {
			p = Pwd
		}
	}
	fmt.Printf("cd %s\n", escapePath(p))
	os.Exit(0)
}

//////////////////
// line editors //
//////////////////

type LineEditor struct {
	buf    []rune // the text, as runes so multi-byte input stays intact
	cursor int    // insertion point as a rune index, 0 <= cursor <= len(buf)
}

// ret an editor holding text with the cursor at pos
// OOR pos clamps into [0, len(text)]
func NewLineEditor(text string, pos int) LineEditor {
	b := []rune(text)
	if pos < 0 {
		pos = 0
	}
	if pos > len(b) {
		pos = len(b)
	}
	return LineEditor{buf: b, cursor: pos}
}

// ret current buffer contents
func (e *LineEditor) Text() string {
	return string(e.buf)
}

// ret insertion point as a rune idx
func (e *LineEditor) Cursor() int {
	return e.cursor
}

// replace buffer and put cursor at the end
func (e *LineEditor) SetText(s string) {
	e.buf = []rune(s)
	e.cursor = len(e.buf)
}

// move insertion point, clamped into range
func (e *LineEditor) SetCursor(pos int) {
	e.cursor = max(0, min(pos, len(e.buf)))
}

// put r at cursor and step past it
func (e *LineEditor) Insert(r rune) {
	e.buf = append(e.buf[:e.cursor:e.cursor], append([]rune{r}, e.buf[e.cursor:]...)...)
	e.cursor++
}

// delete rune before cursor
func (e *LineEditor) Backspace() {
	if e.cursor > 0 {
		e.buf = append(e.buf[:e.cursor-1], e.buf[e.cursor:]...)
		e.cursor--
	}
}

// remove rune under cursor
func (e *LineEditor) Delete() {
	if e.cursor < len(e.buf) {
		e.buf = append(e.buf[:e.cursor], e.buf[e.cursor+1:]...)
	}
}

// step cursor back n runes
func (e *LineEditor) MoveLeft(n int) {
	e.cursor = max(0, e.cursor-n)
}

// step cursor forward n runes
func (e *LineEditor) MoveRight(n int) {
	e.cursor = min(len(e.buf), e.cursor+n)
}

// jump to the start of the line
func (e *LineEditor) Home() {
	e.cursor = 0
}

// jump to end of the line
func (e *LineEditor) End() {
	e.cursor = len(e.buf)
}

// delete from the cursor to the end of the line (C-k)
func (e *LineEditor) KillToEnd() {
	e.buf = e.buf[:e.cursor]
}

// clear the whole line (C-u)
func (e *LineEditor) KillAll() {
	e.buf = nil
	e.cursor = 0
}

func isEditorSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n'
}

// delete word before cursor, and any spaces between it and the cursor (like readline unix-word-rubout)
func (e *LineEditor) KillWord() {
	i := e.cursor
	for i > 0 && isEditorSpace(e.buf[i-1]) {
		i--
	}
	for i > 0 && !isEditorSpace(e.buf[i-1]) {
		i--
	}
	e.buf = append(e.buf[:i], e.buf[e.cursor:]...)
	e.cursor = i
}

// ret buffer contents before the insertion point
func (e *LineEditor) TextBeforeCursor() string {
	return string(e.buf[:e.cursor])
}

// feed one key event to the editor. report whether the
// caller should submit (Enter) or cancel (Escape/C-c);
// anything else is an edit the caller just needs to redraw for
func (e *LineEditor) HandleKey(ev *tcell.EventKey) (submit, cancel bool) {
	switch ev.Key() {
	case tcell.KeyEnter:
		return true, false
	case tcell.KeyEscape, tcell.KeyCtrlC:
		return false, true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		e.Backspace()
	case tcell.KeyDelete:
		e.Delete()
	case tcell.KeyLeft:
		e.MoveLeft(1)
	case tcell.KeyRight:
		e.MoveRight(1)
	case tcell.KeyHome:
		e.Home()
	case tcell.KeyEnd:
		e.End()
	case tcell.KeyCtrlA:
		e.Home()
	case tcell.KeyCtrlE:
		e.End()
	case tcell.KeyCtrlK:
		e.KillToEnd()
	case tcell.KeyCtrlU:
		e.KillAll()
	case tcell.KeyCtrlW:
		e.KillWord()
	case tcell.KeyRune:
		e.Insert(ev.Rune())
	}
	return false, false
}

/////////////
// prompts //
/////////////

type Prompt struct {
	IsActive bool
	Label    string
	Input    LineEditor
	OnSubmit func(string)
}

func OpenPrompt(label, initial string, cursor int, onSubmit func(string)) {
	ActivePrompt = Prompt{
		IsActive: true,
		Label:    label,
		Input:    NewLineEditor(initial, cursor),
		OnSubmit: onSubmit,
	}
}

func HandlePromptInput(ev *tcell.EventKey) {
	submit, cancel := ActivePrompt.Input.HandleKey(ev)
	switch {
	case submit:
		text := ActivePrompt.Input.Text()
		ActivePrompt.OnSubmit(text)
		ActivePrompt.IsActive = false
		SwitchDir(Pwd)
		screen.HideCursor()
	case cancel:
		ActivePrompt.IsActive = false
		Selecting = false
		screen.HideCursor()
		Redraw()
	}
}

/////////////////
// inline edit //
/////////////////

// rename (C-r) and copy (C-y) are both "edit a name inline, then apply it
// to the filesystem", so they share one state machine distinguished by Edit

// HandleEditInput feeds a key event to the active inline edit (rename or copy)
func HandleEditInput(ev *tcell.EventKey) {
	submit, cancel := EditBuf.HandleKey(ev)
	switch {
	case submit:
		commitEdit()
	case cancel:
		Edit = editNone
		screen.HideCursor()
	}
}

// apply active rename or copy and reselects result
// leave original untouched if target dir can't be created
// or underlying fs operation fails
func commitEdit() {
	kind := Edit
	Edit = editNone
	screen.HideCursor()

	name := EditBuf.Text()
	if name == "" || name == EditOrig {
		return
	}

	srcPath := path.Join(Pwd, EditOrig)
	dstPath := path.Join(Pwd, name)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil { // lets you move/copy by typing a/b/c
		return
	}

	var err error
	switch kind {
	case editRename:
		err = os.Rename(srcPath, dstPath)
	case editCopy:
		err = copyPath(srcPath, dstPath)
	}
	if err != nil {
		return
	}

	SwitchDir(Pwd)
	selectByName(filepath.Base(name))
}

// put cursor on entry called name in current dir, if present
func selectByName(name string) {
	for i, f := range Files {
		if f.Name() == name {
			Selected = i
			break
		}
	}
	visibleHeight := height - reservedRows
	if Selected >= visibleHeight {
		TopIndex = Selected - visibleHeight + 1
	}
}

///////////////
// selection //
///////////////

func toggleSelect() {
	list := CurrentList()
	if len(list) == 0 || Selected >= len(list) {
		return
	}
	if Sel == nil {
		Sel = map[string]bool{}
	}
	name := list[Selected]
	if Sel[name] {
		delete(Sel, name)
	} else {
		Sel[name] = true
		LastMarked = name
	}
	if len(Sel) == 0 {
		Selecting = false
		Sel = nil
		return
	}
	MoveCursor(1)
}

// ret marked names in listing order, ignoring any filter
func selectedNames() []string {
	var out []string
	for _, f := range Files {
		if Sel[f.Name()] {
			out = append(out, f.Name())
		}
	}
	return out
}

// braceList makes {a,b,c} like the readme wants, or just the name for one
func braceList(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return "{" + strings.Join(names, ",") + "}"
}

/////////////////
// fs & search //
/////////////////

// copies a file or a whole dir, keeping perms
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dst, info.Mode()); err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(path.Join(src, e.Name()), path.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(info.Mode())
}

func SelectedPath() string {
	name, ok := currentName()
	if !ok {
		return ""
	}
	return path.Join(Pwd, name)
}

func Select() string {
	var list []os.DirEntry
	if len(Results) > 0 {
		list = Results
	} else {
		list = Files
	}

	if len(list) == 0 {
		return ""
	}

	selected := list[Selected]

	if isDirEntry(path.Join(Pwd, selected.Name()), selected) {
		SwitchDir(path.Join(Pwd, selected.Name()))
		return ""
	}
	return path.Join(Pwd, selected.Name())
}

func SwitchDir(where string) error {
	if where == "" {
		return fmt.Errorf("cannot switch to empty directory")
	}

	cleanPath := path.Clean(where)
	if !path.IsAbs(cleanPath) {
		return fmt.Errorf("must provide an absolute path")
	}

	// remember where the cursor was in the dir we're leaving
	if LastSel == nil {
		LastSel = make(map[string]string)
	}
	if list := CurrentList(); len(list) > 0 && Selected < len(list) {
		LastSel[Pwd] = list[Selected]
	}

	// read first so a dir we cant open doesnt leave us in a broken state
	newPwd := cleanPath + "/"
	files, err := os.ReadDir(newPwd)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", newPwd, err)
	}

	// remember the dir we came from so ~ can jump back
	if Pwd != "" && Pwd != newPwd {
		PrevDir = Pwd
	}

	Pwd = newPwd
	Files = files
	Input = ""
	Selected = 0
	TopIndex = 0
	Results = nil
	invalidateList()

	// retain the last selection when coming back to this dir
	if name, ok := LastSel[Pwd]; ok {
		for i, f := range files {
			if f.Name() == name {
				Selected = i
				break
			}
		}
		visibleHeight := height - reservedRows
		if Selected >= visibleHeight {
			TopIndex = Selected - visibleHeight + 1
		}
	}
	return nil
}

// makes sure symlinks to directories work right
func isDirEntry(path string, entry os.DirEntry) bool {
	info, err := entry.Info()
	if err != nil {
		return false
	}

	if info.IsDir() {
		return true
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Stat(path)
		if err == nil && target.IsDir() {
			return true
		}
	}
	return false
}

///////////////////////////
// kitty grafix protocol //
///////////////////////////

// holds png bytes already resized for a given placement,
// so scrolling back to an image at the same size is free
type kittyCacheEntry struct {
	mtime int64
	size  int64
	png   []byte
}

var kittyCache = map[string]kittyCacheEntry{}

// previewImagePath returns the selected file if it's an image we can show, else ""
func previewImagePath() string {
	if (!kittyOK && !sixelOK) || !showPreview {
		return ""
	}
	files := Files
	if len(Results) > 0 {
		files = Results
	}
	if len(files) == 0 || Selected < 0 || Selected >= len(files) {
		return ""
	}
	entry := files[Selected]
	full := path.Join(Pwd, entry.Name())
	if isDirEntry(full, entry) {
		return ""
	}
	info, err := entry.Info()
	if err != nil || info.Size() == 0 || info.Size() > maxImagePreviewSize {
		return ""
	}
	f, err := os.Open(full)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if strings.HasPrefix(http.DetectContentType(buf[:n]), "image/") {
		return full
	}
	return ""
}

// reconcileKitty draws want in the preview pane, or clears it, only when it changes
func reconcileKitty(want string) {
	if !kittyOK || want == KittyShown {
		return
	}
	if KittyShown != "" {
		kittyClear()
	}
	KittyShown = ""
	if want == "" {
		return
	}

	data, err := os.ReadFile(want)
	if err != nil {
		return
	}
	// pull the pixel dims before resizing, to keep the aspect ratio
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return
	}

	cols := width - width/2 - 1
	rows := height - 1
	if cols < 1 || rows < 1 {
		return
	}
	// fit into the pane without stretching, then place at its top-left (1-based)
	c, r := fitCells(cfg.Width, cfg.Height, cols, rows)
	data, err = kittyPNG(want, data, cfg.Width, cfg.Height, c, r)
	if err != nil {
		return
	}

	kittyPlace(data, width/2+1, 1, c, r)
	KittyShown = want
}

// ret: png bytes of want resized to fit a c x r cell placement
// small pngs that already fit are passed through untouched
// anything larger or non-png is decoded, downscaled to pane pixels,
// and re-encoded, because rendering megabytes is really really slow
//
// results are cached by path + mtime + size + placement geometry
func kittyPNG(imgPath string, data []byte, imgW, imgH, cols, rows int) ([]byte, error) {
	cw, ch := cellSize()
	tw, th := cols*cw, rows*ch

	st, err := os.Stat(imgPath)
	if err != nil {
		return nil, err
	}
	key := strings.Join([]string{
		imgPath,
		fmt.Sprint(st.ModTime().UnixNano()),
		fmt.Sprint(st.Size()),
		fmt.Sprint(cols), fmt.Sprint(rows),
		fmt.Sprint(cw), fmt.Sprint(ch),
	}, "\x00")
	if e, ok := kittyCache[key]; ok && e.mtime == st.ModTime().UnixNano() && e.size == st.Size() {
		return e.png, nil
	}

	// happy path path:
	// a png that already fits the pane needs no pixel work
	if isPNG(data) && imgW <= tw && imgH <= th {
		return data, nil
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	imgW, imgH = b.Dx(), b.Dy()

	scale := math.Min(float64(tw)/float64(imgW), float64(th)/float64(imgH))
	if scale <= 0 {
		return nil, fmt.Errorf("bad placement size")
	}
	if scale > 1 {
		// the terminal scales up for zero moneys
		scale = 1
	}
	dw, dh := max(int(math.Round(float64(imgW)*scale)), 1), max(int(math.Round(float64(imgH)*scale)), 1)

	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	out := buf.Bytes()

	if len(kittyCache) > 32 {
		clear(kittyCache)
	}
	kittyCache[key] = kittyCacheEntry{mtime: st.ModTime().UnixNano(), size: st.Size(), png: out}
	return out, nil
}

func isPNG(data []byte) bool {
	return len(data) > 8 && string(data[1:4]) == "PNG"
}

// fitCells shrinks imgW x imgH (pixels) into at most cols x rows cells, keeping aspect
// without this function shit gets shrinked and looks so fucking funny lmao.
// the main limitation of this is that as it's going to be an approximation however
// hard you try and fit it, just by the nature of having columns and rows so
// ¯\_(ツ)_/¯
func fitCells(imgW, imgH, cols, rows int) (int, int) {
	if imgW < 1 || imgH < 1 {
		return cols, rows
	}
	cw, ch := cellSize()
	scale := math.Min(float64(cols*cw)/float64(imgW), float64(rows*ch)/float64(imgH))
	c := int(math.Round(float64(imgW) * scale / float64(cw)))
	r := int(math.Round(float64(imgH) * scale / float64(ch)))
	// clamp into [1, pane]
	c = min(max(c, 1), cols)
	r = min(max(r, 1), rows)
	return c, r
}

// cellSize asks the terminal for its cell size in pixels, falling back to a ~1:2 guess.
// the actual query is platform specific (see cellsize_unix.go / cellsize_other.go)
func cellSize() (int, int) {
	if ttyFile != nil {
		if cw, ch, ok := termCellSize(int(ttyFile.Fd())); ok {
			return cw, ch
		}
	}
	return 10, 20
}

// wraps a kitty graphics escape sequence for tmux's dcs passthrough
// tmux will just drop them otherwise because for no reason other than it hates us and mr. goyal
func wrapTmux(seq string) string {
	if !inTmux {
		return seq
	}
	escaped := strings.ReplaceAll(seq, "\x1b", "\x1b\x1b")
	return "\x1bPtmux;" + escaped + "\x1b\\"
}

func kittyClear() {
	if !kittyOK || ttyFile == nil {
		return
	} else {
		fmt.Fprint(ttyFile, wrapTmux(kgp.DeleteAllFree().Encode()))
	}
}

// kittyPlace transmits a png and displays it, scaled into cols x rows cells at (col,row)
func kittyPlace(png []byte, col, row, cols, rows int) {
	if ttyFile == nil {
		return
	}
	cmd := kgp.NewTransmitDisplay().
		Format(kgp.FormatPNG).
		TransmitDirect(png).
		DisplaySize(cols, rows).
		// if we let the terminal move it past the image itll render the image at 0, 0 sometimes
		CursorMovement(false).
		// otherwise it might pollute tcell
		ResponseSuppression(kgp.ResponseOKOnly).
		Build()

	// tmux chokes on very long passthrough payloads so use smaller chunks there
	// both satisfy kgp's EncodeChunked which is <=4096, divisible by 4
	chunk := 4096
	if inTmux {
		chunk = 1024
	}

	// this HAS to stay one buffered flush
	w := bufio.NewWriter(ttyFile)
	fmt.Fprintf(w, "\x1b[%d;%dH", row, col)
	for _, seq := range cmd.EncodeChunked(chunk) {
		w.WriteString(wrapTmux(seq))
	}
	w.Flush()
}

///////////////////
// sixel fallback //
///////////////////

// reconcileImage picks the graphics protocol: kitty if we have it, else sixel
func reconcileImage(want string) {
	switch {
	case kittyOK:
		reconcileKitty(want)
	case sixelOK:
		reconcileSixel(want)
	}
}

// reconcileSixel draws want in the preview pane, or clears it, only when it changes.
// unlike kitty theres no delete command: the pixels live in cells tcell doesnt track,
// so we force a full repaint to paint over the old one
func reconcileSixel(want string) {
	if want == SixelShown {
		return
	}
	if SixelShown != "" {
		screen.Sync() // repaint every cell so the old image gets covered
	}
	SixelShown = ""
	if want == "" {
		return
	}

	data, err := os.ReadFile(want)
	if err != nil {
		return
	}

	cols := width - width/2 - 1
	rows := height - 1
	if cols < 1 || rows < 1 {
		return
	}

	cw, ch := cellSize()
	img, err := scaleToFit(data, cols*cw, rows*ch)
	if err != nil {
		return
	}
	sixelPlace(img, width/2+1, 1)
	SixelShown = want
}

// scaleToFit decodes data and downscales it into tw x th pixels, keeping aspect.
// sixel needs real pixels; the terminal wont scale it for us like kitty does
func scaleToFit(data []byte, tw, th int) (image.Image, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	scale := math.Min(float64(tw)/float64(b.Dx()), float64(th)/float64(b.Dy()))
	if scale > 1 {
		scale = 1 // dont blow small images up
	}
	if scale <= 0 {
		return nil, fmt.Errorf("bad placement size")
	}
	dw := max(int(math.Round(float64(b.Dx())*scale)), 1)
	dh := max(int(math.Round(float64(b.Dy())*scale)), 1)
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst, nil
}

// sixelPlace encodes img to sixel and paints it at (col,row), 1-based
func sixelPlace(img image.Image, col, row int) {
	if ttyFile == nil {
		return
	}
	var buf bytes.Buffer
	if err := sixel.NewEncoder(&buf).Encode(img); err != nil {
		return
	}
	// one buffered flush, like kittyPlace
	w := bufio.NewWriter(ttyFile)
	fmt.Fprintf(w, "\x1b[%d;%dH", row, col)
	w.WriteString(wrapTmux(buf.String()))
	w.Flush()
}

// detectSixel asks the terminal (before tcell starts) whether it does sixel, via a
// primary device-attributes query (CSI c): attribute 4 in the reply means yes.
// HORSE_SIXEL=1/0 forces it on/off and skips the probe.
func detectSixel() bool {
	switch os.Getenv("HORSE_SIXEL") {
	case "1", "true":
		return true
	case "0", "false":
		return false
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer tty.Close()

	fd := int(tty.Fd())
	old, err := xterm.MakeRaw(fd)
	if err != nil {
		return false
	}
	defer xterm.Restore(fd, old)

	if _, err := tty.WriteString("\x1b[c"); err != nil {
		return false
	}

	// read the reply off-thread so a silent terminal cant hang us;
	// the deferred Close unblocks the Read when we bail on the timeout
	reply := make(chan string, 1)
	go func() {
		var acc []byte
		buf := make([]byte, 64)
		for {
			n, err := tty.Read(buf)
			acc = append(acc, buf[:n]...)
			if bytes.IndexByte(acc, 'c') >= 0 || err != nil {
				break
			}
		}
		reply <- string(acc)
	}()

	var resp string
	select {
	case resp = <-reply:
	case <-time.After(300 * time.Millisecond):
		return false
	}

	// reply is like ESC [ ? 62 ; 4 ; ... c  -- a 4 param means sixel
	for _, field := range strings.Split(resp, ";") {
		if strings.TrimRight(field, "c") == "4" {
			return true
		}
	}
	return false
}

///////////////
// rendering //
///////////////

func Redraw() {
	wantImg := previewImagePath()
	defer reconcileImage(wantImg) // emit after tcell has flushed, so it lands on top
	screen.Clear()

	files := Files
	if len(Results) > 0 {
		files = Results
	}

	if len(files) == 0 {
		DrawFiles()
		screen.Show()
		return
	}

	selectedEntry := files[Selected]
	fullPath := path.Join(Pwd, selectedEntry.Name())

	if showPreview && wantImg == "" {
		if isDirEntry(fullPath, selectedEntry) {
			DrawDirPreview(fullPath, width/2, 0, width-1, height-1)
		} else {
			info, err := selectedEntry.Info()
			drawWarning := func(text string) {
				drawText(width/2+2, 2, width-1, 2, STYLE_MID, text)
			}

			// dont do for 50kB+
			if err != nil || info.Size() > maxTextPreviewSize {
				DrawFiles()
				drawWarning("*file too large (or cant be opened)*")
				return
			}
			if info.Size() == 0 {
				DrawFiles()
				drawWarning("*file empty*")
				return
			}

			file, err := os.Open(fullPath)
			if err != nil {
				DrawFiles()
				drawWarning("*file cant be opened*")
				return
			}
			defer file.Close()

			// kinda hacky but works
			buffer := make([]byte, 512)
			n, _ := file.Read(buffer)
			file.Seek(0, 0)

			contentType := http.DetectContentType(buffer[:n])

			if strings.HasPrefix(contentType, "text/") || contentType == "application/javascript" || contentType == "application/json" {
				DrawFilePreview(file, width/2, 0, width-1, height-1)
			} else {
				drawWarning(contentType)
			}
		}
	}
	DrawFiles()
	screen.Show()
}

func DrawFilePreview(handle *os.File, x1, y1, x2, y2 int) {
	content, err := io.ReadAll(io.LimitReader(handle, previewReadLimit))
	if err != nil {
		return
	}

	lexer := lexers.Match(handle.Name())
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := HL_STYLE
	if style == nil {
		style = styles.Fallback
	}

	iterator, err := lexer.Tokenise(nil, string(content))
	if err != nil {
		return
	}

	x, y := x1, y1
	for _, token := range iterator.Tokens() {
		entry := style.Get(token.Type)

		tcellStyle := tcell.StyleDefault.
			// they map directly
			Foreground(tcell.NewRGBColor(int32(entry.Colour.Red()), int32(entry.Colour.Green()), int32(entry.Colour.Blue()))).
			Background(tcell.ColorReset)

		if entry.Bold == chroma.Yes {
			tcellStyle = tcellStyle.Bold(true)
		}

		// draw each token now
		for _, r := range token.Value {
			if r == '\n' {
				x = x1
				y++
				if y > y2 {
					return
				}
				continue
			}
			if x <= x2 {
				screen.SetContent(x, y, r, nil, tcellStyle)
				x++
			}
		}
	}
}

func DrawDirPreview(fullPath string, x1, y1, x2, y2 int) {
	dirEntries, err := os.ReadDir(fullPath)
	if err != nil {
		return
	}

	for y, entry := range dirEntries {
		isDir := isDirEntry(path.Join(fullPath, entry.Name()), entry)
		if isDir {
			drawText(x1, y, x2, y, STYLE_DIR, entry.Name()+"/")
		} else {
			drawText(x1, y, x2, y, STYLE_BG, entry.Name())
		}
		if y >= y2 {
			break
		}
	}
}

func DrawFiles() {
	filesToShow := CurrentList()

	pwdLen := len(Pwd) + 1
	drawText(1, 1, pwdLen, 1, STYLE_BG, Pwd)

	if len(filesToShow) > 0 && Selected < len(filesToShow) {
		drawText(pwdLen, 1, 999, 1, STYLE_MID, filesToShow[Selected])
	}

	drawText(pwdLen, 1, 999, 1, STYLE_BG, Input)

	// one slot, top-left above the input row: file position in normal
	// mode, selection count in selection mode
	if Selecting {
		selInfo := fmt.Sprintf("[%d/%d] selected", len(Sel), len(filesToShow))
		drawText(1, 0, 999, 0, STYLE_DIR, selInfo)
	} else {
		scrollInfo := fmt.Sprintf("[%d/%d]", Selected+1, len(filesToShow))
		drawText(1, 0, 999, 0, STYLE_MID, scrollInfo)
	}

	if len(filesToShow) == 0 {
		drawText(1, 2, 999, 3, STYLE_MID, "*nothing here*")
		screen.Show()
		return
	}

	Selected = min(Selected, len(filesToShow)-1)
	TopIndex = min(TopIndex, Selected)

	visibleHeight := height - reservedRows
	start := TopIndex
	end := min(start+visibleHeight, len(filesToShow))

	copyShift := 0
	// inline editors arre in the file list (left) panel; preview
	// pane starts at width/2, so clamp them short of it (TODO handle panel being toggled)
	editMaxX := max(width/2-1, 2)
	for i := start; i < end; i++ {
		y := i - start + 2 + copyShift
		name := filesToShow[i]

		// inline rename editor on the selected row
		if Edit == editRename && Selected == i {
			for x := 1; x <= editMaxX; x++ {
				screen.SetContent(x, y, ' ', nil, STYLE_BG)
			}
			drawLineEditor(1, y, editMaxX, STYLE_FG, &EditBuf)
			continue
		}

		style := STYLE_BG

		isDir := false
		if len(Results) > 0 {
			if i < len(Results) {
				fullPath := path.Join(Pwd, Results[i].Name())
				isDir = isDirEntry(fullPath, Results[i])
			}
		} else if i < len(Files) {
			fullPath := path.Join(Pwd, Files[i].Name())
			isDir = isDirEntry(fullPath, Files[i])
		}

		if Selected == i {
			style = STYLE_FG
		}
		if isDir {
			style = STYLE_DIR
			if Selected == i {
				style = STYLE_DIR_SEL
			}
			name += "/"
		}

		// mark selected files with a * in the left gutter
		if Selecting && Sel[filesToShow[i]] {
			screen.SetContent(0, y, '*', nil, STYLE_FG)
		}

		drawText(1, y, 999, y, style, name)

		// on copy, edit the destination path on a new line below the source
		if Edit == editCopy && Selected == i {
			ey := y + 1
			for x := 1; x <= editMaxX; x++ {
				screen.SetContent(x, ey, ' ', nil, STYLE_BG)
			}
			drawLineEditor(1, ey, editMaxX, STYLE_FG, &EditBuf)
			copyShift = 1
		}
	}

	if ActivePrompt.IsActive {
		input := ActivePrompt.Input.Text()
		label := ActivePrompt.Label

		for i := 0; i < width; i++ {
			screen.SetContent(i, 1, ' ', nil, STYLE_BG)
		}

		drawText(1, 1, len(label), 1, STYLE_FG, label)

		lastSlash := strings.LastIndex(input, "/")

		currentX := len(label) + 1

		if lastSlash != -1 {
			dirPart := input[:lastSlash+1]
			filePart := input[lastSlash+1:]

			styleDir := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorBlue)
			drawText(currentX, 1, currentX+len(dirPart), 1, styleDir, dirPart)

			if filePart != "" {
				drawText(currentX+len(dirPart), 1, width-1, 1, STYLE_BG, filePart)
			}
		} else {
			drawText(currentX, 1, width-1, 1, STYLE_BG, input)
		}

		screen.ShowCursor(len(label)+runewidth.StringWidth(ActivePrompt.Input.TextBeforeCursor())+1, 1)
	}
}

// render ed on row y from column x to maxX, scrolling
// horizontally so the cursor stays visible, and shows the cursor at the right cell
func drawLineEditor(x, y, maxX int, style tcell.Style, ed *LineEditor) {
	avail := maxX - x + 1
	if avail < 1 {
		return
	}
	// first visible rune! slide right until the cursor fits on screen
	start := 0
	for start < ed.cursor && runewidth.StringWidth(string(ed.buf[start:ed.cursor])) > avail-1 {
		start++
	}
	col := x
	for i := start; i < len(ed.buf) && col <= maxX; i++ {
		w := runewidth.RuneWidth(ed.buf[i])
		if col+w-1 > maxX {
			break
		}
		screen.SetContent(col, y, ed.buf[i], nil, style)
		col += w
	}
	screen.ShowCursor(x+runewidth.StringWidth(string(ed.buf[start:ed.cursor])), y)
}

func drawText(x1, y1, x2, y2 int, style tcell.Style, text string) {
	row := y1
	col := x1
	for _, r := range []rune(text) {
		screen.SetContent(col, row, r, nil, style)
		col++
		if col >= x2 {
			row++
			col = x1
		}
		if row > y2 {
			break
		}
	}
}

////////////////
// input & ux //
////////////////

func togglePrevDir() {
	if PrevDir == "" {
		return
	}
	SwitchDir(PrevDir)
}

func upDir() {
	splitPwd := strings.Split(strings.TrimSuffix(Pwd, "/"), "/")
	if len(splitPwd) > 1 {
		newPwd := strings.Join(splitPwd[:len(splitPwd)-1], "/")
		SwitchDir(fmt.Sprint("/", newPwd))
	}
}

func backspace(fullWord bool) {
	if len(Input) < 1 {
		upDir()
		return
	}

	modified := Input[:len(Input)-1]
	if fullWord {
		fields := strings.Fields(Input)
		if len(fields) > 0 {
			fields = fields[:len(fields)-1]
		}
		modified = strings.Join(fields, " ")
	}

	results := search(modified)
	Input = modified
	if len(results) == 0 {
		Results = nil
	} else {
		Results = results
	}
	invalidateList()
	Selected = 0
	TopIndex = 0
}

func doInput(r rune) {
	if len(Input) >= maxInputLength {
		return
	}

	modified := Input + string(r)
	results := search(modified)

	if len(results) == 0 {
		return
	}

	Input = modified
	Results = results
	invalidateList()
	Selected = 0
	TopIndex = 0
}

func search(query string) []os.DirEntry {
	if query == "" {
		return nil
	}

	var matches []os.DirEntry
	queryLower := strings.ToLower(query)

	for _, f := range Files {
		name := f.Name()
		nameLower := strings.ToLower(name)

		if strings.Contains(nameLower, queryLower) || fuzzy.MatchFold(query, name) {
			matches = append(matches, f)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		iName := strings.ToLower(matches[i].Name())
		jName := strings.ToLower(matches[j].Name())

		if iName == queryLower {
			return true
		}
		if jName == queryLower {
			return false
		}

		iHasPrefix := strings.HasPrefix(iName, queryLower)
		jHasPrefix := strings.HasPrefix(jName, queryLower)
		if iHasPrefix && !jHasPrefix {
			return true
		}
		if !iHasPrefix && jHasPrefix {
			return false
		}

		if len(iName) != len(jName) {
			return len(iName) < len(jName)
		}

		return iName < jName
	})

	return matches
}

// return visible entry names, so active search results, or
// the full directory listing when there's no search. cached until
// underlying Files/Results change (see invalidateList)
func CurrentList() []string {
	if listCache != nil {
		return listCache
	}
	src := Files
	if len(Results) > 0 {
		src = Results
	}
	names := make([]string, len(src))
	for i, f := range src {
		names[i] = f.Name()
	}
	listCache = names
	return names
}

// drops memoized CurrentList() result
// call after reassigning Files or Results
func invalidateList() {
	listCache = nil
}

// ret name of selected entry in active list, and
// whether one exists. shared guard against empty or out-of-range selection
func currentName() (string, bool) {
	list := CurrentList()
	if len(list) == 0 || Selected >= len(list) {
		return "", false
	}
	return list[Selected], true
}

func MoveCursor(n int) {
	list := CurrentList()
	if len(list) == 0 {
		return
	}

	Selected += n
	if Selected < 0 {
		Selected = len(list) - 1
	} else if Selected >= len(list) {
		Selected = 0
	}

	// keep the selection inside the visible window
	visibleHeight := height - reservedRows
	TopIndex = min(TopIndex, Selected)
	TopIndex = max(TopIndex, Selected-visibleHeight+1)
}

// put cursor on named entry and scroll it into view
// unknown names are ignored
func jumpTo(name string) {
	list := CurrentList()
	for i, n := range list {
		if n == name {
			Selected = i
			visibleHeight := height - reservedRows
			TopIndex = min(TopIndex, Selected)
			TopIndex = max(TopIndex, Selected-visibleHeight+1)
			return
		}
	}
}

/////////////
// helpers //
/////////////

func escapePath(p string) string {
	return "'" + strings.ReplaceAll(p, "'", "'\\''") + "'"
}
