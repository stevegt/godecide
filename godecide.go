package main

import (
	"bytes"
	"embed"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/soudy/mathcat"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/godecide/fin"

	"gopkg.in/yaml.v2"
)

//go:embed examples/*.yaml
var fs embed.FS

var now = time.Now()

var Fpf = fmt.Fprintf

func sayerr(args ...interface{}) {
	msg := formatArgs(args...)
	fmt.Fprintf(os.Stderr, msg)
}

var usage string = `Usage: %s {src} {dst}
	- src is either 'stdin', 'example:NAME', or a filename
	- dst is either (stdout|xdot|yaml) or a filename

	e.g.:  'godecide example:hbr xdot' runs xdot with the hbr example 

	%s`

func main() {

	if len(os.Args) < 3 {
		sayerr(usage, os.Args[0], Examples())

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
		buf, err = Example(src)
		Ck(err)
	case src == "stdin":
		buf, err = ioutil.ReadAll(os.Stdin)
		Ck(err)
	default:
		buf, err = ioutil.ReadFile(src)
		Ck(err)
	}

	// parse
	roots, err := FromYAML(buf)
	Ck(err)

	// sum up Cash and Duration
	for _, root := range roots {
		root.Forward(nil)
	}

	// calculate Probable values
	for _, root := range roots {
		root.Backward()
	}

	/*
		debugbuf := &bytes.Buffer{}
		memviz.Map(debugbuf, &roots)
		fmt.Println(debugbuf.String())
		// err := ioutil.WriteFile("roots.dot", buf.Bytes(), 0644)
		// Ck(err)
	*/

	// show results
	dotbuf, err := Dot(roots)
	Ck(err)
	switch dst {
	case "stdout":
		fmt.Print(dotbuf.String())
	case "yaml":
		fmt.Print(string(buf))
	case "xdot":
		tmpfile, err := ioutil.TempFile("/tmp", "godecide.*.dot")
		Ck(err)
		defer os.Remove(tmpfile.Name())
		_, err = tmpfile.Write(dotbuf.Bytes())
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
		_, err = fh.Write(dotbuf.Bytes())
		Ck(err)
		err = fh.Close()
		Ck(err)
	}

}

type Node struct {
	Desc    string
	Cash    string
	Days    string
	Repeat  string
	FinRate float64
	ReRate  float64
	Paths   Paths `yaml:",omitempty"`
}

type Paths map[string]float64

type Nodes map[string]Node

type Stats struct {
	Duration time.Duration
	Cash     float64
	Npv      float64
	Mirr     float64
}

type Ast struct {
	Name     string
	Desc     string
	Repeat   int
	Prob     float64
	FinRate  float64
	ReRate   float64
	Period   Stats
	Node     Stats
	Path     Stats
	Expected Stats
	Start    time.Time
	End      time.Time
	Timeline fin.Timeline
	Parent   *Ast
	Children map[string]*Ast
}

func FromYAML(buf []byte) (roots []*Ast, err error) {
	defer Return(&err)
	var nodes Nodes
	err = yaml.Unmarshal(buf, &nodes)
	Ck(err)
	for name, _ := range nodes.RootNodes() {
		root := nodes.FromNode(name, 1, nil)
		roots = append(roots, root)
	}
	return
}

// RootNode returns a map of all root nodes.
func (nodes Nodes) RootNodes() (rootnodes Nodes) {
	rootnodes = make(Nodes)
	// accumulate all names
	for name, node := range nodes {
		rootnodes[name] = node
	}
	// remove children from list
	for _, parent := range nodes {
		for child, _ := range parent.Paths {
			delete(rootnodes, child)
		}
	}
	return
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

func dieif(cond bool, args ...interface{}) {
	if cond == false {
		return
	}
	var msg string
	if len(args) == 1 {
		msg = fmt.Sprintf("%v", args[0])
	}
	if len(args) > 1 {
		msg = fmt.Sprintf(args[0].(string), args[1:]...)
	}
	fmt.Fprintf(os.Stderr, "%v\n", msg)
	os.Exit(1)
}

func (nodes Nodes) FromNode(name string, prob float64, parent *Ast) (root *Ast) {
	node, ok := nodes[name]
	dieif(!ok, "missing node: %s", name)

	cashrat, err := mathcat.Eval(node.Cash)
	dieif(err != nil, err)
	cash, _ := cashrat.Float64()

	daysrat, err := mathcat.Eval(node.Days)
	dieif(err != nil, err)
	days, _ := daysrat.Float64()

	repeatrat, err := mathcat.Eval(node.Repeat)
	dieif(err != nil, err)
	dieif(!(repeatrat.IsInt() && repeatrat.Denom().Int64() == 1), "repeat must evaluate to int: %s", node)
	repeat := int(repeatrat.Num().Int64())
	repeat = int(math.Max(1, float64(repeat)))

	root = &Ast{
		Name: name,
		Desc: node.Desc,
		Period: Stats{
			Cash:     cash,
			Duration: time.Duration(days) * 24 * time.Hour,
		},
		Repeat:  repeat,
		FinRate: node.FinRate,
		ReRate:  node.ReRate,
		Prob:    prob,
		Parent:  parent,
	}
	root.Node.Cash = root.Period.Cash * float64(root.Repeat)
	root.Node.Duration = root.Period.Duration * time.Duration(root.Repeat)

	root.Children = make(map[string]*Ast)
	for childname, prob := range node.Paths {
		root.Children[childname] = nodes.FromNode(childname, prob, root)
	}
	return
}

// calculate .Path.*
func (this *Ast) Forward(parent *Ast) {
	if parent != nil {
		this.Timeline = parent.Timeline
		this.Path.Cash = parent.Path.Cash
		this.Path.Duration = parent.Path.Duration
	}

	this.Start = now.Add(this.Path.Duration)
	this.Path.Cash += this.Node.Cash
	this.Path.Duration += this.Node.Duration
	this.End = now.Add(this.Path.Duration)

	if this.FinRate != 0 {
		this.Timeline.SetFinRate(this.Start, this.FinRate)
	}
	if this.ReRate != 0 {
		this.Timeline.SetReRate(this.Start, this.ReRate)
	}
	for i := 1; i <= this.Repeat; i++ {
		var date time.Time
		date = this.Start.Add(time.Duration(i) * this.Period.Duration)
		this.Timeline.Event(date, this.Period.Cash)
	}
	this.Timeline.Recalc()
	this.Path.Npv = this.Timeline.Npv()
	this.Path.Mirr = this.Timeline.Mirr()

	/*
		Pl(this.Name)
		for _, t := range this.Timeline.Events() {
			Pf("%v %v\n", t.Date, t.Cash)
		}
		Pf("%.2f\n", this.Path.Mirr)
		Pl()
	*/

	for _, child := range this.Children {
		child.Forward(this)
	}
}

// calculate .Expected.*
func (this *Ast) Backward() {
	if len(this.Children) > 0 {
		// root or inner node
		totalProb := 0.0
		for _, child := range this.Children {
			totalProb += child.Prob
		}
		if math.Abs(totalProb-1) > .001 {
			fmt.Fprintf(os.Stderr, "normalizing path probabilities: %s\n", this.Name)
			for _, child := range this.Children {
				child.Prob /= totalProb
			}
		}
		for _, child := range this.Children {
			child.Backward()
			this.Expected.Cash += child.Expected.Cash * child.Prob
			this.Expected.Duration += time.Duration(float64(child.Expected.Duration) * child.Prob)
			this.Expected.Npv += child.Expected.Npv * child.Prob
			this.Expected.Mirr += child.Expected.Mirr * child.Prob
		}
	} else {
		// leaf -- we fold the path stuff back into expected here, and only here
		this.Expected.Cash = this.Path.Cash
		this.Expected.Duration = this.Path.Duration
		this.Expected.Npv = this.Path.Npv
		this.Expected.Mirr = this.Path.Mirr
	}
}

func Dot(roots []*Ast) (buf bytes.Buffer, err error) {
	g := graphviz.New()
	graph, err := g.Graph()
	Ck(err)
	defer func() {
		err := graph.Close()
		Ck(err)
		g.Close()
	}()

	graph.SetRankDir("LR")

	for _, root := range roots {
		root.Dot(graph)
	}

	err = g.Render(graph, "dot", &buf)
	Ck(err)
	return
}

func form(n float64) string {
	res := humanize.Comma(int64(n))
	return res
}

func days(d time.Duration) string {
	return Spf("%.0f days", float64(d)/float64(24*time.Hour))
}

// Dot adds a record-shaped node to the graphviz graph for Ast node p
// (parent), and adds a graphviz edge from p to each of the children
// of p.
func (a *Ast) Dot(graph *cgraph.Graph) (gvparent *cgraph.Node, err error) {
	defer Return(&err)
	gvparent, err = graph.CreateNode(Spf("%p", a))
	gvparent.SetShape("record")
	Ck(err)

	dates := Spf("%s - %s", a.Start.Format("2006-01-02"), a.End.Format("2006-01-02"))
	// row headings
	head := "             |cash|duration|npv    |mirr  "

	// columns
	n := a.Node
	p := a.Path
	e := a.Expected
	node := Spf("node     | %s | %s     | %s    |      ", form(n.Cash), days(n.Duration), form(n.Npv))
	path := Spf("path     | %s | %s     | %s    |%.1f%% ", form(p.Cash), days(p.Duration), form(p.Npv), p.Mirr)
	expe := Spf("expected | %s | %s     | %s    |%.1f%% ", form(e.Cash), days(e.Duration), form(e.Npv), e.Mirr)

	// put it all together
	label := Spf("%s | %s | %s | { {%s} | {%s} | {%s} | {%s}}", a.Name, a.Desc, dates, head, node, path, expe)

	gvparent.SetLabel(label)
	for _, child := range a.Children {
		// XXX should first verify that child exists in node list so
		// we don't silently create a zero-filled node
		gvchild, err := child.Dot(graph)
		Ck(err)
		edge, err := graph.CreateEdge("", gvparent, gvchild)
		Ck(err)
		edge.SetLabel(Spf("%.2f", child.Prob))
	}
	return
}

func Example(src string) (buf []byte, err error) {
	defer Return(&err)
	parts := strings.Split(src, ":")
	Assert(len(parts) == 2, "invalid example name: %s", src)
	name := parts[1]
	buf, err = fs.ReadFile(Spf("examples/%s.yaml", name))
	Ck(err)
	return
}

func Examples() (out string) {
	files, err := ioutil.ReadDir("examples")
	if err != nil || len(files) == 0 {
		return
	}
	out = "Examples available:\n\n"
	re := regexp.MustCompile(`(.*)\.yaml`)
	for _, f := range files {
		fn := f.Name()
		m := re.FindStringSubmatch(fn)
		if len(m) < 2 {
			continue
		}
		name := m[1]
		buf, err := ioutil.ReadFile(Spf("examples/%s", fn))
		Ck(err)
		lines := strings.Split(string(buf), "\n")
		var desc string
		if len(lines) > 0 && strings.HasPrefix(lines[0], "#") {
			desc = lines[0]
		}
		out += Spf("\t\texample:%s\t%s\n", name, desc)
	}
	return
}
