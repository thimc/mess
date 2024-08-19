// Command mess implements a less(1)-like wrapper for the mblaze suite.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tcellterm "git.sr.ht/~rockorager/tcell-term"
	"github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v2/views"
)

var (
	// limit determines how many messages there will be in the overview (mscan output)
	// TODO(thimc): We are currently limited to using 5 because of the implementation of [scanfmt].
	limit = 5

	styleDefault = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	styleError   = styleDefault.Reverse(true)
	style        = styleDefault

	mshowArgs = os.Getenv("MSHOW_ARGS")
)

type point struct{ x, y int }

func drawString(s tcell.Screen, p *point, str string, ml bool) {
	w, h := s.Size()
	if p.y >= h {
		return
	}
	for _, r := range str {
		s.SetContent(p.x, p.y, r, nil, style)
		p.x++
		if r == '\n' || (ml && p.x >= w) {
			p.y++
			p.x = 0
		} else if !ml && p.x >= w {
			break
		}
	}
}

func runCmd(cmd string, args ...string) ([]string, error) {
	c := exec.Command(cmd, args...)
	c.Env = os.Environ()
	buf, err := c.Output()
	output := strings.TrimSuffix(string(buf), "\n")
	return strings.Split(output, "\n"), err
}

func cmdtoi(cmd string, args ...string) (int, error) {
	out, err := runCmd(cmd, args...)
	if err != nil {
		return -1, err
	}
	if len(out) < 1 || strings.Join(out, "\n") == "" {
		return -1, fmt.Errorf("%v: empty output", cmd)
	}
	n, err := strconv.Atoi(out[0])
	if err != nil {
		return -1, err
	}
	return n, nil
}

func scanfmt(dot, total int) string {
	// TODO(thimc): Calculate how the range should be defined rather
	// than using hard coded values.
	switch dot {
	case 1:
		return ".-0:.+5"
	case 2:
		return ".-1:.+4"
	case total - 2:
		return ".-3:.+2"
	case total - 1:
		return ".-4:.+1"
	case total:
		return ".-5:.+1"
	default:
		return ".-2:.+3"
	}
}

type UI struct {
	s         tcell.Screen
	v         *views.ViewPort
	p         *views.TextArea
	t         *tcellterm.VT
	dot       int
	mailcount int
	maillen   int
	curmail   []string
	poffset   int
	html      bool
	raw       bool
}

// NewUI creates a new user interface
func NewUI(style tcell.Style) (*UI, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.SetStyle(style)
	u := &UI{
		s: s,
		t: tcellterm.New(),
	}
	v := views.NewViewPort(u.s, 0, 5, -1, -1)

	u.v = views.NewViewPort(u.s, 0, 0, -1, 5)
	u.p = views.NewTextArea()
	u.p.SetView(u.v)

	u.t.SetSurface(u.v)
	u.t.SetSurface(v)
	u.t.Attach(func(ev tcell.Event) { u.s.PostEvent(ev) })
	u.s.EnableMouse()

	u.displaymail()

	return u, nil
}

func (u *UI) displaymail() error {

	pager := os.Getenv("PAGER")
	if pager == "" || pager == "less" {
		pager = "less -R"
	}
	cmd := exec.Command("mshow")
	cmd.Env = append(os.Environ(), "MBLAZE_PAGER="+pager)
	if err := u.t.Start(cmd); err != nil {
		return err
	}
	return nil
}

// Run runs the UI for one frame, it returns io.EOF when the user has
// requested the program to exit. Any other error is handled by
// rendering them on the screen.
func (u *UI) Run() {
	for {

		out, err := runCmd("mscan", []string{scanfmt(1, 10)}...)
		_ = err
		u.p.SetContent(strings.Join(out, "\n"))

		u.p.Draw()
		u.t.Draw()
		u.s.Show()

		ev := u.s.PollEvent()
		if ev == nil {
			break
		}
		u.update(ev)
	}
}

// Close destroys the user interface and quits the program.
func (u *UI) Close() {
	u.s.Suspend()
	if u.t != nil {
		u.t.Close()
	}
	u.s.Fini()
	os.Exit(0)
}

func (u *UI) refresh() error {
	mails, err := runCmd("mseq", "-f", ":")
	if err != nil {
		return err
	}
	c := exec.Command("mseq", "-S")
	c.Env = os.Environ()
	c.Stdin = strings.NewReader(strings.Join(mails, "\n"))
	return c.Run()
}

func (u *UI) update(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		u.s.Sync()
	case *tcellterm.EventRedraw:
		u.t.Draw()
	case *tcell.EventKey:
		switch {
		case ev.Rune() == '{':
			for i := u.poffset - 1; i != 0; i-- {
				if u.curmail[i] == "" {
					u.poffset = i
					break
				}
			}
		case ev.Rune() == '}':
			for i := u.poffset + 1; i != len(u.curmail)-1; i++ {
				if u.curmail[i] == "" {
					u.poffset = i
					break
				}
			}
		case ev.Rune() == '^':
			runCmd("mseq", "-C", ".^")
		case ev.Rune() == '0':
			u.dot = 1
			runCmd("mseq", "-C", fmt.Sprint(u.dot))
		case ev.Rune() == '$':
			u.dot = u.mailcount
			runCmd("mseq", "-C", fmt.Sprint(u.dot))
		case ev.Rune() == 'c':
			u.execCmd("mcom")
		case ev.Rune() == 'd':
			runCmd("mflag", "-S", ".")
			u.refresh()
			runCmd("mseq", "-C", "+")
		case ev.Rune() == 'f':
			u.execCmd("mfwd")
		case ev.Rune() == 'g', ev.Key() == tcell.KeyHome:
			u.poffset = 0
		case ev.Rune() == 'q':
			u.Close()
			return
		case ev.Rune() == 'r':
			u.execCmd("mrep")
			return
		case ev.Rune() == 'u':
			runCmd("mflag", "-s", ".")
			u.refresh()
			runCmd("mseq", "-C", "+")
		case ev.Rune() == 'D', ev.Key() == tcell.KeyDelete:
			var delete bool
		prompt:
			for {
				wmax, hmax := u.s.Size()
				for x := range wmax {
					u.s.SetContent(x, hmax-1, ' ', nil, style)
				}
				style = styleError
				drawString(u.s, &point{x: 0, y: hmax - 1}, "Delete the selected mail? (y/N)", false)
				style = styleDefault
				u.s.Show()
				switch e := u.s.PollEvent(); e := e.(type) {
				case *tcell.EventResize:
					continue
				case *tcell.EventKey:
					delete = e.Rune() == 'y' || e.Rune() == 'Y'
					break prompt
				}
			}
			if delete {
				curr, _ := runCmd("mseq", ".")
				defer os.Remove(curr[0])
				runCmd("mseq", "-C", "+")
				u.refresh()
			}
			return
		case ev.Rune() == 'G', ev.Key() == tcell.KeyEnd:
			max := len(u.curmail) - limit - 1
			if max < 0 {
				max = 0
			}
			u.poffset = max
		case ev.Rune() == 'H':
			u.html = !u.html
			return
		case ev.Rune() == 'J':
			u.poffset = 0
			runCmd("mseq", "-C", ".+1")
			u.displaymail()
			return
		case ev.Rune() == 'K':
			u.poffset = 0
			runCmd("mseq", "-C", ".-1")
			u.displaymail()
			return
		case ev.Rune() == 'N':
			unseen, _ := runCmd("magrep", "-v", "-m1", ":S", ".:")
			runCmd("mseq", "-C", unseen[0])
			return
		case ev.Rune() == 'R':
			u.raw = !u.raw
			return
		case ev.Rune() == 'T':
			mails, _ := runCmd("mseq", ".+1:")
			c := exec.Command("sed", "-n", "/^[^ <]/{p;q;}")
			c.Env = os.Environ()
			c.Stdin = strings.NewReader(strings.Join(mails, "\n"))
			buf, _ := c.Output()
			output := strings.TrimSuffix(string(buf), "\n")
			runCmd("mseq", "-C", output)
			return
		case ev.Key() == tcell.KeyCtrlD, ev.Key() == tcell.KeyPgDn:
			_, pg := u.s.Size()
			pg -= limit - 1
			max := len(u.curmail) - limit - 1
			if max < 0 {
				max = 0
			}
			if u.poffset+pg >= max {
				u.poffset = max
			} else {
				u.poffset += pg
			}
			return
		case ev.Key() == tcell.KeyCtrlU, ev.Key() == tcell.KeyPgUp:
			_, pg := u.s.Size()
			pg -= limit - 1
			if u.poffset-pg <= 0 {
				u.poffset = 0
			} else {
				u.poffset -= pg
			}
			return
		case ev.Key() == tcell.KeyCtrlL:
			u.s.Clear()
			return
		}
		if u.t != nil {
			u.t.HandleEvent(ev)
			u.t.Draw()
		}
	}
}

func (u *UI) execCmd(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := u.s.Suspend(); err != nil {
		return err
	}
	if err := c.Run(); err != nil {
		return err
	}
	if err := u.s.Resume(); err != nil {
		return err
	}
	return nil
}

func main() {
	ui, err := NewUI(styleDefault)
	if err != nil {
		panic(err)
	}
	defer ui.Close()
	for {
		ui.Run()
	}
}
