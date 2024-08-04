// Command mess implements a less(1)-like wrapper for the mblaze suite.
package main

import (
	"fmt"
	"io"
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
			continue
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

func scanfmt(dot, total int) string {
	// TODO(thimc): Calculate how the range should be defined rather
	// than using hard coded values.
	switch dot {
	case 1:
		return ".-0:.+5"
	case 2:
		return ".-1:.+4"
	case limit - 2:
		return ".-3:.+2"
	case limit - 1:
		return ".-4:.+1"
	case total:
		return ".-5:.+0"
	default:
		return ".-2:.+3"
	}
}

type UI struct {
	s        tcell.Screen
	total    int
	dot      int
	offset   int
	lncount  int
	showHTML bool
	raw      bool
}

func newUI() (*UI, error) {
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

func (u *UI) Close() {
	u.s.Fini()
	os.Exit(0)
}

func (u *UI) Draw() error {
	// TODO(thimc): Handle the errors in the event loop by printing them
	// to the screen rather than panicking when it makes sense.
	u.s.Clear()
	var err error
	if u.total, err = cmdtoi("mscan", "-n", "--", "-1"); err != nil {
		return err
	}
	if u.dot, err = cmdtoi("mscan", "-f", "%n", "."); err != nil {
		return err
	}
	var p = point{0, 0}

	overview, err := runCmd("mscan", scanfmt(u.dot, u.total))
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
		if u.showHTML {
			args = []string{"-A", "text/html", fmt.Sprint(u.dot)}
		}
		out, err = runCmd("mshow", args...)
		if err != nil {
			return err
		}
	}
	for n, ln := range out {
		if n < u.offset {
			continue
		}
		p.x = 0
		drawString(u.s, &p, ln, true)
		p.y++
	}
	u.lncount = len(out)

	wmax, hmax := u.s.Size()
	for x := range wmax {
		u.s.SetContent(x, hmax-1, ' ', nil, style)
	}
	style = styleError
	drawString(u.s, &point{x: 0, y: hmax - 1}, fmt.Sprintf("mail %d of %d", u.dot, u.total), false)
	style = styleDefault

	u.s.Show()
	return nil
}

func (u *UI) Event() error {
	switch ev := u.s.PollEvent(); ev := ev.(type) {
	case *tcell.EventResize:
		u.s.Sync()
	case *tcell.EventKey:
		r := ev.Rune()
		drawString(u.s, &point{0, 0}, fmt.Sprintf("r=%c", r), false)
		switch {
		case r == '^':
			if _, err := runCmd("mseq", "-C", "'.^'"); err != nil {
				return err
			}
		case r == '0':
			u.dot = 1
			if _, err := runCmd("mseq", "-C", fmt.Sprint(u.dot)); err != nil {
				return err
			}
		case r == '$':
			u.dot = u.total
			if _, err := runCmd("mseq", "-C", fmt.Sprint(u.dot)); err != nil {
				return err
			}
		case r == 'c':
			if err := u.execCmd("mcom"); err != nil {
				return err
			}
		case r == 'd':
			if _, err := runCmd("mflag", "-S", "."); err != nil {
				return err
			}
			if _, err := runCmd("mflag", "-f", ":", "|", "mseq", "-S"); err != nil {
				return err
			}
			if _, err := runCmd("mseq", "-C", "+"); err != nil {
				return err
			}
		case r == 'f':
			if err := u.execCmd("mfwd"); err != nil {
				return err
			}
		case r == 'g', ev.Key() == tcell.KeyHome:
			u.offset = 0
		case r == 'j', ev.Key() == tcell.KeyDown, ev.Key() == tcell.KeyEnter:
			_, hmax := u.s.Size()
			hmax += (limit * 4)
			if hmax < 0 {
				hmax = 0
			}
			if u.offset >= hmax {
				u.offset = hmax
			} else {
				u.offset++
			}
		case r == 'k', ev.Key() == tcell.KeyUp:
			u.offset--
			if u.offset < 0 {
				u.offset = 0
			}
		case r == 'q':
			u.Close()
		case r == 'r':
			if err := u.execCmd("mrep"); err != nil {
				return err
			}
		case r == 'u':
			runCmd("mflag", "-s", ".")
			runCmd("mflag", "-f", ":", "|", "mseq", "-S")
			runCmd("mseq", "-C", "+")
		case r == 'D', ev.Key() == tcell.KeyDelete:
			var delete bool
		prompt:
			for {
				if err := u.Draw(); err != nil {
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
				if _, err := runCmd("rm", curr[0]); err != nil {
					return err
				}
			}
		case r == 'G', ev.Key() == tcell.KeyEnd:
			max := u.lncount - limit - 1
			if max < 0 {
				max = 0
			}
			u.offset = max
		case r == 'H':
			u.showHTML = !u.showHTML
		case r == 'J':
			if _, err := runCmd("mseq", "-C", ".+1"); err != nil {
				return err
			}
			u.offset = 0
		case r == 'K':
			if _, err := runCmd("mseq", "-C", ".-1"); err != nil {
				return err
			}
			u.offset = 0
		case r == 'N':
			unseen, err := runCmd("magrep", "-v", "-m1", ":S", ".:")
			if err != nil {
				return err
			}
			if _, err := runCmd("mseq", "-C", unseen[0]); err != nil {
				return err
			}
		case r == 'R':
			u.raw = !u.raw
		case r == 'T':
			thread, err := runCmd("mseq", ".+1:", "|", "sed", "-n", "'/^[^ <]/{p;q;}'")
			if err != nil {
				return err
			}
			if _, err := runCmd("mseq", "-C", thread[0]); err != nil {
				return err
			}
		default:
			if ev.Key() == tcell.KeyCtrlL {
				u.s.Clear()
			}
		}
	}
	return nil
}

func cmdtoi(cmd string, args ...string) (int, error) {
	out, err := runCmd(cmd, args...)
	if err != nil {
		return -1, err
	}
	if len(out) < 1 {
		return -1, fmt.Errorf("run %v: empty output", cmd)
	}
	n, err := strconv.Atoi(out[0])
	if err != nil {
		return -1, err
	}
	return n, nil
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
	ui, err := newUI()
	if err != nil {
		panic(err)
	}
	defer ui.Close()

	for {
		ui.Draw()
		ui.Event()
	}
}
