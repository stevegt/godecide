package tree

import (
	"bytes"
	"embed"
	"fmt"
	"math"
	"os"
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

type Warn func(args ...interface{})

type Node struct {
	Desc    string
	Cash    string
	Days    string
	Repeat  string
	FinRate float64
	ReRate  float64
	Due     time.Time
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
	Due      time.Time
	Timeline fin.Timeline
	Parent   *Ast
	Children map[string]*Ast
}

func FromYAML(buf []byte) (roots []*Ast, err error) {
	defer Return(&err)
	var nodes Nodes
	err = yaml.Unmarshal(buf, &nodes)
	Ck(err)
	roots = nodes.ToAst()
	return
}

func (nodes Nodes) ToAst() (roots []*Ast) {
	for name, _ := range nodes.RootNodes() {
		root := nodes.toAst(name, 1, nil)
		roots = append(roots, root)
	}
	return
}

func (nodes Nodes) ToYAML() (buf []byte, err error) {
	defer Return(&err)
	buf, err = yaml.Marshal(&nodes)
	Ck(err)
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

func (nodes Nodes) toAst(name string, prob float64, parent *Ast) (root *Ast) {
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
		Due:     node.Due,
		FinRate: node.FinRate,
		ReRate:  node.ReRate,
		Prob:    prob,
		Parent:  parent,
	}
	root.Node.Cash = root.Period.Cash * float64(root.Repeat)
	root.Node.Duration = root.Period.Duration * time.Duration(root.Repeat)

	root.Children = make(map[string]*Ast)
	for childname, prob := range node.Paths {
		root.Children[childname] = nodes.toAst(childname, prob, root)
	}
	return
}

// calculate .Path.*
func (this *Ast) Forward(parent *Ast, now time.Time, warn Warn) {
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
	if !this.Due.IsZero() && this.End.After(this.Due) {
		warn("late: %s end %s due %s\n", this.Name, this.End, this.Due)
		this.Expected.Mirr = math.NaN()
	}

	/*
		Pl(this.Name)
		for _, t := range this.Timeline.Events() {
			Pf("%v %v\n", t.Date, t.Cash)
		}
		Pf("%.2f\n", this.Path.Mirr)
		Pl()
	*/

	for _, child := range this.Children {
		child.Forward(this, now, warn)
	}
}

// calculate .Expected.*
func (this *Ast) Backward(warn Warn) {
	if len(this.Children) > 0 {
		// root or inner node
		totalProb := 0.0
		for _, child := range this.Children {
			totalProb += child.Prob
		}
		if math.Abs(totalProb-1) > .001 {
			warn("normalizing path probabilities: %s\n", this.Name)
			for _, child := range this.Children {
				child.Prob /= totalProb
			}
		}
		for _, child := range this.Children {
			child.Backward(warn)
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
func (a *Ast) Dot(graph *cgraph.Graph, loMirr, hiMirr float64, warn Warn) (gvparent *cgraph.Node, err error) {
	defer Return(&err)
	gvparent, err = graph.CreateNode(Spf("%p", a))
	Ck(err)

	gvparent.SetShape("record")

	gvparent.SetStyle("filled")
	mirr := a.Expected.Mirr
	Assert(mirr >= loMirr, mirr)
	Assert(mirr <= hiMirr, mirr)
	var hue, value float64
	if math.IsNaN(mirr) || math.IsInf(mirr, 0) {
		gvparent.SetFillColor("white")
	} else {
		// hue := 120 / 360.0 * (mirr - loMirr) / float64(hiMirr-loMirr)
		if mirr > 0 {
			hue = 20/360.0 + 100/360.0*(mirr-0)/(hiMirr-0)
			value = math.Max(0.4, mirr/hiMirr)
		} else {
			hue = 60 / 360.0 * (mirr - loMirr) / math.Abs(loMirr)
			value = math.Max(0.4, (0-mirr)/(0-loMirr))
		}
		// Pl(hue, mirr, loMirr, hiMirr, (mirr - loMirr), float64(hiMirr-loMirr))
		if math.IsNaN(hue) {
			warn("hue NaN %f %f %f\n", loMirr, hiMirr, mirr)
			hue = 0
		}
		color := Spf("%.3f 1.0 %.3f", hue, value)
		gvparent.SetFillColor(color)
	}

	dates := Spf("%s - %s", a.Start.Format("2006-01-02"), a.End.Format("2006-01-02"))
	if !a.Due.IsZero() {
		dates = Spf("%s \\n due: %s", dates, a.Due.Format("2006-01-02"))
		if a.End.After(a.Due) {
			color := Spf("%.3f 1.0 1.0", hue)
			gvparent.SetFontColor(color)
			gvparent.SetFillColor("0.0 0.0 0.3")
		}
	}
	// row headings
	head := "             |cash|duration|npv    |mirr  "

	// columns
	n := a.Node
	p := a.Path
	e := a.Expected
	node := Spf("node     | %s | %s     | %s    |      ", form(n.Cash), days(n.Duration), form(n.Npv))
	path := Spf("past     | %s | %s     | %s    |%.1f%% ", form(p.Cash), days(p.Duration), form(p.Npv), p.Mirr)
	expe := Spf("future   | %s | %s     | %s    |%.1f%% ", form(e.Cash), days(e.Duration), form(e.Npv), e.Mirr)

	// put it all together
	label := Spf("%s \\n %s \\n %s | { {%s} | {%s} | {%s} | {%s}}", a.Name, a.Desc, dates, head, node, path, expe)
	gvparent.SetLabel(label)
	for _, child := range a.Children {
		gvchild, err := child.Dot(graph, loMirr, hiMirr, warn)
		Ck(err)
		edge, err := graph.CreateEdge("", gvparent, gvchild)
		Ck(err)
		edge.SetLabel(Spf("%.2f", child.Prob))
		edge.SetPenWidth(math.Pow(child.Prob+.1, .5) * 10)
	}
	return
}

func getMirrs(as []*Ast) (lo, hi float64) {
	lo = math.MaxFloat64
	hi = math.MaxFloat64 * -1
	Assert(!math.IsInf(lo, 0), lo)
	Assert(!math.IsInf(hi, 0), hi)
	for _, a := range as {
		var children []*Ast
		for _, c := range a.Children {
			children = append(children, c)
		}
		clo, chi := getMirrs(children)
		if !math.IsNaN(clo) {
			lo = math.Min(lo, clo)
		}
		if !math.IsNaN(chi) {
			hi = math.Max(hi, chi)
		}
		mirr := a.Expected.Mirr
		if !math.IsNaN(mirr) && !math.IsInf(mirr, 0) {
			lo = math.Min(lo, mirr)
			hi = math.Max(hi, mirr)
		}
	}
	Assert(!math.IsInf(lo, 0), lo)
	Assert(!math.IsInf(hi, 0), hi)
	Assert(!math.IsNaN(lo), lo)
	Assert(!math.IsNaN(hi), hi)
	return
}

func Recalc(roots []*Ast, now time.Time, warn Warn) {

	// sum up Cash and Duration
	for _, root := range roots {
		root.Forward(nil, now, warn)
	}

	// calculate expected values
	for _, root := range roots {
		root.Backward(warn)
	}
}

func ToDot(roots []*Ast, warn Warn, tb bool) (buf []byte) {

	loMirr, hiMirr := getMirrs(roots)
	// Pl(loMirr, hiMirr)

	g := graphviz.New()
	graph, err := g.Graph()
	Ck(err)
	defer func() {
		err := graph.Close()
		Ck(err)
		g.Close()
	}()

	if tb {
		graph.SetRankDir("TB")
	} else {
		graph.SetRankDir("LR")
	}

	for _, root := range roots {
		root.Dot(graph, loMirr, hiMirr, warn)
	}

	var dotbuf bytes.Buffer
	err = g.Render(graph, "dot", &dotbuf)
	Ck(err)

	buf = dotbuf.Bytes()
	return
}

func CatExample(fs embed.FS, src string) (buf []byte, err error) {
	defer Return(&err)
	parts := strings.Split(src, ":")
	Assert(len(parts) == 2, "invalid example name: %s", src)
	name := parts[1]
	buf, err = fs.ReadFile(Spf("examples/%s.yaml", name))
	Ck(err)
	return
}

func LsExamples(fs embed.FS) (out string) {
	files, err := fs.ReadDir("examples")
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
		buf, err := fs.ReadFile(Spf("examples/%s", fn))
		Ck(err)
		lines := strings.Split(string(buf), "\n")
		var desc string
		if len(lines) > 0 && strings.HasPrefix(lines[0], "#") {
			desc = lines[0]
		}
		out += Spf("\t\texample:%-15s%s\n", name, desc)
	}
	return
}
