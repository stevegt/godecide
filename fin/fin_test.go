package fin

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"testing"
	"time"

	"github.com/rickb777/date/period"
	. "github.com/stevegt/goadapt"
	"gopkg.in/yaml.v2"
)

type TcEvent struct {
	T       period.Period
	Cash    float64
	FinRate float64
	ReRate  float64
}

type TestCase struct {
	Npv    float64
	Mirr   float64
	Start  time.Time
	Events []TcEvent
}

func (tc *TestCase) toTl() (tl *Timeline) {
	tl = &Timeline{}
	for _, e := range tc.Events {
		t, _ := e.T.AddTo(tc.Start)
		if e.FinRate != 0 {
			tl.SetFinRate(t, e.FinRate)
		}
		if e.ReRate != 0 {
			tl.SetReRate(t, e.ReRate)
		}
		tl.Event(t, e.Cash)
		// Pl(e.T, t, e.Cash, tl.Start)
	}
	return
}

func dump(v interface{}) {
	buf, err := json.MarshalIndent(v, "   ", "")
	Ck(err)
	Pl(string(buf))
}

func TestAll(t *testing.T) {

	files, err := ioutil.ReadDir("testdata")
	Ck(err)
	for _, f := range files {
		fn := f.Name()
		buf, err := ioutil.ReadFile(path.Join("testdata", fn))
		Ck(err)

		var tc TestCase
		err = yaml.Unmarshal(buf, &tc)
		Ck(err)

		t.Run(fn, func(t *testing.T) {
			tl := tc.toTl()
			tl.Recalc()
			// dump(tl.events)
			// Pf("%#v\n", tl)
			Tassert(t, tl.Npv() == tc.Npv, "NPV got %v want %v", tl.Npv(), tc.Npv)
			Tassert(t, tl.Mirr() == tc.Mirr, "MIRR got %v want %v", tl.Mirr(), tc.Mirr)
		})
	}
}
