package archive

import "testing"

func TestComputeSilverCoverage(t *testing.T) {
	rollups := []RollupExportLine{
		{ViewerSamples: 1}, {ViewerSamples: 1}, {ViewerSamples: 0}, {ViewerSamples: 0},
	}
	ratio, status := ComputeSilverCoverage(rollups, 4, 0.5)
	if ratio != 0.5 {
		t.Fatalf("ratio = %v want 0.5", ratio)
	}
	if status != StatusConfirmed {
		t.Fatalf("status = %q want confirmed", status)
	}
	_, partial := ComputeSilverCoverage(rollups[:1], 4, 0.5)
	if partial != StatusPartial {
		t.Fatalf("expected partial, got %q", partial)
	}
}
