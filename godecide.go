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

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/goxirr"
	"gopkg.in/yaml.v2"
)

//go:embed example.yaml
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

	// sum up Cost, Yield, Duration
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
	Desc  string
	Cost  float64
	Yield float64
	Days  float64
	Paths Paths `yaml:",omitempty"`
}

type Paths map[string]float64

type Nodes map[string]Node

/*
func (nodes Nodes) Names() (names []string) {
	names := make([]string, len(nodes))
	i := 0
	for name := range nodes {
		names[i] = name
		i++
	}
}

func (nodes Nodes) Roots() (roots *List) {
	roots := list.New()
	for _, name := range nodes.Names() {
		roots.PushBack(name)
	}
	e := roots.Front()
	for parentname
		parentname := e.Value()
		parentnode := nodes[parentname]
		for childname, _ := range parentnode.Paths {
			if rootname == childname {
				// parent is closer to root
				rootname = parentname
			}
		}
	}
	return
}
*/

type Ast struct {
	Name             string
	Desc             string
	Cost             float64
	Yield            float64
	Duration         time.Duration
	Prob             float64
	TotalCost        float64
	TotalYield       float64
	TotalDuration    time.Duration
	ProbableCost     float64
	ProbableYield    float64
	ProbableDuration time.Duration
	Transactions     goxirr.Transactions
	Start            time.Time
	End              time.Time
	Irr              float64
	ProbableIrr      float64
	Parent           *Ast
	Children         map[string]*Ast
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

func (nodes Nodes) FromNode(name string, prob float64, parent *Ast) (root *Ast) {
	node := nodes[name]
	root = &Ast{
		Name:     name,
		Desc:     node.Desc,
		Cost:     node.Cost,
		Yield:    node.Yield,
		Duration: time.Duration(node.Days) * 24 * time.Hour,
		Prob:     prob,
		Parent:   parent,
	}
	root.Children = make(map[string]*Ast)
	for childname, prob := range node.Paths {
		root.Children[childname] = nodes.FromNode(childname, prob, root)
	}
	return
}

/*
func FromNodes(nodes Nodes) (root *Ast) {
	// start with a tentative top AST node
	root = &Ast{Name: "top"}
	// iterate over all input nodes
	for name, node := range nodes {
		// create ast node

		// recurse through children

		for childname, prob := range parentnode.Paths {

			_, ok := roots[childname]
			if ok {
				delete(roots, childname)
				continue
			}

			if rootname == childname {
				// parent is closer to root
				rootname = parentname
			}
		}
	}
	return
}
*/

// sum up Cost, Yield, Duration from root to leaves
func (this *Ast) Forward(parent *Ast) {
	if parent != nil {
		this.TotalCost = parent.TotalCost
		this.TotalYield = parent.TotalYield
		this.TotalDuration = parent.TotalDuration
		this.Transactions = parent.Transactions
	}

	this.Start = now.Add(this.TotalDuration)
	this.TotalCost += this.Cost
	this.TotalYield += this.Yield
	this.TotalDuration += this.Duration
	this.End = now.Add(this.TotalDuration)

	// assume costs are up front and yields are at end of duration
	cost := goxirr.Transaction{
		Date: this.Start,
		Cash: -this.Cost,
	}
	this.Transactions = append(this.Transactions, cost)
	yield := goxirr.Transaction{
		Date: this.End,
		Cash: this.Yield,
	}
	this.Transactions = append(this.Transactions, yield)
	// Pf("%#v\n", this.Transactions)
	this.Irr = goxirr.Xirr(this.Transactions)

	for _, child := range this.Children {
		child.Forward(this)
	}
}

// calculate probable values
func (parent *Ast) Backward() {
	if len(parent.Children) == 0 {
		// leaf
		parent.ProbableCost = parent.TotalCost
		parent.ProbableYield = parent.TotalYield
		parent.ProbableDuration = parent.TotalDuration
		parent.ProbableIrr = parent.Irr
	} else {
		totalProb := 0.0
		for _, child := range parent.Children {
			totalProb += child.Prob
		}
		if math.Abs(totalProb-1) > .001 {
			fmt.Fprintf(os.Stderr, "normalizing path probabilities: %s\n", parent.Name)
			for _, child := range parent.Children {
				child.Prob /= totalProb
			}
		}
		for _, child := range parent.Children {
			child.Backward()
			parent.ProbableCost += child.ProbableCost * child.Prob
			parent.ProbableYield += child.ProbableYield * child.Prob
			parent.ProbableDuration += child.ProbableDuration * time.Duration(child.Prob)
			parent.ProbableIrr += child.ProbableIrr * child.Prob
		}
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

func (p *Ast) Dot(graph *cgraph.Graph) (gvparent *cgraph.Node, err error) {
	defer Return(&err)
	gvparent, err = graph.CreateNode(Spf("%p", p))
	gvparent.SetShape("record")
	Ck(err)
	rowheads := "|cost|yield|irr"
	this := Spf("this | %.0f | %.0f | ", p.Cost, p.Yield)
	dates := Spf("%s - %s", p.Start.Format("2006-01-02"), p.End.Format("2006-01-02"))
	totals := Spf("to date | %.0f | %.0f | %.1f", p.TotalCost, p.TotalYield, p.Irr)
	probs := Spf("future | %.0f | %.0f |  %.1f", p.ProbableCost, p.ProbableYield, p.ProbableIrr)
	label := Spf("%s | %s | %s | { {%s} | {%s} | {%s} | {%s}}", p.Name, p.Desc, dates, rowheads, this, totals, probs)

	gvparent.SetLabel(label)
	for _, child := range p.Children {
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
	buf, err = fs.ReadFile("example.yaml")
	Ck(err)
	return
}
