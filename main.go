// Command mess implements a less(1)-like wrapper for the mblaze suite.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tcellterm "git.sr.ht/~rockorager/tcell-term"
	"github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v2/views"
)

var (
	styleDefault = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	styleError   = styleDefault.Reverse(true)
	style        = styleDefault

	// TODO(thimc): We are currently limited to using 5 because of the implementation of [scanfmt].
	limitflag = flag.Int("limit", 5, "amount of mails to be previewed in mscan")
	mouseflag = flag.Bool("mouse", true, "enables mouse support")
)

type point struct{ x, y int }

func drawString(s tcell.Screen, p *point, str string, multiline bool) {
	w, h := s.Size()
	if p.y >= h {
		return
	}
	for _, r := range str {
		s.SetContent(p.x, p.y, r, nil, style)
		p.x++
		if r == '\n' || (multiline && p.x >= w) {
			p.y++
			p.x = 0
		} else if !multiline && p.x >= w {
			break
		}
	}
}

// scanfmt determines how the mscan format should be printed.
//
// TODO(thimc): Calculate how the range should be defined rather
// than using hard coded values.
func (u *UI) scanfmt() (string, error) {
	var err error
	u.total, err = cmdtoi("mscan", "-n", "--", "-1")
	if err != nil {
		return "", err
	}
	var s string
	switch u.dot {
	case 1:
		s = ".-0:.+5"
	case 2:
		s = ".-1:.+4"
	case u.total - 2:
		s = ".-3:.+2"
	case u.total - 1:
		s = ".-4:.+1"
	case u.total:
		s = ".-5:.+0"
	default:
		s = ".-2:.+3"
	}
	return s, nil
}

type UI struct {
	s  tcell.Screen
	v  *views.ViewPort // mscan view
	tv *views.ViewPort // mshow view
	p  *views.TextArea
	t  *tcellterm.VT

	dot      int
	total    int
	rangefmt string

	html bool // Assume the mail is HTML
	raw  bool // Print the raw file
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
	s.EnablePaste()
	u := &UI{s: s}
	if *mouseflag {
		u.s.EnableMouse()
	}
	u.v = views.NewViewPort(u.s, 0, 0, -1, *limitflag+1)
	u.p = views.NewTextArea()
	u.p.SetView(u.v)
	u.setupterm()
	return u, u.mshow()
}

// setupterm initializes the virtual terminal which runs a process
// that renders a mail.
func (u *UI) setupterm() {
	u.t = tcellterm.New()
	_, h := u.v.Size()
	u.tv = views.NewViewPort(u.s, 0, h, -1, -1)
	u.t.SetSurface(u.tv)
	u.t.Attach(func(ev tcell.Event) { u.s.PostEvent(ev) })
}

// mshow prints the current mail in a virtual terminal, launched
// with `$MBLAZE_PAGER`
func (u *UI) mshow() error {
	var (
		pager     = os.Getenv("PAGER")
		mshowArgs = os.Getenv("MSHOW_ARGS")
		args      = strings.Split(mshowArgs, " ")
		cmd       *exec.Cmd
	)
	if pager == "" || pager == "less" {
		pager = "less -R"
	}
	if u.raw {
		fname, err := runCmd("mseq", ".")
		if err != nil {
			return err
		}
		p := strings.Split(pager, " ")
		var a []string
		if len(p) > 1 {
			a = append(a, p[1:]...)
		}
		a = append(a, fname[0])
		cmd = exec.Command(p[0], a...)
	} else {
		if u.html {
			args = append(args, "-A", "text/html")
		}
		cmd = exec.Command("mshow", args...)
	}
	args = append(args, "COLUMNS=99999999")
	args = append(args, ".")
	cmd.Env = append(os.Environ(), "MBLAZE_PAGER="+pager)
	if u.t != nil {
		u.t.Close()
	}
	u.setupterm()
	return u.t.Start(cmd)
}

func (u *UI) errorf(format string, v ...any) {
	u.s.Clear()
	var (
		msg        = "Error"
		string     = fmt.Sprintf(format, v...)
		wmax, hmax = u.s.Size()
	)
	u.s.SetStyle(styleError)
	drawString(u.s, &point{(wmax - len(msg)) / 2, hmax/2 - 1}, msg, true)
	u.s.SetStyle(styleDefault)
	drawString(u.s, &point{(wmax - len(string)) / 2, hmax / 2}, string, true)
	u.s.Show()
loop:
	for {
		ev := u.s.PollEvent()
		if ev == nil {
			break loop
		}
		switch ev.(type) {
		case *tcell.EventKey:
			break loop
		}
	}
	u.Exit()
}

// Run runs the UI for one frame, it returns io.EOF when the user has
// requested the program to exit. Any other error is handled by
// rendering them on the screen.
func (u *UI) Run() {
	var err error
	defer u.Exit()
	for {
		u.dot, err = cmdtoi("mscan", "-n", ".")
		if err != nil {
			u.errorf("mscan: %s", err)
			continue
		}
		u.rangefmt, err = u.scanfmt()
		if err != nil {
			u.errorf("scanfmt: %s", err)
			continue
		}
		lines, err := runCmd("mscan", []string{u.rangefmt}...)
		if err != nil {
			u.errorf("mscan: %s", err)
			continue
		}
		// TODO(thimc): Pipe output to something similar to mless
		// "colorscan" function that colorizes the output. Respect
		// the NO_COLOR environment variable.
		u.p.SetContent(strings.Join(lines, "\n"))
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

// mseq reinitializes the mblaze mailbox sequence.
func (u *UI) mseq() error {
	defer u.mshow()
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
func (u *UI) update(ev tcell.Event) error {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		u.s.Sync()
	case *tcellterm.EventRedraw:
		u.p.Draw()
		u.t.Draw()
	case *tcell.EventMouse:
		var e *tcell.EventKey
		switch ev.Buttons() {
		case tcell.WheelDown:
			e = tcell.NewEventKey(tcell.KeyRune, 'j', 0)
		case tcell.WheelUp:
			e = tcell.NewEventKey(tcell.KeyRune, 'k', 0)
		}
		if e != nil {
			u.t.HandleEvent(e)
			return nil
		}
	case *tcell.EventKey:
		switch {
		case ev.Rune() == '^':
			_, err := runCmd("mseq", "-C", ".^")
			return err
		case ev.Rune() == '0':
			if _, err := runCmd("mseq", "-C", "1"); err != nil {
				return err
			}
			return u.mshow()
		case ev.Rune() == '$':
			total, err := cmdtoi("mscan", "-n", "--", "-1")
			if err != nil {
				return err
			}
			if _, err := runCmd("mseq", "-C", fmt.Sprint(total)); err != nil {
				return err
			}
			return u.mshow()
		case ev.Rune() == 'c':
			return u.execCmd("mcom")
		case ev.Rune() == 'd':
			if _, err := runCmd("mflag", "-S", "."); err != nil {
				return err
			}
			if err := u.mseq(); err != nil {
				return err
			}
			_, err := runCmd("mseq", "-C", "+")
			return err
		case ev.Rune() == 'f':
			return u.execCmd("mfwd")
		case ev.Rune() == 'q':
			u.Exit()
		case ev.Rune() == 'r':
			return u.execCmd("mrep")
		case ev.Rune() == 'u':
			_, err := runCmd("mflag", "-s", ".")
			if err != nil {
				return err
			}
			if err := u.mseq(); err != nil {
				return err
			}
			_, err = runCmd("mseq", "-C", "+")
			return err
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
				curr, err := runCmd("mseq", ".")
				if err != nil {
					return err
				}
				_ = err
				defer os.Remove(curr[0])
				if _, err := runCmd("mseq", "-C", "+"); err != nil {
					return err
				}
				return u.mseq()
			}
			return nil
		case ev.Rune() == 'H':
			u.html = !u.html
			return u.mshow()
		case ev.Rune() == 'J':
			if _, err := runCmd("mseq", "-C", ".+1"); err != nil {
				return err
			}
			return u.mshow()
		case ev.Rune() == 'K':
			if _, err := runCmd("mseq", "-C", ".-1"); err != nil {
				return err
			}
			return u.mshow()
		case ev.Rune() == 'N':
			unseen, err := runCmd("magrep", "-v", "-m1", ":S", ".:")
			if err != nil {
				return err
			}
			if _, err := runCmd("mseq", "-C", unseen[0]); err != nil {
				return err
			}
			return u.mseq()
		case ev.Rune() == 'R':
			u.raw = !u.raw
			return u.mshow()
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
			if _, err := runCmd("mseq", "-C", output); err != nil {
				return err
			}
			return u.mshow()
		case ev.Key() == tcell.KeyCtrlL:
			u.s.Clear()
			return nil
		}
		if u.t != nil {
			u.t.HandleEvent(ev)
		}
	}
	return nil
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
	if err != nil {
		return nil, err
	}
	output := strings.TrimSuffix(string(buf), "\n")
	return strings.Split(output, "\n"), nil
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
	flag.Parse()
	f := os.Stdin
	fi, err := f.Stat()
	if err != nil {
		log.Fatalln(err)
	}
	if (fi.Mode() & os.ModeCharDevice) < 1 {
		buf, err := io.ReadAll(f)
		if err != nil {
			log.Fatalln(err)
		}
		cmd := exec.Command("mseq", "-S")
		cmd.Stdin = bytes.NewBuffer(buf)
		if err := cmd.Run(); err != nil {
			log.Fatalf("mseq -S: %q", err)
		}
		if _, err := runCmd("mseq", "-C", "1"); err != nil {
			log.Fatalf("mseq -C 1: %q", err)
		}
	}
	ui, err := NewUI(styleDefault)
	if err != nil {
		panic(err)
	}
	ui.Run()
}
