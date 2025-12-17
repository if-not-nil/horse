// horse: https://github.com/if-not-nil/horse
package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

var (
	width  = 20
	height = 20
)

type State struct {
	Pwd      string
	Input    string
	Files    []os.DirEntry
	Results  []os.DirEntry
	Selected int
	TopIndex int
}

func main() {
	s, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("%+v", err)
	}
	if err := s.Init(); err != nil {
		log.Fatalf("%+v", err)
	}
	tmpFile, err := os.Create("/tmp/horselast")
	if err != nil {
		panic("couldnt create /tmp/horselast")
	}
	defer tmpFile.Close()

	defStyle := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	selStyle := tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
	sgStyle := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorGrey)
	s.SetStyle(defStyle)

	s.Clear()

	width, height = s.Size()

	var state State
	a, err := os.Getwd()
	if err != nil {
		log.Fatal(err, "getpwd")
	}
	state.SwitchDir(a)

	quit_on_sel := func() {
		s.Fini()

		selectedPath := state.Select()
		if selectedPath == "" {
			os.Exit(0)
		}
		fmt.Println("$EDITOR", selectedPath)
		os.Exit(0)
	}

	quit_on_pwd := func() {
		s.Fini()
		fmt.Println("cd", state.Pwd)
		os.Exit(0)
	}

	redraw := func() {
		s.Clear()

		filesToShow := state.CurrentList()

		pwdLen := len(state.Pwd) + 1
		drawText(s, 1, 1, pwdLen, 1, defStyle, state.Pwd)

		if len(filesToShow) > 0 && state.Selected < len(filesToShow) {
			drawText(s, pwdLen, 1, 999, 1, sgStyle, filesToShow[state.Selected])
		}

		drawText(s, pwdLen, 1, 999, 1, defStyle, state.Input)

		scrollInfo := fmt.Sprintf("[%d/%d]", state.Selected+1, len(filesToShow))
		drawText(s, width-len(scrollInfo)-2, 1, 999, 1, sgStyle, scrollInfo)

		if len(filesToShow) == 0 {
			drawText(s, 1, 3, 999, 3, sgStyle, "(empty)")
			s.Show()
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

			style := defStyle

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
				style = selStyle
			}
			if isDir {
				name += "/"
				if state.Selected == i {
					style = style.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue)
				} else {
					style = style.Foreground(tcell.ColorBlue).Background(tcell.ColorBlack)
				}
			}

			drawText(s, 1, y, 999, y, style, name)
		}
	}
	redraw()

	for {
		s.Show()

		ev := s.PollEvent()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			s.Sync()
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape, tcell.KeyCtrlC:
				s.Fini()
				os.Exit(1)
			// scrolling
			case tcell.KeyDown, tcell.KeyCtrlJ, tcell.KeyCtrlN:
				state.MoveCursor(1)
			case tcell.KeyUp, tcell.KeyCtrlK, tcell.KeyCtrlP:
				state.MoveCursor(-1)
			case tcell.KeyTab:
				shouldQuit := state.Select()
				if shouldQuit != "" {
					quit_on_sel()
				}
			case tcell.KeyEnter:
				quit_on_pwd()
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				state.backspace(false)
			case tcell.KeyCtrlW:
				state.backspace(true)
			case tcell.KeyCtrlE:
				home_dir, err := os.UserHomeDir()
				path.Clean(home_dir)

				if err != nil {
					home_dir = path.Clean("/")
				}
				state.SwitchDir(home_dir)
			case tcell.KeyRune:
				state.doInput(ev.Rune())
			}
			redraw()
		}

	}
}

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
	modified := fmt.Sprint(s.Input, string(what))
	results := s.search(modified)
	if len(results) == 0 {
		length := len(s.Input)
		s.Input = s.Results[0].Name()[:length]
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

	var exact, prefix, fuzzyMatches []os.DirEntry

	// prefix and exact matches first
	for _, f := range s.Files {
		name := f.Name()
		switch {
		case name == query:
			exact = append(exact, f)
		case strings.HasPrefix(name, query):
			prefix = append(prefix, f)
		default:
			fuzzyMatches = append(fuzzyMatches, f)
		}
	}

	// fuzzy matches on the rest
	names := make([]string, len(fuzzyMatches))
	for i, f := range fuzzyMatches {
		names[i] = f.Name()
	}

	matchedNames := fuzzy.FindFold(query, names)
	var fuzzyRanked []os.DirEntry
	for _, name := range matchedNames {
		for _, f := range fuzzyMatches {
			if f.Name() == name {
				fuzzyRanked = append(fuzzyRanked, f)
				break
			}
		}
	}

	return append(append(exact, prefix...), fuzzyRanked...)
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

func (s *State) Select() string {
	var selected os.DirEntry
	if len(s.Results) > 0 {
		selected = s.Results[s.Selected]
	} else {
		selected = s.Files[s.Selected]
	}

	if isDirEntry(path.Join(s.Pwd, selected.Name()), selected) {
		s.SwitchDir(path.Join(s.Pwd, selected.Name()))
		return ""
	}
	return path.Join(s.Pwd, selected.Name())
}

func (s *State) SwitchDir(where string) {
	if path.IsAbs(where) {
		s.Pwd = fmt.Sprint(path.Clean(where), "/")
	} else {
		log.Fatalf("called switch with a relative path")
	}
	s.Input = ""

	files, err := os.ReadDir(s.Pwd)
	if err != nil {
		log.Fatalln(err, "listdir")
	}
	s.Files = files

	s.Selected = 0
	s.Results = nil
}

func drawText(s tcell.Screen, x1, y1, x2, y2 int, style tcell.Style, text string) {
	row := y1
	col := x1
	for _, r := range []rune(text) {
		s.SetContent(col, row, r, nil, style)
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
