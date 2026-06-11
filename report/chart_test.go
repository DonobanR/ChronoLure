package report

import (
	"bytes"
	"testing"
)

// pngMagic is the 8-byte PNG signature.
var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

func TestChartPNGDocumentDataset(t *testing.T) {
	m := Funnel(FunnelInput{Sent: 17, Opened: 16, Clicked: 16, Submitted: 2})
	png, err := ChartPNG(m)
	if err != nil {
		t.Fatalf("ChartPNG: %v", err)
	}
	if len(png) == 0 || !bytes.HasPrefix(png, pngMagic) {
		t.Fatalf("output is not a PNG (len=%d)", len(png))
	}
}

func TestChartPNGAllZero(t *testing.T) {
	png, err := ChartPNG(FunnelMetrics{})
	if err != nil {
		t.Fatalf("ChartPNG all-zero must not error: %v", err)
	}
	if !bytes.HasPrefix(png, pngMagic) {
		t.Fatalf("all-zero output is not a PNG")
	}
}
