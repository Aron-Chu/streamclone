package analytics

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func jsonFieldNames(t reflect.Type) []string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	names := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = strings.ToLower(field.Name[:1]) + field.Name[1:]
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func assertJSONFieldNames(t *testing.T, typ reflect.Type, golden []string) {
	t.Helper()
	got := jsonFieldNames(typ)
	want := append([]string(nil), golden...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("json field count mismatch for %s\ngot:  %v\nwant: %v", typ.Name(), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("json field mismatch for %s at index %d\ngot:  %v\nwant: %v", typ.Name(), i, got, want)
		}
	}
}

func TestContractExtensionCoverageJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(ExtensionCoverage{}), []string{
		"backfillReason",
		"canBackfill",
		"chatSource",
		"chatSourceDetail",
		"copyKey",
		"coverageEndOffsetSeconds",
		"coverageStartOffsetSeconds",
		"hasFullStreamCoverage",
		"hasGaps",
		"manualRetryAllowed",
		"message",
		"missingRanges",
		"state",
		"trackedFromStart",
		"vodStatus",
	})
}

func TestContractExtensionCoverageRangeJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(ExtensionCoverageRange{}), []string{
		"fromOffsetSeconds",
		"toOffsetSeconds",
	})
}

func TestContractClipCandidateJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(ClipCandidate{}), []string{
		"chatCount",
		"confidence",
		"confidenceBand",
		"coverageState",
		"createdAt",
		"emoteCount",
		"endSeconds",
		"id",
		"inboxState",
		"job",
		"login",
		"minuteTs",
		"offsetSeconds",
		"pickReason",
		"reason",
		"renderabilityStatus",
		"score",
		"signals",
		"sourceCheckedAt",
		"sourceKind",
		"sourceStatus",
		"startSeconds",
		"state",
		"statusCopy",
		"streamCategory",
		"streamId",
		"streamTitle",
		"startedAt",
		"topEmotes",
		"updatedAt",
		"viewerCount",
		"vodId",
	})
}

func TestContractPublicHubResponseJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(PublicHubResponse{}), []string{
		"activity",
		"corpus",
		"corpusPipeline",
		"coverage",
		"emoteIntel",
		"featuredSession",
		"generatedAt",
		"liveChannels",
		"livePulseMoments",
		"livePulseMomentsReason",
		"livePulseMomentsStatus",
		"moments",
		"poolSize",
		"topEmotes",
		"topMovers",
	})
}

func TestContractHubCorpusPipelineJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(HubCorpusPipeline{}), []string{
		"collectorActive",
		"collectorMax",
		"generatedAt",
		"gold",
		"liveAdmissionEnabled",
		"liveAdmissionTopN",
		"maxActiveIrcChannels",
		"metadataSampledAgoSeconds",
		"roster",
		"runtimeConfigFingerprint",
		"silver",
		"state",
		"topN",
	})
}

func TestContractPublicStatusResponseJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(PublicStatusResponse{}), []string{
		"api",
		"degraded",
		"incident",
		"status",
		"updatedAt",
	})
}

func TestContractPublicHubMomentsResponseJSONKeys(t *testing.T) {
	assertJSONFieldNames(t, reflect.TypeOf(PublicHubMomentsResponse{}), []string{
		"activityWindowMinutes",
		"bucketEnd",
		"bucketStart",
		"bucketT",
		"hubGeneratedAt",
		"moments",
		"reason",
		"source",
		"status",
	})
}

func TestContractExtensionPulseResponseCriticalJSONKeys(t *testing.T) {
	got := jsonFieldNames(reflect.TypeOf(ExtensionPulseResponse{}))
	critical := []string{
		"coverage",
		"coverageStartOffsetSeconds",
		"currentOffsetSeconds",
		"isLive",
		"lanes",
		"login",
		"peaks",
		"rollups",
		"streamId",
		"tracking",
		"vodId",
	}
	for _, key := range critical {
		found := false
		for _, g := range got {
			if g == key {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("ExtensionPulseResponse missing critical json key %q (have %v)", key, got)
		}
	}
}
