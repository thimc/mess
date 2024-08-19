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

// total returns the amount of mails in the current sequence
func total() int {
	total, err := cmdtoi("mscan", "-n", "--", "-1")
	_ = err
	return total + 1
}

// scanfmt determines how the mscan format should be printed.
//
// TODO(thimc): Calculate how the range should be defined rather
// than using hard coded values.
func scanfmt(dot int) string {
	switch dot {
	case 1:
		return ".-0:.+5"
	case 2:
		return ".-1:.+4"
	case total() - 2:
		return ".-3:.+2"
	case total() - 1:
		return ".-4:.+1"
	case total():
		return ".-5:.+1"
	default:
		return ".-2:.+3"
	}
}

type UI struct {
	s    tcell.Screen
	v    *views.ViewPort // mscan
	tv   *views.ViewPort // terminal
	p    *views.TextArea
	t    *tcellterm.VT
	dot  int
	html bool
	raw  bool
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
	u := &UI{s: s}
	u.s.EnableMouse()
	u.v = views.NewViewPort(u.s, 0, 0, -1, 5)
	u.p = views.NewTextArea()
	u.p.SetView(u.v)

	u.initterm()
	u.displaymail()

	return u, nil
}

// initterm initializes the virtual terminal
func (u *UI) initterm() {
	u.t = tcellterm.New()
	u.tv = views.NewViewPort(u.s, 0, 5, -1, -1)
	u.t.SetSurface(u.tv)
	u.t.Attach(func(ev tcell.Event) { u.s.PostEvent(ev) })
}

// displaymail prints the current mail in a virtual terminal, launched
// with `$MBLAZE_PAGER`
func (u *UI) displaymail() error {
	pager := os.Getenv("PAGER")
	if pager == "" || pager == "less" {
		pager = "less -R"
	}
	var args = strings.Split(mshowArgs, " ")
	if u.raw {
		args = append(args, "-H", "-r")
		// TODO(thimc): the -r flag causes mshow to print its output
		// directly to standard output and not to less / MBLAZE_PAGER
		// so we have to pipe the output to less manually.
	} else if u.html {
		args = append(args, "-A", "text/html")
	}
	args = append(args, ".")
	cmd := exec.Command("mshow", args...)
	cmd.Env = append(os.Environ(), "MBLAZE_PAGER="+pager)
	if u.t != nil {
		u.t.Close()
	}
	u.initterm()
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

		dot, err := cmdtoi("mscan", "-n", ".")
		_ = err
		out, err := runCmd("mscan", []string{scanfmt(dot)}...)
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

// Exit destroys the user interface and quits the program.
func (u *UI) Exit() {
	if u.t != nil {
		u.t.Close()
	}
	u.s.Fini()
	os.Exit(0)
}

// refresh refreshes the mailbox sequence
func (u *UI) refresh() error {
	mails, err := runCmd("mseq", "-f", ":")
	if err != nil {
		return err
	}
	cmd := exec.Command("mseq", "-S")
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(strings.Join(mails, "\n"))
	return cmd.Run()
}

// update handles the user input;
func (u *UI) update(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		u.s.Sync()
	case *tcellterm.EventRedraw:
		u.t.Draw()
	case *tcell.EventKey:
		switch {
		case ev.Rune() == '^':
			runCmd("mseq", "-C", ".^")
		case ev.Rune() == '0':
			_, _ = runCmd("mseq", "-C", "1")
			u.displaymail()
		case ev.Rune() == '$':
			tot := total()
			_, _ = runCmd("mseq", "-C", fmt.Sprint(tot))
			u.displaymail()
		case ev.Rune() == 'c':
			u.execCmd("mcom")
		case ev.Rune() == 'd':
			runCmd("mflag", "-S", ".")
			u.refresh()
			runCmd("mseq", "-C", "+")
		case ev.Rune() == 'f':
			u.execCmd("mfwd")
		case ev.Rune() == 'q':
			u.Exit()
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
		case ev.Rune() == 'H':
			u.html = !u.html
			u.displaymail()
			return
		case ev.Rune() == 'J':
			runCmd("mseq", "-C", ".+1")
			u.displaymail()
			return
		case ev.Rune() == 'K':
			runCmd("mseq", "-C", ".-1")
			u.displaymail()
			return
		case ev.Rune() == 'N':
			unseen, _ := runCmd("magrep", "-v", "-m1", ":S", ".:")
			runCmd("mseq", "-C", unseen[0])
			u.displaymail()
			return
		case ev.Rune() == 'R':
			u.raw = !u.raw
			u.displaymail()
			return
		case ev.Rune() == 'T':
			mails, _ := runCmd("mseq", ".+1:")
			c := exec.Command("sed", "-n", "/^[^ <]/{p;q;}")
			c.Env = os.Environ()
			c.Stdin = strings.NewReader(strings.Join(mails, "\n"))
			buf, _ := c.Output()
			output := strings.TrimSuffix(string(buf), "\n")
			runCmd("mseq", "-C", output)
			u.displaymail()
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

// cmdtoi wraps runCmd and parses the output as an integer.
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

// runCmd runs the cmd in the background.
func runCmd(cmd string, args ...string) ([]string, error) {
	c := exec.Command(cmd, args...)
	c.Env = os.Environ()
	buf, err := c.Output()
	output := strings.TrimSuffix(string(buf), "\n")
	return strings.Split(output, "\n"), err
}

// execCmd susepnds the user interface and runs the cmd.
// It resumes the interface when it is done.
func (u *UI) execCmd(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := u.s.Suspend(); err != nil {
		return err
	}
	defer u.s.Resume()
	return c.Run()
}

func main() {
	ui, err := NewUI(styleDefault)
	if err != nil {
		panic(err)
	}
	defer ui.Exit()
	for {
		ui.Run()
	}
}
