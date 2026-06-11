package report

import (
	"bytes"
	"fmt"

	chart "github.com/wcharczuk/go-chart/v2"
)

// ChartPNG renders "Gráfico 1: Resultados" as a PNG bar chart of the disjoint
// funnel buckets. This is the ONLY auto-generated artifact in the report; every
// other figure is supplied by the operator. The PNG is consumed by Render as
// the image for the grafico_1 slot, exactly like any other image swap.
func ChartPNG(m FunnelMetrics) ([]byte, error) {
	bars := []chart.Value{
		{Value: float64(m.Ignorado), Label: "Correo Ignorado"},
		{Value: float64(m.Abierto), Label: "Correo Abierto"},
		{Value: float64(m.Clic), Label: "Clic al Enlace"},
		{Value: float64(m.Datos), Label: "Envío de Datos"},
	}

	// go-chart cannot scale a bar chart whose values are all zero; fall back to
	// a unit axis so an empty campaign still produces a valid (flat) chart.
	max := m.Ignorado
	for _, v := range []int64{m.Abierto, m.Clic, m.Datos} {
		if v > max {
			max = v
		}
	}
	bc := chart.BarChart{
		Title:    "Resultados",
		Width:    900,
		Height:   450,
		BarWidth: 90,
		Bars:     bars,
	}
	if max <= 0 {
		bc.YAxis.Range = &chart.ContinuousRange{Min: 0, Max: 1}
	}

	var buf bytes.Buffer
	if err := bc.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("rendering results chart: %w", err)
	}
	return buf.Bytes(), nil
}
