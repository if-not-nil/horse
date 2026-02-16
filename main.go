// horse: https://github.com/if-not-nil/horse
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http" // todo: another library for filetypes
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	width                           = 20
	height                          = 20
	STYLE_BG                        = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	STYLE_DIR                       = tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorBlue)
	STYLE_FG                        = tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
	STYLE_MID                       = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorGrey)
	draw_file_preview               = false
	HL_STYLE          *chroma.Style = styles.Get("monokai")
	screen            tcell.Screen
)

type State struct {
	Pwd      string
	Input    string
	Files    []os.DirEntry
	Results  []os.DirEntry
	Selected int
	TopIndex int

	ActivePrompt Prompt
}

type Prompt struct {
	IsActive bool
	Label    string
	Input    string
	OnSubmit func(string)
}

func (s *State) OpenPrompt(label string, onSubmit func(string)) {
	s.ActivePrompt = Prompt{
		IsActive: true,
		Label:    label,
		Input:    "",
		OnSubmit: onSubmit,
	}
}

func (s *State) HandlePromptInput(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEnter:
		s.ActivePrompt.OnSubmit(s.ActivePrompt.Input)
		s.ActivePrompt.IsActive = false
		s.SwitchDir(s.Pwd)
		screen.HideCursor()
	case tcell.KeyEscape:
		s.ActivePrompt.IsActive = false
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(s.ActivePrompt.Input) > 0 {
			s.ActivePrompt.Input = s.ActivePrompt.Input[:len(s.ActivePrompt.Input)-1]
		}
	case tcell.KeyRune:
		s.ActivePrompt.Input += string(ev.Rune())
	}
}

func main() {
	flag.BoolVar(&draw_file_preview, "preview", true, "show a file preview on the right side")
	flag.BoolVar(&draw_file_preview, "p", true, "alias for -preview")
	flag.Parse()

	s, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("%+v", err)
	}
	screen = s
	if err := screen.Init(); err != nil {
		log.Fatalf("%+v", err)
	}
	width, height = screen.Size()
	tmpFile, err := os.Create("/tmp/horselast")
	if err != nil {
		panic("couldnt create /tmp/horselast")
	}
	defer tmpFile.Close()

	screen.SetStyle(STYLE_BG)

	screen.Clear()

	var state State
	a, err := os.Getwd()
	if err != nil {
		log.Fatal(err, "getpwd")
	}
	state.SwitchDir(a)

	quit_on_sel := func() {
		screen.Fini()

		selectedPath := state.Select()
		if selectedPath == "" {
			os.Exit(0)
		}
		fmt.Println("$EDITOR", selectedPath)
		os.Exit(0)
	}

	quit_on_pwd := func() {
		screen.Fini()
		if state.Input == "" {
			fmt.Println("cd", state.Pwd)
		} else {
			p := path.Join(state.Pwd, state.Files[state.Selected].Name())
			fmt.Println("cd", p)
		}
		os.Exit(0)
	}

	state.Redraw()

	for {
		screen.Show()

		ev := screen.PollEvent()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventKey:

			if state.ActivePrompt.IsActive {
				state.HandlePromptInput(ev)
				state.Redraw()
				continue
			}
			switch ev.Key() {

			case tcell.KeyCtrlO:
				var selectedEntry os.DirEntry
				if len(state.Results) > 0 {
					selectedEntry = state.Results[state.Selected]
				} else if len(state.Files) > 0 {
					selectedEntry = state.Files[state.Selected]
				} else {
					continue
				}

				full_path := path.Join(state.Pwd, selectedEntry.Name())

				go func(p string) {
					var cmd *exec.Cmd
					switch runtime.GOOS {
					case "linux":
						cmd = exec.Command("xdg-open", p)
					case "darwin":
						cmd = exec.Command("open", p)
					default:
						screen.Fini()
						fmt.Println("dont actually know how to open a file on your OS, pls submit an issue")
						os.Exit(0)
					}
					_ = cmd.Run()
				}(full_path)

			case tcell.KeyCtrlD:
				selected := state.Files[state.Selected].Name()
				fullPath := path.Join(state.Pwd, selected)

				state.OpenPrompt("delete "+selected+"? (y/n): ", func(input string) {
					if strings.ToLower(input) == "y" {
						os.RemoveAll(fullPath)
						state.SwitchDir(state.Pwd)
					}
				})

			case tcell.KeyCtrlA:
				state.OpenPrompt("create: ", func(name string) {
					if name == "" {
						return
					}

					fullPath := path.Join(state.Pwd, name)

					last_dir := fullPath
					if strings.HasSuffix(name, "/") {
						os.MkdirAll(fullPath, 0o755)
					} else {
						dir := filepath.Dir(fullPath)

						os.MkdirAll(dir, 0o755)
						last_dir = dir

						f, err := os.Create(fullPath)
						if err == nil {
							f.Close()
						}
					}
					state.SwitchDir(last_dir)
				})
			case tcell.KeyEscape, tcell.KeyCtrlC:
				screen.Fini()
				os.Exit(0)
			// scrolling
			case tcell.KeyDown, tcell.KeyCtrlJ, tcell.KeyCtrlN:
				state.MoveCursor(1)
			case tcell.KeyUp, tcell.KeyCtrlK, tcell.KeyCtrlP:
				state.MoveCursor(-1)
			case tcell.KeyTab, tcell.KeyCtrlL, tcell.KeyCtrlF:
				shouldQuit := state.Select()
				if shouldQuit != "" {
					quit_on_sel()
				}
			case tcell.KeyEnter:
				quit_on_pwd()
			case tcell.KeyBackspace, tcell.KeyBackspace2, tcell.KeyCtrlB:
				state.backspace(false)
			case tcell.KeyCtrlW:
				state.backspace(true)
			case tcell.KeyCtrlE:
				home_dir, err := os.UserHomeDir()
				target_dir := home_dir
				if err != nil || path.Clean(state.Pwd) == path.Clean(home_dir) {
					target_dir = path.Clean("/")
				}
				target_dir = path.Clean(target_dir)

				state.SwitchDir(target_dir)
			case tcell.KeyRune:
				state.doInput(ev.Rune())
			}
			state.Redraw()
		}

	}
}

//
// drawing code
//

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

func (state *State) Redraw() {
	screen.Clear()

	files := state.Files
	if len(state.Results) > 0 {
		files = state.Results
	}

	if len(files) == 0 {
		state.DrawFiles()
		screen.Show()
		return
	}

	selected_entry := files[state.Selected]
	full_path := path.Join(state.Pwd, selected_entry.Name())

	if draw_file_preview {
		if !selected_entry.IsDir() {
			info, err := selected_entry.Info()
			draw_warning := func(text string) {
				drawText(width/2+2, 2, width-1, 2, STYLE_MID, text)
			}

			// dont do for 50kB+
			if err != nil || info.Size() > 50*1000 {
				state.DrawFiles()
				draw_warning("*file too large (or cant be opened)*")
				return
			}
			if info.Size() == 0 {
				state.DrawFiles()
				draw_warning("*file empty*")
				return
			}

			file, err := os.Open(full_path)
			if err != nil {
				state.DrawFiles()
				draw_warning("*file cant be opened*")
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
				draw_warning(contentType)
			}
		}
	}
	state.DrawFiles()
	screen.Show()
}

func DrawFilePreview(handle *os.File, x1, y1, x2, y2 int) {
	content, err := io.ReadAll(io.LimitReader(handle, 10000)) // 10KB limit is fast
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

func DrawDirPreview(scr tcell.Screen, full_path string, x1, y1, x2, y2 int) {
	dir_entries, err := os.ReadDir(full_path)
	if err != nil {
		return
	}

	for y, entry := range dir_entries {
		isDir := isDirEntry(path.Join(full_path, entry.Name()), entry)
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

func (state *State) DrawFiles() {
	filesToShow := state.CurrentList()

	pwdLen := len(state.Pwd) + 1
	drawText(1, 1, pwdLen, 1, STYLE_BG, state.Pwd)

	if len(filesToShow) > 0 && state.Selected < len(filesToShow) {
		drawText(pwdLen, 1, 999, 1, STYLE_MID, filesToShow[state.Selected])
	}

	drawText(pwdLen, 1, 999, 1, STYLE_BG, state.Input)

	scrollInfo := fmt.Sprintf("[%d/%d]", state.Selected+1, len(filesToShow))
	drawText(width-len(scrollInfo)-2, 1, 999, 1, STYLE_MID, scrollInfo)

	if len(filesToShow) == 0 {
		drawText(1, 2, 999, 3, STYLE_MID, "*nothing here*")
		screen.Show()
		return
	}

	if state.Selected >= len(filesToShow) {
		state.Selected = len(filesToShow) - 1
	}
	if state.TopIndex > state.Selected {
		state.TopIndex = state.Selected
	}

	visibleHeight := height - 3
	start := state.TopIndex
	end := min(start+visibleHeight, len(filesToShow))

	for i := start; i < end; i++ {
		y := i - start + 2
		name := filesToShow[i]

		style := STYLE_BG

		isDir := false
		if len(state.Results) > 0 {
			if i < len(state.Results) {
				fullPath := path.Join(state.Pwd, state.Results[i].Name())
				isDir = isDirEntry(fullPath, state.Results[i])
			}
		} else if i < len(state.Files) {
			fullPath := path.Join(state.Pwd, state.Files[i].Name())
			isDir = isDirEntry(fullPath, state.Files[i])
		}

		if state.Selected == i {
			style = STYLE_FG
		}
		if isDir {
			style = style.Foreground(tcell.ColorBlue).Background(tcell.ColorBlack)
			name += "/"
			if state.Selected == i {
				style = invertStyle(style)
			}
		}

		drawText(1, y, 999, y, style, name)
	}

	if state.ActivePrompt.IsActive {
		input := state.ActivePrompt.Input
		label := state.ActivePrompt.Label

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
		screen.ShowCursor(len(label)+len(input)+1, 1)
	}
}

//
// input & ux
//

func (s *State) backspace(full_word bool) {
	if len(s.Input) < 1 {
		splitPwd := strings.Split(strings.TrimSuffix(s.Pwd, "/"), "/")
		if len(splitPwd) > 1 {
			newPwd := strings.Join(splitPwd[:len(splitPwd)-1], "/")
			s.SwitchDir(fmt.Sprint("/", newPwd))
		}
		return
	}

	modified := ""
	if !full_word {
		modified = s.Input[:len(s.Input)-1]
	}

	results := s.search(modified)
	if len(results) == 0 {
		s.Input = modified
		s.Results = nil
		s.Selected = 0
		s.TopIndex = 0
		return
	}
	s.Input = modified
	s.Results = results
	s.Selected = 0
	s.TopIndex = 0
}

func (s *State) doInput(what rune) {
	const maxInputLength = 100

	if len(s.Input) >= maxInputLength {
		return
	}

	modified := s.Input + string(what)
	results := s.search(modified)

	if len(results) == 0 {
		return
	}

	s.Input = modified
	s.Results = results
	s.Selected = 0
	s.TopIndex = 0
}

func (s *State) search(query string) []os.DirEntry {
	if query == "" {
		return nil
	}

	var matches []os.DirEntry
	queryLower := strings.ToLower(query)

	for _, f := range s.Files {
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

func (s *State) CurrentList() []string {
	if len(s.Results) > 0 {
		names := make([]string, len(s.Results))
		for i, f := range s.Results {
			names[i] = f.Name()
		}
		return names
	}
	names := make([]string, len(s.Files))
	for i, f := range s.Files {
		names[i] = f.Name()
	}
	return names
}

func (s *State) MoveCursor(n int) {
	list := s.CurrentList()
	if len(list) == 0 {
		return
	}

	s.Selected += n
	if s.Selected < 0 {
		s.Selected = len(list) - 1
	} else if s.Selected >= len(list) {
		s.Selected = 0
	}

	visibleHeight := height - 3
	if s.Selected < s.TopIndex {
		s.TopIndex = s.Selected
	} else if s.Selected >= s.TopIndex+visibleHeight {
		s.TopIndex = s.Selected - visibleHeight + 1
	}
}

//
// fs & search
//

func (s *State) Select() string {
	var list []os.DirEntry
	if len(s.Results) > 0 {
		list = s.Results
	} else {
		list = s.Files
	}

	if len(list) == 0 {
		return ""
	}

	selected := list[s.Selected]

	if isDirEntry(path.Join(s.Pwd, selected.Name()), selected) {
		s.SwitchDir(path.Join(s.Pwd, selected.Name()))
		return ""
	}
	return path.Join(s.Pwd, selected.Name())
}

func (s *State) SwitchDir(where string) error {
	if where == "" {
		return fmt.Errorf("cannot switch to empty directory")
	}

	cleanPath := path.Clean(where)
	if !path.IsAbs(cleanPath) {
		return fmt.Errorf("must provide an absolute path")
	}

	s.Pwd = cleanPath + "/"

	s.Input = ""
	s.Selected = 0
	s.Results = nil

	files, err := os.ReadDir(s.Pwd)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", s.Pwd, err)
	}

	s.Files = files
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

//
// helpers
//

func invertStyle(st tcell.Style) tcell.Style {
	fg, bg, _ := st.Decompose()
	return st.Foreground(bg).Background(fg)
}
