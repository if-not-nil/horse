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

func main() {
	s, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("%+v", err)
	}
	if err := s.Init(); err != nil {
		log.Fatalf("%+v", err)
	}
	tmpFile, err := os.Create("/tmp/horselast")
	defer tmpFile.Close()

	defStyle := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	selStyle := tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
	sgStyle := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorGrey)
	s.SetStyle(defStyle)

	// Clear screen
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
		tmpFile.Write([]byte(path.Join(state.Pwd, state.Results[state.Selected])))
		os.Exit(0)
	}
	quit_on_pwd := func() {
		s.Fini()
		tmpFile.Write([]byte(state.Pwd))
		os.Exit(0)
	}

	redraw := func() {
		s.Clear()

		var filesToShow []string
		if len(state.Results) > 0 {
			filesToShow = state.Results
		} else {
			for _, f := range state.Files {
				filesToShow = append(filesToShow, f.Name())
			}
		}
		pwdLen := len(state.Pwd) + 1
		drawText(s, 1, 1, pwdLen, 1, defStyle, state.Pwd)
		if len(filesToShow) > 0 {
			drawText(s, pwdLen, 1, 999, 1, sgStyle, filesToShow[state.Selected])
		}
		drawText(s, pwdLen, 1, 999, 1, defStyle, state.Input)

		for i, name := range filesToShow {
			if state.Selected == i {
				drawText(s, 1, i+2, 999, i+2, selStyle, name)
			} else {
				drawText(s, 1, i+2, 999, i+2, defStyle, name)
			}
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
				os.Exit(0)
			case tcell.KeyDown, tcell.KeyCtrlJ:
				state.MoveCursor(1)
			case tcell.KeyUp, tcell.KeyCtrlK:
				state.MoveCursor(-1)
			case tcell.KeyTab:
				shouldQuit := state.Select()
				if shouldQuit != "" {
					quit_on_sel()
				}
			case tcell.KeyEnter:
				quit_on_pwd()
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				state.backspace()
			case tcell.KeyRune:
				state.doInput(ev.Rune())
			}
			redraw()
		}

	}
}

func (s *State) backspace() {
	if len(s.Input) < 1 {
		splitPwd := strings.Split(s.Pwd, "/")

		newPwd := splitPwd[:len(splitPwd)-2]
		newPath := path.Join(newPwd...)
		s.SwitchDir(fmt.Sprint("/", newPath))
		return
	}
	modified := s.Input[:len(s.Input)-1]
	results := s.search(modified)
	if len(results) == 0 {
		return
	} else {
		s.Input = modified
		s.Results = results
	}
}

func (s *State) doInput(what rune) {
	modified := fmt.Sprint(s.Input, string(what))
	results := s.search(modified)
	if len(results) == 0 {
		return
	} else {
		s.Input = modified
		s.Results = results
	}
}

func (s *State) search(query string) []string {
	var mapped []string
	for _, f := range s.Files {
		mapped = append(mapped, f.Name())
	}
	res := fuzzy.Find(query, mapped)
	// log.Println(res)
	return res
}

type State struct {
	Pwd      string
	Input    string
	Files    []os.DirEntry
	Results  []string
	Selected int
}

func (s *State) MoveCursor(n int) {
	if s.Selected+n > len(s.Files)-1 {
		s.Selected = 0
	} else if s.Selected+n < 0 {
		s.Selected = len(s.Files) - 1
	} else {
		s.Selected += n
	}
}

func (s *State) Select() string {
	var selectedName string
	if len(s.Results) > 0 {
		selectedName = s.Results[s.Selected]
	} else {
		selectedName = s.Files[s.Selected].Name()
	}

	for _, f := range s.Files {
		if f.Name() == selectedName {
			if f.IsDir() {
				s.SwitchDir(path.Join(s.Pwd, f.Name()))
				break
			} else {
				return path.Join(s.Pwd, f.Name())
			}
		}
	}
	return ""
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
