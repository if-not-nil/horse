// horse: https://github.com/if-not-nil/horse
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

var (
	width             = 20
	height            = 20
	STYLE_BG          = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	STYLE_DIR         = tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorBlue)
	STYLE_FG          = tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
	STYLE_MID         = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorGrey)
	draw_file_preview = false
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
	flag.BoolVar(&draw_file_preview, "preview", true, "show a file preview on the right side")
	flag.BoolVar(&draw_file_preview, "p", true, "alias for -preview")
	flag.Parse()

	s, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("%+v", err)
	}
	if err := s.Init(); err != nil {
		log.Fatalf("%+v", err)
	}
	width, height = s.Size()
	tmpFile, err := os.Create("/tmp/horselast")
	if err != nil {
		panic("couldnt create /tmp/horselast")
	}
	defer tmpFile.Close()

	s.SetStyle(STYLE_BG)

	s.Clear()

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
		if state.Input == "" {
			fmt.Println("cd", state.Pwd)
		} else {
			p := path.Join(state.Pwd, state.Files[state.Selected].Name())
			fmt.Println("cd", p)
		}
		os.Exit(0)
	}

	state.Redraw(s)

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
			state.Redraw(s)
		}

	}
}

//
// drawing code
//

func drawText(scr tcell.Screen, x1, y1, x2, y2 int, style tcell.Style, text string) {
	row := y1
	col := x1
	for _, r := range []rune(text) {
		scr.SetContent(col, row, r, nil, style)
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

func (state *State) Redraw(scr tcell.Screen) {
	scr.Clear()

	files := state.Files
	if len(state.Results) > 0 {
		files = state.Results
	}

	if len(files) == 0 {
		state.DrawFiles(scr)
		scr.Show()
		return
	}

	selected_entry := files[state.Selected]
	full_path := path.Join(state.Pwd, selected_entry.Name())

	if draw_file_preview {
		if selected_entry.IsDir() {
			DrawDirPreview(scr, full_path, width/2, 0, width-1, height-1)
		} else {
			info, err := selected_entry.Info()
			if err != nil || info.Size() > 50*1000 {
				state.DrawFiles(scr)
				return
			}
			file, err := os.Open(full_path)
			if err != nil {
				state.DrawFiles(scr)
				return
			}
			defer file.Close()
			DrawFilePreview(scr, file, width/2, 0, width-1, height-1)
		}
	}
	state.DrawFiles(scr)
	scr.Show()
}

func DrawFilePreview(scr tcell.Screen, handle *os.File, x1, y1, x2, y2 int) {
	// TODO: use "github.com/alecthomas/chroma/v2/quick" to highlight the preview file
	const maxPreviewSize = 100 * 1024 // 100 KB
	const maxPreviewLines = 1000

	limitedReader := io.LimitReader(handle, maxPreviewSize)
	scanner := bufio.NewScanner(limitedReader)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	y := y1
	lineCount := 0
	for scanner.Scan() && y <= y2 && lineCount < maxPreviewLines {
		drawText(scr, x1, y, x2, y, STYLE_BG, scanner.Text())
		y++
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		log.Printf("File preview error: %v", err)
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
			drawText(scr, x1, y, x2, y, STYLE_DIR, entry.Name()+"/")
		} else {
			drawText(scr, x1, y, x2, y, STYLE_BG, entry.Name())
		}
		if y >= y2 {
			break
		}
	}
}

func (state *State) DrawFiles(scr tcell.Screen) {
	filesToShow := state.CurrentList()

	pwdLen := len(state.Pwd) + 1
	drawText(scr, 1, 1, pwdLen, 1, STYLE_BG, state.Pwd)

	if len(filesToShow) > 0 && state.Selected < len(filesToShow) {
		drawText(scr, pwdLen, 1, 999, 1, STYLE_MID, filesToShow[state.Selected])
	}

	drawText(scr, pwdLen, 1, 999, 1, STYLE_BG, state.Input)

	scrollInfo := fmt.Sprintf("[%d/%d]", state.Selected+1, len(filesToShow))
	drawText(scr, width-len(scrollInfo)-2, 1, 999, 1, STYLE_MID, scrollInfo)

	if len(filesToShow) == 0 {
		drawText(scr, 1, 2, 999, 3, STYLE_MID, "*nothing here*")
		scr.Show()
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

		drawText(scr, 1, y, 999, y, style, name)
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
