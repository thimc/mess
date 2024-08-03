// Command mess implements a less(1)-like wrapper for the mblaze suite.
package main

import (
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

func draw(s tcell.Screen, p *point, dot, total int) {
	overview, err := runCmd("mscan", scanfmt(dot, total))
	if err != nil {
		log.Fatalf("mscan: %v", err)
	}
	for _, ln := range overview {
		p.x = 0
		drawString(s, p, ln, false)
		p.y++
	}
}

func current(s tcell.Screen, p *point, dot, o int, html, raw bool) (int, error) {
	var (
		out []string
		err error
	)
	if raw {
		fpath, err := runCmd("mseq", "-r", fmt.Sprint(dot))
		if err != nil {
			return -1, err
		}
		if len(fpath) < 1 {
			return -1, fmt.Errorf("mseq -r: empty output")
		}
		var fname = fpath[0]
		f, err := os.Open(fname)
		if err != nil {
			return -1, err
		}
		defer f.Close()
		buf, err := io.ReadAll(f)
		if err != nil {
			return -1, err
		}
		out = append([]string{fname}, strings.Split(string(buf), "\n")...)
	} else {
		args := []string{fmt.Sprint(dot)}
		if html {
			args = []string{"-A", "text/html", fmt.Sprint(dot)}
		}
		out, err = runCmd("mshow", args...)
		if err != nil {
			return -1, err
		}
	}
	for n, ln := range out {
		if n < o {
			continue
		}
		p.x = 0
		drawString(s, p, ln, true)
		p.y++
	}
	return p.y, nil
}

func statusbar(s tcell.Screen, dot, total int) {
	wmax, hmax := s.Size()
	for x := range wmax {
		s.SetContent(x, hmax-1, ' ', nil, style)
	}
	style = styleError
	drawString(s, &point{x: 0, y: hmax - 1}, fmt.Sprintf("mail %d of %d", dot, total), false)
	style = styleDefault
}

func execCmd(s tcell.Screen, cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := s.Suspend(); err != nil {
		return err
	}
	if err := c.Run(); err != nil {
		return err
	}
	if err := s.Resume(); err != nil {
		return err
	}
	return nil
}

func main() {
	s, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("new screen: %v", err)
	}
	if err := s.Init(); err != nil {
		log.Fatalf("init: %v", err)
	}
	s.SetStyle(style)
	s.Clear()
	cleanup := func() {
		s.Fini()
		os.Exit(0)
	}
	defer cleanup()
	var (
		total    int
		dot      int
		offset   int
		lncount  int
		showHTML bool
		raw      bool
	)
	for {
		// TODO(thimc): Handle the errors in the event loop by printing them
		// to the screen rather than panicking when it makes sense.
		s.Clear()
		if total, err = cmdtoi("mscan", "-n", "--", "-1"); err != nil {
			log.Fatalf("mscan: %v", err)
		}
		if dot, err = cmdtoi("mscan", "-f", "%n", "."); err != nil {
			log.Fatalf("mscan: %v", err)
		}
		var p = point{0, 0}
		draw(s, &p, dot, total)
		lncount, err = current(s, &p, dot, offset, showHTML, raw)
		if err != nil {
			log.Fatalf("current: %v", err)
		}
		statusbar(s, dot, total)

		s.Show()
		switch ev := s.PollEvent(); ev := ev.(type) {
		case *tcell.EventResize:
			s.Sync()
		case *tcell.EventKey:
			r := ev.Rune()
			drawString(s, &point{0, 0}, fmt.Sprintf("r=%c", r), false)
			switch {
			case ev.Key() == tcell.KeyHome:
				offset = 0
			case ev.Key() == tcell.KeyEnd:
				_, hmax := s.Size()
				max := lncount - hmax - limit - 1
				if max < 0 {
					max = 0
				}
				offset = max
			case r == '^':
				runCmd("mseq", "-C", "'.^'")
			case r == '0':
				dot = 1
				runCmd("mseq", "-C", fmt.Sprint(dot))
			case r == '$':
				dot = total
				runCmd("mseq", "-C", fmt.Sprint(dot))
			case r == 'c':
				if err := execCmd(s, "mcom"); err != nil {
					log.Fatalf("execCmd: %v", err)
				}
			case r == 'd':
				runCmd("mflags", "-S", ".")
				runCmd("mflags", "-f", ":", "|", "mseq", "-S")
				runCmd("mseq", "-C", "+")
			case r == 'f':
				if err := execCmd(s, "mfwd"); err != nil {
					log.Fatalf("execCmd: %v", err)
				}
			case r == 'j', ev.Key() == tcell.KeyDown, ev.Key() == tcell.KeyEnter:
				offset++
				// TODO(thimc): fix clamping when scrolling down the mail
				// _, hmax := s.Size()
				// max := lncount - hmax - limit - 1
				// if max < 0 {
				// 	max = 0
				// }
				// if offset >= max {
				// 	offset = max
				// }
			case r == 'k', ev.Key() == tcell.KeyUp:
				offset--
				if offset < 0 {
					offset = 0
				}
			case r == 'q':
				cleanup()
			case r == 'r':
				if err := execCmd(s, "mrep"); err != nil {
					log.Fatalf("execCmd: %v", err)
				}
			case r == 'u':
				runCmd("mflags", "-s", ".")
				runCmd("mflags", "-f", ":", "|", "mseq", "-S")
				runCmd("mseq", "-C", "+")
			case r == 'D', ev.Key() == tcell.KeyDelete:
				var delete bool
			prompt:
				for {
					// TODO(thimc): delete mail loop: Resizing will have consequences :)
					wmax, hmax := s.Size()
					for x := range wmax {
						s.SetContent(x, hmax-1, ' ', nil, style)
					}
					style = styleError
					drawString(s, &point{x: 0, y: hmax - 1}, "Delete the selected mail? (y/N)", false)
					style = styleDefault
					s.Show()
					switch e := s.PollEvent(); e := e.(type) {
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
						log.Fatalf("mseq: %v", err)
					}
					if _, err := runCmd("rm", curr[0]); err != nil {
						log.Fatalf("rm: %v", err)
					}
				}
			case r == 'H':
				showHTML = !showHTML
			case r == 'J':
				runCmd("mseq", "-C", ".+1")
				offset = 0
			case r == 'K':
				runCmd("mseq", "-C", ".-1")
				offset = 0
			case r == 'N':
				unseen, err := runCmd("magrep", "-v", "-m1", ":S", ".:")
				if err != nil {
					log.Fatalf("magrep: %v", err)
				}
				runCmd("mseq", "-C", unseen[0])
			case r == 'R':
				raw = !raw
			case r == 'T':
				thread, err := runCmd("mseq", ".+1:", "|", "sed", "-n", "'/^[^ <]/{p;q;}'")
				if err != nil {
					log.Fatalf("mseq: %v", err)
				}
				runCmd("mseq", "-C", thread[0])
			default:
				if ev.Key() == tcell.KeyCtrlL {
					s.Clear()
				}
			}
		}
	}
}
