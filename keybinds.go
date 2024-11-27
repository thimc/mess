package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
)

type binding struct {
	r rune
	k tcell.Key
}

type cmd func(ev *tcell.EventKey, u *UI) error

var binds map[binding]cmd

func init() {
	binds = map[binding]cmd{
		{r: '^'}:             cmdParent,
		{r: '0'}:             cmdTop,
		{r: '$'}:             cmdBottom,
		{r: 'c'}:             cmdCompose,
		{r: 'd'}:             cmdMarkRead,
		{r: 'f'}:             cmdForward,
		{r: 'q'}:             cmdQuit,
		{r: 'r'}:             cmdReply,
		{r: 'u'}:             cmdMarkUnread,
		{r: 'D'}:             cmdDelete,
		{k: tcell.KeyDelete}: cmdDelete,
		{r: 'J'}:             cmdNextMail,
		{r: 'K'}:             cmdPrevMail,
		{r: 'N'}:             cmdNextUnseen,
		{r: 'R'}:             cmdToggleRaw,
		{r: 'T'}:             cmdMoveThread,
		{r: 't'}:             cmdMoveThread,
		{k: tcell.KeyCtrlL}:  cmdClear,
	}
}

func cmdParent(ev *tcell.EventKey, u *UI) error {
	_, err := u.runCmd(true, "mseq", "-C", ".^")
	return err
}

func cmdTop(ev *tcell.EventKey, u *UI) error {
	if _, err := u.runCmd(true, "mseq", "-C", "1"); err != nil {
		return err
	}
	return u.mshow()
}

func cmdBottom(ev *tcell.EventKey, u *UI) error {
	if _, err := u.runCmd(true, "mseq", "-C", fmt.Sprint(u.total)); err != nil {
		return err
	}
	return u.mshow()
}

func cmdCompose(ev *tcell.EventKey, u *UI) error {
	_, err := u.runCmd(false, "mcom")
	return err
}

func cmdMarkRead(ev *tcell.EventKey, u *UI) error {
	if _, err := u.runCmd(true, "mflag", "-S", "."); err != nil {
		return err
	}
	if err := u.mseq(); err != nil {
		return err
	}
	if _, err := u.runCmd(true, "mseq", "-C", "+"); err != nil {
		return err
	}
	return u.mshow()
}

func cmdForward(ev *tcell.EventKey, u *UI) error {
	_, err := u.runCmd(false, "mfwd")
	return err
}

func cmdQuit(ev *tcell.EventKey, u *UI) error { u.Exit(); return nil }

func cmdReply(ev *tcell.EventKey, u *UI) error {
	_, err := u.runCmd(false, "mrep")
	return err
}

func cmdMarkUnread(ev *tcell.EventKey, u *UI) error {
	_, err := u.runCmd(true, "mflag", "-s", ".")
	if err != nil {
		return err
	}
	if err := u.mseq(); err != nil {
		return err
	}
	_, err = u.runCmd(true, "mseq", "-C", "+")
	return err
}

func cmdDelete(ev *tcell.EventKey, u *UI) error {
	var delete bool
prompt:
	for {
		wmax, hmax := u.s.Size()
		for x := range wmax {
			u.s.SetContent(x, hmax-1, ' ', nil, styleDefault)
		}
		draw(u.s, &point{x: 0, y: hmax - 1}, styleError, "Delete the selected mail? (y/N)", false)
		u.s.Show()
		switch e := u.s.PollEvent(); e := e.(type) {
		case *tcell.EventKey:
			delete = e.Rune() == 'y' || e.Rune() == 'Y'
			break prompt
		}
	}
	if delete {
		curr, err := u.runCmd(true, "mseq", ".")
		if err != nil {
			return err
		}
		if err := os.Remove(curr[0]); err != nil {
			return err
		}
		defer u.update(tcell.NewEventKey(tcell.KeyRune, 'J', 0))
		return u.mshow()
	}
	return nil
}

func cmdToggleHTML(ev *tcell.EventKey, u *UI) error {
	u.html = !u.html
	return u.mshow()
}

func cmdNextMail(ev *tcell.EventKey, u *UI) error {
	if u.dot >= u.total {
		return nil
	}
	if _, err := u.runCmd(true, "mseq", "-C", ".+1"); err != nil {
		return err
	}
	return u.mshow()
}

func cmdPrevMail(ev *tcell.EventKey, u *UI) error {
	if u.dot <= 1 {
		return nil
	}
	if _, err := u.runCmd(true, "mseq", "-C", ".-1"); err != nil {
		return err
	}
	return u.mshow()
}

func cmdNextUnseen(ev *tcell.EventKey, u *UI) error {
	seq, err := u.runCmd(true, "magrep", "-v", "-m1", ":S", ".:")
	if err != nil {
		return err
	}
	unseen := strings.TrimLeft(seq[0], " ")
	if _, err := u.runCmd(true, "mseq", []string{"-C", unseen}...); err != nil {
		return err
	}
	return u.mshow()
}

func cmdToggleRaw(ev *tcell.EventKey, u *UI) error {
	u.raw = !u.raw
	return u.mshow()
}

func cmdMoveThread(ev *tcell.EventKey, u *UI) error {
	var cmd string = ".+1:"
	if ev.Rune() == 't' {
		cmd = "0:.-1"
	}
	mails, err := u.runCmd(true, "mseq", cmd)
	if err != nil {
		return err
	}
	if ev.Rune() == 't' {
		for i, j := 0, len(mails)-1; i < j; i, j = i+1, j-1 {
			mails[i], mails[j] = mails[j], mails[i]
		}
	}
	c := exec.Command("sed", "-n", "/^[^ <]/{p;q;}")
	c.Env = os.Environ()
	c.Stdin = strings.NewReader(strings.Join(mails, "\n"))
	buf, err := c.Output()
	if err != nil {
		return err
	}
	output := strings.TrimSuffix(string(buf), "\n")
	if _, err := u.runCmd(true, "mseq", "-C", output); err != nil {
		return err
	}
	return u.mshow()
}

func cmdClear(ev *tcell.EventKey, u *UI) error {
	u.s.Clear()
	return nil
}
