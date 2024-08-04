// Command mess implements a less(1)-like wrapper for the mblaze suite.
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
)

var (
	// limit determines how many messages there will be in the overview (mscan output)
	// TODO(thimc): We are currently limited to using 5 because of the implementation of [scanfmt].
	limit = 5

	styleDefault = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	styleError   = styleDefault.Reverse(true)
	style        = styleDefault
)

type point struct{ x, y int }

func drawString(s tcell.Screen, p *point, str string, ml bool) {
	w, h := s.Size()
	if p.y >= h {
		return
	}
	for _, r := range []rune(str) {
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
		return ".-5:.+0"
	default:
		return ".-2:.+3"
	}
}

type UI struct {
	s         tcell.Screen
	dot       int
	mailcount int
	maillen   int
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
	return &UI{s: s}, nil
}

// Run runs the UI for one frame, it returns io.EOF when the user has
// requested the program to exit. Any other error is handled by
// rendering them on the screen.
func (u *UI) Run() error {
	// TODO(thimc): Handle the errors in the event loop by printing them
	// to the screen rather than panicking when it makes sense.
	u.s.Clear()
	if err := u.draw(); err != nil {
		u.s.Suspend()
		log.Fatal(err)
	}
	u.s.Show()
	if err := u.event(); err != nil {
		if errors.Is(err, io.EOF) {
			return err
		}
		u.s.Suspend()
		log.Fatal(err)
	}
	return nil
}

// Close destroys the user interface and quits the program.
func (u *UI) Close() {
	u.s.Fini()
	os.Exit(0)
}

func (u *UI) draw() error {
	var err error
	if u.mailcount, err = cmdtoi("mscan", "-n", "--", "-1"); err != nil {
		return err
	}
	if u.dot, err = cmdtoi("mscan", "-f", "%n", "."); err != nil {
		return err
	}
	var p = point{0, 0}

	overview, err := runCmd("mscan", scanfmt(u.dot, u.mailcount))
	if err != nil {
		return err
	}
	for _, ln := range overview {
		p.x = 0
		drawString(u.s, &p, ln, false)
		p.y++
	}

	var out []string
	if u.raw {
		fpath, err := runCmd("mseq", "-r", fmt.Sprint(u.dot))
		if err != nil {
			return err
		}
		if len(fpath) < 1 {
			return fmt.Errorf("mseq -r: empty output")
		}
		var fname = fpath[0]
		f, err := os.Open(fname)
		if err != nil {
			return err
		}
		defer f.Close()
		buf, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		out = append([]string{fname}, strings.Split(string(buf), "\n")...)
	} else {
		args := []string{fmt.Sprint(u.dot)}
		if u.html {
			args = []string{"-A", "text/html", fmt.Sprint(u.dot)}
		}
		out, err = runCmd("mshow", args...)
		if err != nil {
			return err
		}
	}
	for n, ln := range out {
		if n < u.poffset {
			continue
		}
		p.x = 0
		drawString(u.s, &p, ln, true)
		p.y++
	}
	u.maillen = len(out)
	u.statusbar()
	return nil
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

func (u *UI) statusbar() {
	wmax, hmax := u.s.Size()
	for x := range wmax {
		u.s.SetContent(x, hmax-1, ' ', nil, style)
	}
	style = styleError
	var (
		s  = fmt.Sprintf("mail %d of %d", u.dot, u.mailcount)
		pt = &point{x: 0, y: hmax - 1}
	)
	drawString(u.s, pt, s, false)
	style = styleDefault
}

func (u *UI) event() error {
	switch ev := u.s.PollEvent(); ev := ev.(type) {
	case *tcell.EventResize:
		u.s.Sync()
	case *tcell.EventKey:
		switch {
		case ev.Rune() == '^':
			_, err := runCmd("mseq", "-C", ".^")
			return err
		case ev.Rune() == '0':
			u.dot = 1
			_, err := runCmd("mseq", "-C", fmt.Sprint(u.dot))
			return err
		case ev.Rune() == '$':
			u.dot = u.mailcount
			_, err := runCmd("mseq", "-C", fmt.Sprint(u.dot))
			return err
		case ev.Rune() == 'c':
			if err := u.execCmd("mcom"); err != nil {
				return err
			}
		case ev.Rune() == 'd':
			if _, err := runCmd("mflag", "-S", "."); err != nil {
				return err
			}
			if err := u.refresh(); err != nil {
				return err
			}
			_, err := runCmd("mseq", "-C", "+")
			return err
		case ev.Rune() == 'f':
			if err := u.execCmd("mfwd"); err != nil {
				return err
			}
		case ev.Rune() == 'g', ev.Key() == tcell.KeyHome:
			u.poffset = 0
		case ev.Rune() == 'j', ev.Key() == tcell.KeyDown, ev.Key() == tcell.KeyEnter:
			max := u.maillen - limit - 1
			if max < 0 {
				max = 0
			}
			if u.poffset >= max {
				u.poffset = max
				return fmt.Errorf("already at the bottom")
			} else {
				u.poffset++
			}
		case ev.Rune() == 'k', ev.Key() == tcell.KeyUp:
			u.poffset--
			if u.poffset < 0 {
				u.poffset = 0
				return fmt.Errorf("already at the top")
			}
		case ev.Rune() == 'q':
			return io.EOF
		case ev.Rune() == 'r':
			if err := u.execCmd("mrep"); err != nil {
				return err
			}
		case ev.Rune() == 'u':
			if _, err := runCmd("mflag", "-s", "."); err != nil {
				return err
			}
			if err := u.refresh(); err != nil {
				return err
			}
			_, err := runCmd("mseq", "-C", "+")
			return err
		case ev.Rune() == 'D', ev.Key() == tcell.KeyDelete:
			var delete bool
		prompt:
			for {
				if err := u.draw(); err != nil {
					return err
				}
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
				curr, err := runCmd("mseq", ".")
				if err != nil {
					return err
				}
				if err := u.refresh(); err != nil {
					return err
				}
				return os.Remove(curr[0])
			}
		case ev.Rune() == 'G', ev.Key() == tcell.KeyEnd:
			max := u.maillen - limit - 1
			if max < 0 {
				max = 0
			}
			u.poffset = max
		case ev.Rune() == 'H':
			u.html = !u.html
		case ev.Rune() == 'J':
			u.poffset = 0
			_, err := runCmd("mseq", "-C", ".+1")
			return err
		case ev.Rune() == 'K':
			u.poffset = 0
			_, err := runCmd("mseq", "-C", ".-1")
			return err
		case ev.Rune() == 'N':
			unseen, err := runCmd("magrep", "-v", "-m1", ":S", ".:")
			if err != nil {
				return err
			}
			_, err = runCmd("mseq", "-C", unseen[0])
			return err
		case ev.Rune() == 'R':
			u.raw = !u.raw
		case ev.Rune() == 'T':
			mails, err := runCmd("mseq", ".+1:")
			if err != nil {
				return err
			}
			c := exec.Command("sed", "-n", "/^[^ <]/{p;q;}")
			c.Env = os.Environ()
			c.Stdin = strings.NewReader(strings.Join(mails, "\n"))
			buf, err := c.Output()
			if err != nil {
				return err
			}
			output := strings.TrimSuffix(string(buf), "\n")
			_, err = runCmd("mseq", "-C", output)
			return err
		case ev.Key() == tcell.KeyCtrlD, ev.Key() == tcell.KeyPgDn:
			_, pg := u.s.Size()
			pg -= limit - 1
			max := u.maillen - limit - 1
			if max < 0 {
				max = 0
			}
			if u.poffset+pg >= max {
				u.poffset = max
			} else {
				u.poffset += pg
			}
		case ev.Key() == tcell.KeyCtrlU, ev.Key() == tcell.KeyPgUp:
			_, pg := u.s.Size()
			pg -= limit - 1
			if u.poffset-pg <= 0 {
				u.poffset = 0
			} else {
				u.poffset -= pg
			}
		default:
			if ev.Key() == tcell.KeyCtrlL {
				u.s.Clear()
			}
		}
	}
	return nil
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
		if err := ui.Run(); err != nil {
			break
		}
	}
}
