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
	"strings"
)

var (
	limitflag = flag.Int("limit", 5, "amount of mails to be previewed in mscan")
	mouseflag = flag.Bool("mouse", false, "enables mouse support")
)

func main() {
	pager = os.Getenv("PAGER")
	mshowArgs = os.Getenv("MSHOW_ARGS")
	if pager == "" || pager == "less" {
		pager = "less -R"
	}
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
