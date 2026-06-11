package report

import (
	"fmt"
	"math"
	"time"

	"github.com/gophish/gophish/models"
)

// FunnelInput holds cumulative, unique-recipient "reached at least this stage"
// counts. Invariant: Sent >= Opened >= Clicked >= Submitted. Both report
// sources normalize their (differently-shaped) raw data into this canonical
// form before Funnel derives the disjoint buckets used by the results table.
type FunnelInput struct {
	TotalRecipients int64
	Sent            int64
	Opened          int64
	Clicked         int64
	Submitted       int64
	Errored         int64
	Reported        int64
}

// FunnelMetrics holds the mutually-exclusive buckets of the results table
// (Tabla 1) plus the submitted-data percentage used in the conclusions.
type FunnelMetrics struct {
	Total    int64 `json:"total"`    // emails sent = Ignorado+Abierto+Clic+Datos
	Ignorado int64 `json:"ignorado"` // sent, not opened
	Abierto  int64 `json:"abierto"`  // opened, not clicked
	Clic     int64 `json:"clic"`     // clicked, not submitted
	Datos    int64 `json:"datos"`    // submitted data
	Error    int64 `json:"error"`    // errored (informational; not in table total)
	PctDatos int   `json:"pct_datos"`
}

// Funnel converts cumulative funnel counts into the disjoint buckets of the
// results table. Reproduces the reference document exactly: for sent=17,
// opened=16, clicked=16, submitted=2 it yields 1/0/14/2, total 17, 12%.
func Funnel(in FunnelInput) FunnelMetrics {
	m := FunnelMetrics{
		Total:    in.Sent,
		Ignorado: in.Sent - in.Opened,
		Abierto:  in.Opened - in.Clicked,
		Clic:     in.Clicked - in.Submitted,
		Datos:    in.Submitted,
		Error:    in.Errored,
	}
	if m.Total > 0 {
		m.PctDatos = int(math.Round(float64(m.Datos) / float64(m.Total) * 100))
	}
	return m
}

// rank orders a result/journey status by how far it advanced in the funnel. It
// delegates to models.FunnelRank so the report, the campaign-group stats and the
// dashboard share ONE canonical per-recipient classification.
func rank(status string) int { return models.FunnelRank(status) }

// fromCampaignStats maps an individual campaign's stats (already cumulative,
// computed by models.getCampaignStats) onto the canonical FunnelInput.
func fromCampaignStats(s models.CampaignStats) FunnelInput {
	return FunnelInput{
		TotalRecipients: s.Total,
		Sent:            s.EmailsSent,
		Opened:          s.OpenedEmail,
		Clicked:         s.ClickedLink,
		Submitted:       s.SubmittedData,
		Errored:         s.Error,
		Reported:        s.EmailReported,
	}
}

// funnelInputFromJourneys derives a cumulative, unique-recipient FunnelInput
// from a campaign group's per-recipient journeys.
//
// ARCHITECTURAL DECISION (do not regress): the group funnel is derived from
// RecipientJourneys, NOT from the aggregate counters of GetCampaignGroupStats.
// Those counters are per-event and not per-recipient (a single recipient can
// raise several "Email Opened" events, and counting differs from the cumulative
// per-result accounting used for individual campaigns in getCampaignStats).
// The report must represent the FINAL STATE PER RECIPIENT, not the raw number
// of events generated, so each recipient is classified once by the furthest
// stage reached across all campaigns of the group. This keeps the group and the
// single-campaign funnels semantically identical. See report/README.md.
func funnelInputFromJourneys(journeys []models.RecipientJourney) FunnelInput {
	var sentOnly, openedOnly, clickedOnly, submitted, errored int64
	for _, j := range journeys {
		best := 0
		hadError := false
		for _, cr := range j.CampaignResults {
			if cr.Status == models.Error {
				hadError = true
			}
			if r := rank(cr.Status); r > best {
				best = r
			}
		}
		switch best {
		case 4:
			submitted++
		case 3:
			clickedOnly++
		case 2:
			openedOnly++
		case 1:
			sentOnly++
		default:
			if hadError {
				errored++
			}
		}
	}
	clicked := submitted + clickedOnly
	opened := clicked + openedOnly
	sent := opened + sentOnly
	return FunnelInput{
		TotalRecipients: int64(len(journeys)),
		Sent:            sent,
		Opened:          opened,
		Clicked:         clicked,
		Submitted:       submitted,
		Errored:         errored,
	}
}

var mesesES = [12]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// fechaES formats a date as "2 de febrero del 2026".
func fechaES(t time.Time) string {
	return fmt.Sprintf("%d de %s del %d", t.Day(), mesesES[int(t.Month())-1], t.Year())
}

// rangoFechasES formats an execution window. When start and end fall in the
// same month and year it produces the document's compact form
// "del 02 al 06 de febrero del año 2026"; otherwise a full from/to range.
func rangoFechasES(start, end time.Time) string {
	if start.Year() == end.Year() && start.Month() == end.Month() {
		return fmt.Sprintf("del %02d al %02d de %s del año %d",
			start.Day(), end.Day(), mesesES[int(start.Month())-1], start.Year())
	}
	return fmt.Sprintf("del %s al %s", fechaES(start), fechaES(end))
}
