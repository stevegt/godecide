package fin

import (
	"math"
	"time"
	// . "github.com/stevegt/goadapt"
)

var DaysPerYear float64 = 365.2425

type Event struct {
	Date         time.Time
	T            time.Duration // since Timeline.start
	Cash         float64
	FinRate      float64 // finance rate
	ReRate       float64 // reinvestment rate
	YearsElapsed float64
	YearsLeft    float64
}

type Timeline struct {
	events []*Event
	Start  time.Time
	End    time.Time
	npv    float64
	pvneg  float64
	fvpos  float64
	mirr   float64
}

func (tl *Timeline) Npv() float64 {
	return tl.npv
}

func (tl *Timeline) Mirr() float64 {
	return tl.mirr * 100
}

func (tl *Timeline) SetFinRate(date time.Time, finrate float64) (e *Event) {
	_, rerate := tl.LastRates()
	e = &Event{Date: date, FinRate: finrate, ReRate: rerate}
	tl.events = append(tl.events, e)
	return
}

func (tl *Timeline) SetReRate(date time.Time, rerate float64) (e *Event) {
	finrate, _ := tl.LastRates()
	e = &Event{Date: date, FinRate: finrate, ReRate: rerate}
	tl.events = append(tl.events, e)
	return
}

func (tl *Timeline) LastRates() (finrate, rerate float64) {
	last := tl.Last()
	if last != nil {
		finrate = last.FinRate
		rerate = last.ReRate
	}
	return
}

func (tl *Timeline) Event(date time.Time, cash float64) (e *Event) {
	if tl.Start.IsZero() {
		tl.Start = date
	}
	if date.After(tl.End) {
		tl.End = date
	}
	t := tl.End.Sub(tl.Start)
	finrate, rerate := tl.LastRates()
	e = &Event{Date: date, Cash: cash, FinRate: finrate, ReRate: rerate, T: t}
	tl.events = append(tl.events, e)
	return
}

func (tl *Timeline) Events() (es []*Event) {
	for _, e := range tl.events {
		es = append(es, e)
	}
	return
}

func (tl *Timeline) Last() (e *Event) {
	if len(tl.events) > 0 {
		return tl.events[len(tl.events)-1]
	}
	return nil
}

func dur2years(dur time.Duration) float64 {
	return float64(dur) / (float64(time.Hour) * 24 * DaysPerYear)
}

func (tl *Timeline) Recalc() {
	tl.npv = 0
	tl.pvneg = 0
	tl.fvpos = 0
	tl.mirr = 0

	yearsTotal := dur2years(tl.End.Sub(tl.Start))

	for _, e := range tl.events {
		yearsElapsed := dur2years(e.T)
		yearsLeft := yearsTotal - yearsElapsed
		e.YearsElapsed = yearsElapsed
		e.YearsLeft = yearsLeft

		// https://en.wikipedia.org/wiki/Present_value
		pv := e.Cash / math.Pow(1+e.FinRate, yearsElapsed)
		// Pl(e.T, pv)
		tl.npv += pv

		if e.Cash < 0 {
			// https://en.wikipedia.org/wiki/Present_value
			tl.pvneg -= e.Cash / math.Pow(1+e.FinRate, yearsElapsed)
		} else {
			// https://en.wikipedia.org/wiki/Future_value
			// Pl(e.Cash, e.ReRate, yearsLeft)
			tl.fvpos += e.Cash * math.Pow(1+e.ReRate, yearsLeft)
		}

	}

	// https://en.wikipedia.org/wiki/Modified_internal_rate_of_return
	tl.mirr = math.Pow(tl.fvpos/tl.pvneg, 1/yearsTotal) - 1
}
