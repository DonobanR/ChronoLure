package report

import (
	"testing"

	"github.com/gophish/gophish/models"
)

// TestFunnelDocumentDataset reproduces the exact numbers of the reference
// document's results table: sent 17, opened(cum) 16, clicked(cum) 16,
// submitted 2 -> Ignorado 1, Abierto 0, Clic 14, Datos 2, Total 17, 12%.
func TestFunnelDocumentDataset(t *testing.T) {
	got := Funnel(FunnelInput{Sent: 17, Opened: 16, Clicked: 16, Submitted: 2})
	want := FunnelMetrics{Total: 17, Ignorado: 1, Abierto: 0, Clic: 14, Datos: 2, PctDatos: 12}
	if got != want {
		t.Fatalf("Funnel mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if got.Ignorado+got.Abierto+got.Clic+got.Datos != got.Total {
		t.Fatalf("buckets do not sum to total: %+v", got)
	}
}

func TestFunnelZeroSentNoDivByZero(t *testing.T) {
	got := Funnel(FunnelInput{})
	if got.Total != 0 || got.PctDatos != 0 {
		t.Fatalf("expected zeroed metrics, got %+v", got)
	}
}

func TestFromCampaignStatsIsCumulative(t *testing.T) {
	// CampaignStats are already cumulative; map straight through.
	in := fromCampaignStats(models.CampaignStats{
		Total: 20, EmailsSent: 17, OpenedEmail: 16, ClickedLink: 16,
		SubmittedData: 2, Error: 1, EmailReported: 3,
	})
	got := Funnel(in)
	if got.Total != 17 || got.Clic != 14 || got.Datos != 2 || got.PctDatos != 12 {
		t.Fatalf("unexpected funnel from campaign stats: %+v", got)
	}
}

// TestFunnelFromJourneys verifies the group path: each unique recipient is
// classified by the furthest stage reached across the campaigns of the group,
// even when their events are split across several campaigns.
func TestFunnelFromJourneys(t *testing.T) {
	j := func(statuses ...string) models.RecipientJourney {
		rj := models.RecipientJourney{Email: "x"}
		for _, s := range statuses {
			rj.CampaignResults = append(rj.CampaignResults, models.CampaignRecipientResult{Status: s})
		}
		return rj
	}
	journeys := []models.RecipientJourney{
		j(models.EventSent),                        // sent only      -> Ignorado
		j(models.EventOpened, models.EventSent),    // opened (max)   -> Abierto
		j(models.EventClicked),                     // clicked        -> Clic
		j(models.EventSent, models.EventDataSubmit), // submitted(max) -> Datos
		j(models.Error),                            // errored        -> not in funnel
	}
	got := Funnel(funnelInputFromJourneys(journeys))
	want := FunnelMetrics{Total: 4, Ignorado: 1, Abierto: 1, Clic: 1, Datos: 1, Error: 1, PctDatos: 25}
	if got != want {
		t.Fatalf("group funnel mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestRangoFechasESSameMonth(t *testing.T) {
	got := rangoFechasES(date(2026, 2, 2), date(2026, 2, 6))
	want := "del 02 al 06 de febrero del año 2026"
	if got != want {
		t.Fatalf("rangoFechasES = %q, want %q", got, want)
	}
}
