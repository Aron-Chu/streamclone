package analytics

import (
	"strings"
	"testing"
)

func TestFetchVODCommentsRejectsSerialWhenGoldSegmentsEnabled(t *testing.T) {
	svc := &SyncService{
		goldVODSegmentsEnabled: true,
		vodGQLConcurrency:      1,
	}
	err := svc.fetchVODComments(
		t.Context(),
		"stream-serial-1",
		"xqc",
		"vod-serial-1",
		map[int][]string{},
		600,
		0,
		nil,
		nil,
		gqlFetchScheduleHints{},
	)
	if err == nil {
		t.Fatal("expected serial guard error")
	}
	if !strings.Contains(err.Error(), "gold vod segments enabled requires VOD GQL concurrency > 1") {
		t.Fatalf("error = %q", err.Error())
	}
}
