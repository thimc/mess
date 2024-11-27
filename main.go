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

	limitflag = flag.Int("limit", 5, "amount of mails to be previewed in mscan")
	mouseflag = flag.Bool("mouse", false, "enables mouse support")

	pager     string
	mshowArgs string
)

type point struct{ x, y int }

func draw(s tcell.Screen, p *point, style tcell.Style, str string, multiline bool) {
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
	u.t = tcellterm.New()
	_, h := u.v.Size()
	u.tv = views.NewViewPort(u.s, 0, h, -1, -1)
	u.t.SetSurface(u.tv)
	u.t.Attach(func(ev tcell.Event) { u.s.PostEvent(ev) })
	return u, u.mshow()
}

func (u *UI) mshow() error {
	var (
		args = strings.Split(mshowArgs, " ")
		cmd  *exec.Cmd
	)
	if u.raw {
		fname, err := u.runCmd(true, "mseq", ".")
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
	cmd.Env = append(append(os.Environ(), "MBLAZE_PAGER="+pager), "LESS=-R")
	u.t.Close()
	return u.t.Start(cmd)
}

func (u *UI) error(err error) {
	wmax, hmax := u.s.Size()
	for w := range wmax {
		u.s.SetContent(w, hmax-1, ' ', nil, styleDefault)
	}
	draw(u.s, &point{0, hmax - 1}, styleError, err.Error(), true)
	u.s.Show()
loop:
	for {
		ev := u.s.PollEvent()
		switch ev.(type) {
		case *tcell.EventKey:
			break loop
		}
	}
}

func (u *UI) scanfmt() (string, error) {
	out, err := u.runCmd(true, "mscan", "-n", "--", "-1")
	if err != nil {
		return "", err
	}
	u.total, err = strconv.Atoi(out[0])
	if err != nil {
		return "", err
	}
	var (
		dot    = u.dot - 1
		total  = u.total - 1
		limit  = *limitflag
		before = limit / 2
		after  = limit - before
	)
	if dot <= before {
		before = dot
		after = limit - before
	} else if dot > total-after {
		after = total - dot
		before = limit - after
	} else {
		before = limit / 2
		after = limit - before
	}
	return fmt.Sprintf(".-%d:.+%d", before, after), nil
}

func (u *UI) mscan() error {
	out, err := u.runCmd(true, "mscan", "-n", ".")
	if err != nil {
		return err
	}
	u.dot, err = strconv.Atoi(out[0])
	if err != nil {
		return err
	}

	u.rangefmt, err = u.scanfmt()
	if err != nil {
		return err
	}
	lines, err := u.runCmd(true, "mscan", u.rangefmt)
	if err != nil {
		return err
	}
	// TODO(thimc): Pipe output to something similar to the mless
	// "colorscan" awk function that colorizes the output.
	// Respect the NO_COLOR environment variable.
	u.p.SetContent(strings.Join(lines, "\n"))
	u.p.Draw()
	return nil
}

func (u *UI) Run() {
	defer u.Exit()
	for {
		if err := u.mscan(); err != nil {
			u.error(err)
		}
		u.t.Draw()
		u.s.Show()
		if err := u.update(u.s.PollEvent()); err != nil {
			u.error(err)
		}
	}
}

func (u *UI) Exit() {
	if u.t != nil {
		u.t.Close()
	}
	u.s.Fini()
	os.Exit(0)
}

func (u *UI) mseq() error {
	mails, err := u.runCmd(true, "mseq", "-f", ":")
	if err != nil {
		return err
	}
	cmd := exec.Command("mseq", "-S")
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(strings.Join(mails, "\n"))
	return cmd.Run()
}

func (u *UI) update(ev tcell.Event) error {
	if ev == nil {
		return nil
	}
	switch ev := ev.(type) {
	case *tcell.EventResize:
		// TODO(thimc): handle cases where the new window size
		// is lesser than *limitflag.
		w, h := ev.Size()
		u.v.Resize(0, 0, w, *limitflag+1)
		u.tv.Resize(0, *limitflag+1, w, h)
		u.t.Resize(w, h)
		u.p.Resize()
		if err := u.mscan(); err != nil {
			return err
		}
		return u.mshow()
	case *tcellterm.EventRedraw:
		u.t.Draw()
		u.p.Draw()
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
		if fn, ok := binds[binding{r: ev.Rune()}]; ok {
			return fn(ev, u)
		} else if fn, ok := binds[binding{k: ev.Key()}]; ok {
			return fn(ev, u)
		}
		if u.t != nil {
			u.t.HandleEvent(ev)
		}
	}
	return nil
}

func (u *UI) runCmd(bg bool, cmd string, args ...string) ([]string, error) {
	c := exec.Command(cmd, args...)
	c.Env = os.Environ()
	if !bg {
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := u.s.Suspend(); err != nil {
			return nil, err
		}
		defer u.s.Resume()
		return nil, c.Run()
	}
	buf, err := c.Output()
	if err != nil {
		return nil, err
	}
	output := strings.TrimSuffix(string(buf), "\n")
	return strings.Split(output, "\n"), nil
}

func init() {
	pager = os.Getenv("PAGER")
	mshowArgs = os.Getenv("MSHOW_ARGS")
	if pager == "" || pager == "less" {
		pager = "less -R"
	}
}

func main() {
	flag.Usage = func() {
		fmt.Printf("Usage: %s [msg]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	f := os.Stdin
	fi, err := f.Stat()
	if err != nil {
		log.Fatalln(err)
	}
	if flag.NArg() > 0 {
		arg := os.Args[len(os.Args)-1]
		cmd := exec.Command("mseq", "-C", arg)
		if err := cmd.Run(); err != nil {
			log.Fatalf("%s %s: %q", cmd.Path, strings.Join(cmd.Args, " "), err)
		}
	}
	ui, err := NewUI(styleDefault)
	if err != nil {
		panic(err)
	}
	if (fi.Mode() & os.ModeCharDevice) < 1 {
		buf, err := io.ReadAll(f)
		if err != nil {
			log.Fatalln(err)
		}
		cmd := exec.Command("mseq", "-S")
		cmd.Env = os.Environ()
		cmd.Stdin = bytes.NewBuffer(buf)
		if err := cmd.Run(); err != nil {
			log.Fatalf("%s %s: %q", cmd.Path, strings.Join(cmd.Args, " "), err)
		}
		cmd = exec.Command("mseq", "-C", "1")
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			log.Fatalf("%s %s: %q", cmd.Path, strings.Join(cmd.Args, " "), err)
		}
	}
	ui.Run()
}
