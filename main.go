package main

import (
	"embed"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/godecide/tree"
)

//go:embed examples/*.yaml
var fs embed.FS

var usage string = `Usage: %s {src} {dst}
	- src is either 'stdin', 'example:NAME', or a filename
	- dst is either (stdout|xdot|yaml) or a filename

	e.g.:  'godecide example:hbr xdot' runs xdot with the hbr example 

	%s`

func main() {
	now := time.Now()

	if len(os.Args) < 3 {
		warn(usage, os.Args[0], tree.LsExamples(fs))
		os.Exit(1)
	}
	//  get subcommand
	src := os.Args[1]
	dst := os.Args[2]

	var buf []byte
	var err error

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

	tree.TreeCalc(roots, now, warn)

	dotbuf := tree.ToDot(roots, warn)

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
