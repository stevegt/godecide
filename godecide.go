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
	"time"

	"github.com/dustin/go-humanize"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/soudy/mathcat"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/goxirr"

	"gopkg.in/yaml.v2"
)

//go:embed examples/example.yaml
var fs embed.FS

var now = time.Now()

var Fpf = fmt.Fprintf

func main() {

	if len(os.Args) < 3 {
		Fpf(os.Stderr, "usage: %s {src} {dst}\n", os.Args[0])
		Fpf(os.Stderr, "- src is either (stdin|example) or a filename\n")
		Fpf(os.Stderr, "- dst is either (stdout|xdot|yaml) or a filename\n")
		os.Exit(1)
	}
	//  get subcommand
	src := os.Args[1]
	dst := os.Args[2]

	var buf []byte
	var err error

	// get input
	switch src {
	case "example":
		buf, err = Example()
		Ck(err)
	case "stdin":
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
	AtStart bool
	Paths   Paths `yaml:",omitempty"`
}

type Paths map[string]float64

type Nodes map[string]Node

type Ast struct {
	Name             string
	Desc             string
	PeriodCash       float64
	PeriodDuration   time.Duration
	Repeat           int64
	Prob             float64
	AtStart          bool
	NodeCash         float64
	NodeDuration     time.Duration
	NetCash          float64
	NetDuration      time.Duration
	DurationToDate   time.Duration
	ExpectedCash     float64
	ExpectedDuration time.Duration
	Start            time.Time
	End              time.Time
	Transactions     goxirr.Transactions
	// Irr              float64
	Return     float64
	NetPerYear float64
	Parent     *Ast
	Children   map[string]*Ast
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
	node, ok := nodes[name] // XXX this should blow up when child is missing
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
	repeat := repeatrat.Num().Int64()
	repeat = int64(math.Max(1, float64(repeat)))

	root = &Ast{
		Name:           name,
		Desc:           node.Desc,
		PeriodCash:     cash,
		PeriodDuration: time.Duration(days) * 24 * time.Hour,
		Repeat:         repeat,
		AtStart:        node.AtStart,
		Prob:           prob,
		Parent:         parent,
	}
	// XXX NPV
	root.NodeCash = root.PeriodCash * float64(root.Repeat)

	root.NodeDuration = root.PeriodDuration * time.Duration(root.Repeat)

	root.Children = make(map[string]*Ast)
	for childname, prob := range node.Paths {
		root.Children[childname] = nodes.FromNode(childname, prob, root)
	}
	return
}

// sum up Cash, Duration from root to leaves
func (this *Ast) Forward(parent *Ast) {
	if parent != nil {
		this.DurationToDate = parent.DurationToDate
	}

	this.Start = now.Add(this.DurationToDate)
	this.DurationToDate += this.NodeDuration
	this.End = now.Add(this.DurationToDate)

	for _, child := range this.Children {
		child.Forward(this)
	}
}

// calculate probable values
func (this *Ast) Backward() {
	if len(this.Children) > 0 {
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
			this.ExpectedCash += child.NetCash * child.Prob
			this.ExpectedDuration += time.Duration(float64(child.NetDuration) * child.Prob)
		}
	}
	this.NetCash = this.NodeCash + this.ExpectedCash
	this.NetDuration = this.NodeDuration + this.ExpectedDuration

	/*
		var t int64
		for t = 1; t <= this.Repeat; t++ {
			var date time.Time
			if this.AtStart {
				date = this.Start.Add(time.Duration(t-1) * this.PeriodDuration)
			} else {
				date = this.Start.Add(time.Duration(t) * this.PeriodDuration)
			}
			cash := goxirr.Transaction{
				Date: date,
				Cash: this.PeriodCash,
			}
			this.Transactions = append(this.Transactions, cash)
		}
		expect := goxirr.Transaction{
			Date: this.Start.Add(this.ExpectedDuration),
			Cash: this.ExpectedCash,
		}
		this.Transactions = append(this.Transactions, expect)
		this.Irr = goxirr.Xirr(this.Transactions)
		Pf("%s %.2f\n", this.Name, this.Irr)
		for _, t := range this.Transactions {
			Pf("%v %v\n", t.Date, t.Cash)
		}
		Pl()
	*/

	if this.NodeCash < 0 && this.ExpectedCash > 0 {
		years := float64(this.NetDuration / (365 * 24 * time.Hour))
		this.Return = (math.Pow(this.ExpectedCash/(-1*this.NodeCash), (1/years)) - 1) * 100
	} else {
		this.Return = math.NaN()
	}

	years := float64(this.NetDuration / (365 * 24 * time.Hour))
	this.NetPerYear = this.NetCash / years

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
func (p *Ast) Dot(graph *cgraph.Graph) (gvparent *cgraph.Node, err error) {
	defer Return(&err)
	gvparent, err = graph.CreateNode(Spf("%p", p))
	gvparent.SetShape("record")
	Ck(err)
	rowheads := "|cash|duration|return"
	this := Spf("node | %s | %s |", form(p.NodeCash), days(p.NodeDuration))
	dates := Spf("%s - %s", p.Start.Format("2006-01-02"), p.End.Format("2006-01-02"))
	expect := Spf("expected | %s | %s | ", form(p.ExpectedCash), days(p.ExpectedDuration))
	net := Spf("net | %s | %s | %.1f%%", form(p.NetCash), days(p.NetDuration), p.Return)
	label := Spf("%s | %s | %s | { {%s} | {%s} | {%s} | {%s}}", p.Name, p.Desc, dates, rowheads, this, expect, net)

	gvparent.SetLabel(label)
	for _, child := range p.Children {
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

func Example() (buf []byte, err error) {
	defer Return(&err)
	buf, err = fs.ReadFile("examples/example.yaml")
	Ck(err)
	return
}
