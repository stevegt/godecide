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
	Edges    []*Edge
}

type Edge struct {
	Prob  float64
	Child *Ast
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
	for name := range nodes.RootNodes() {
		root := nodes.toAst(name, nil)
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
		for child := range parent.Paths {
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

func (nodes Nodes) toAst(name string, parent *Ast) (nodeAst *Ast) {
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

	nodeAst = &Ast{
		Name:   name,
		Desc:   node.Desc,
		Repeat: repeat,
		Period: Stats{
			Cash:     cash,
			Duration: time.Duration(days) * 24 * time.Hour,
		},
		Due:     node.Due,
		FinRate: node.FinRate,
		ReRate:  node.ReRate,
		Parent:  parent,
	}
	nodeAst.Node.Cash = nodeAst.Period.Cash * float64(nodeAst.Repeat)
	nodeAst.Node.Duration = nodeAst.Period.Duration * time.Duration(nodeAst.Repeat)

	nodeAst.Edges = make([]*Edge, 0)
	// Build edges for each child from the YAML Paths map.
	for childname, childProb := range node.Paths {
		childAst := nodes.toAst(childname, nodeAst)
		nodeAst.Edges = append(nodeAst.Edges, &Edge{
			Prob:  childProb,
			Child: childAst,
		})
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

	for _, edge := range this.Edges {
		edge.Child.Forward(this, now, warn)
	}
}

// calculate .Expected.*
func (this *Ast) Backward(warn Warn) {
	if len(this.Edges) > 0 {
		// root or inner node
		totalProb := 0.0
		for _, edge := range this.Edges {
			totalProb += edge.Prob
		}
		if math.Abs(totalProb-1) > .001 {
			warn("normalizing path probabilities: %s\n", this.Name)
			for _, edge := range this.Edges {
				edge.Prob /= totalProb
			}
		}
		for _, edge := range this.Edges {
			edge.Child.Backward(warn)
			this.Expected.Cash += edge.Child.Expected.Cash * edge.Prob
			this.Expected.Duration += time.Duration(float64(edge.Child.Expected.Duration) * edge.Prob)
			this.Expected.Npv += edge.Child.Expected.Npv * edge.Prob
			this.Expected.Mirr += edge.Child.Expected.Mirr * edge.Prob
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

// Dot adds a record-shaped node to the graphviz graph for Ast node a
// (parent), and adds a graphviz edge from a to each of the children
// of a.
func (a *Ast) Dot(graph *cgraph.Graph, loMirr, hiMirr float64, warn Warn) (gvparent *cgraph.Node, err error) {
	defer Return(&err)

	gvparent, err = graph.CreateNode(Spf("%p", a))
	Ck(err)

	gvparent.SetShape("record")
	gvparent.SetStyle("filled")
	mirr := a.Expected.Mirr
	var hue, value float64
	if math.IsNaN(mirr) || math.IsInf(mirr, 0) {
		gvparent.SetFillColor("white")
	} else {
		switch {
		case math.IsNaN(mirr):
			warn("NaN mirr %f\n", mirr)
			hue = 0
			value = 0.4
		case mirr < loMirr:
			fallthrough
		case mirr > hiMirr:
			warn("lomirr %f mirr %f himirr %f\n", loMirr, mirr, hiMirr)
			hue = 0
			value = 0.4
		case mirr > 0:
			hue = 20/360.0 + 100/360.0*(mirr-0)/(hiMirr-0)
			value = math.Max(0.4, mirr/hiMirr)
		default:
			hue = 60 / 360.0 * (mirr - loMirr) / math.Abs(loMirr)
			value = math.Max(0.4, (0-mirr)/(0-loMirr))
		}
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

	// Prepare dynamic table columns: cash, duration, npv, mirr.
	n := a.Node
	p := a.Path
	e := a.Expected

	// Determine which metrics have non-zero (or non-NaN for mirr) values.
	includeCash := !(n.Cash == 0 && p.Cash == 0 && e.Cash == 0)
	includeDuration := !(n.Duration == 0 && p.Duration == 0 && e.Duration == 0)
	includeNpv := !(n.Npv == 0 && p.Npv == 0 && e.Npv == 0)
	includeMirr := true
	// For mirr, note that the node row is always blank so we check past and future.
	if (math.IsNaN(p.Mirr) || p.Mirr == 0) && (math.IsNaN(e.Mirr) || e.Mirr == 0) {
		includeMirr = false
	}

	// Build header and rows based on the metrics to include.
	var headers []string
	var nodeFields []string
	var pastFields []string
	var futureFields []string

	// top left cell is always blank
	headers = append(headers, "")

	if includeCash {
		headers = append(headers, "cash")
		nodeFields = append(nodeFields, form(n.Cash))
		pastFields = append(pastFields, form(p.Cash))
		futureFields = append(futureFields, form(e.Cash))
	}
	if includeDuration {
		headers = append(headers, "duration")
		nodeFields = append(nodeFields, days(n.Duration))
		pastFields = append(pastFields, days(p.Duration))
		futureFields = append(futureFields, days(e.Duration))
	}
	if includeNpv {
		headers = append(headers, "npv")
		nodeFields = append(nodeFields, form(n.Npv))
		pastFields = append(pastFields, form(p.Npv))
		futureFields = append(futureFields, form(e.Npv))
	}
	if includeMirr {
		headers = append(headers, "mirr")
		// No mirr in node row.
		nodeFields = append(nodeFields, "")
		pastFields = append(pastFields, Spf("%.1f%%", p.Mirr))
		futureFields = append(futureFields, Spf("%.1f%%", e.Mirr))
	}

	// Create label parts.
	headerRow := strings.Join(headers, "|")
	nodeRow := Spf("node     | %s", strings.Join(nodeFields, " | "))
	pastRow := Spf("past     | %s", strings.Join(pastFields, " | "))
	futureRow := Spf("future   | %s", strings.Join(futureFields, " | "))

	// put it all together
	label := Spf("%s \\n %s \\n %s | { {%s} | {%s} | {%s} | {%s}}", a.Name, a.Desc, dates, headerRow, nodeRow, pastRow, futureRow)
	gvparent.SetLabel(label)

	// Determine the critical child based on the longest path (in days)
	var criticalChild *Ast
	var maxExtra time.Duration
	for _, edge := range a.Edges {
		child := edge.Child
		// extra duration from this node to the end of child's path
		extra := child.Path.Duration - a.Path.Duration
		if extra > maxExtra {
			maxExtra = extra
			criticalChild = child
		}
	}

	// Create edges for each child, coloring the edge red if it is the critical path
	for _, edge := range a.Edges {
		gvchild, err := edge.Child.Dot(graph, loMirr, hiMirr, warn)
		Ck(err)
		gvedge, err := graph.CreateEdge("", gvparent, gvchild)
		Ck(err)
		gvedge.SetLabel(Spf("%.2f", edge.Prob))
		gvedge.SetPenWidth(math.Pow(edge.Prob+0.1, 0.5) * 10)
		if edge.Child == criticalChild {
			gvedge.SetColor("red")
		}
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
		for _, edge := range a.Edges {
			children = append(children, edge.Child)
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
