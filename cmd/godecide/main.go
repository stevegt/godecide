package main

import (
	"embed"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	. "github.com/stevegt/goadapt"
	tree "github.com/stevegt/godecide"
)

//go:embed tree/examples/*.yaml
var fs embed.FS

var usage string = `Usage: %s [-tb -now=<RFC3339 timestamp>] {src} {dst}

src: either 'stdin', 'example:NAME', or a filename
dst: either (stdout|xdot|yaml) or a filename

%s

e.g.:  'godecide example:hbr xdot' runs xdot with the hbr example 

For the -now flag, the default is the current time.

`

func main() {
	// set custom usage
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0], tree.LsExamples(fs))
		fmt.Fprint(os.Stderr, "Flags:\n\n")
		flag.PrintDefaults()
	}

	// parse flags
	var tb bool
	nowStr := time.Now().Format(time.RFC3339)
	flag.BoolVar(&tb, "tb", false, "set graphviz rankdir=TB (top to bottom)")
	flag.StringVar(&nowStr, "now", nowStr, "set timestamp (in RFC3339 format) for current time")
	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	now, err := time.Parse(time.RFC3339, nowStr)
	Ck(err)

	// get src and dst
	src := flag.Arg(0)
	dst := flag.Arg(1)

	var buf []byte

	// get input
	switch {
	case strings.HasPrefix(src, "example:"):
		buf, err = tree.CatExample(fs, src)
		Ck(err)
	case src == "stdin":
		buf, err = ioutil.ReadAll(os.Stdin)
		Ck(err)
	default:
		buf, err = ioutil.ReadFile(src)
		Ck(err)
	}

	// parse
	roots, err := tree.FromYAML(buf)
	Ck(err)

	tree.Recalc(roots, now, warn)

	dotbuf := tree.ToDot(roots, warn, tb)

	switch dst {
	case "stdout":
		fmt.Print(string(dotbuf))
	case "yaml":
		fmt.Print(string(buf))
	case "xdot":
		tmpfile, err := ioutil.TempFile("/tmp", "godecide.*.dot")
		Ck(err)
		defer os.Remove(tmpfile.Name())
		_, err = tmpfile.Write(dotbuf)
		Ck(err)
		err = tmpfile.Close()
		Ck(err)
		cmd := exec.Command("xdot", tmpfile.Name())
		err = cmd.Run()
		Ck(err)
	default:
		fh, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			// backup existing dst file
			bakfn := Spf("%s-*.dot", path.Base(dst))
			bakfh, err := ioutil.TempFile("/tmp", bakfn)
			Ck(err)
			bakbuf, err := ioutil.ReadFile(dst)
			Ck(err)
			_, err = bakfh.Write(bakbuf)
			Ck(err)
			err = bakfh.Close()
			Ck(err)
			fh, err = os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC, 0644)
			Ck(err)
		}
		_, err = fh.Write(dotbuf)
		Ck(err)
		err = fh.Close()
		Ck(err)
	}
	return
}

func warn(args ...interface{}) {
	msg := formatArgs(args...)
	fmt.Fprintf(os.Stderr, msg)
}

func formatArgs(args ...interface{}) (msg string) {
	if len(args) == 1 {
		msg = fmt.Sprintf("%v", args[0])
	}
	if len(args) > 1 {
		msg = fmt.Sprintf(args[0].(string), args[1:]...)
	}
	return
}
