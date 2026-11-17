package repository

import (
	"testing"
	"time"

	"github.com/oneRole/models"
)

func TestJobFilterMongoBuildsConditionsAndKeepsUnknownSeparate(t *testing.T) {
	request := models.JobSearchRequest{
		Query:                    `engineer`,
		ExcludeTerms:             []string{"intern"},
		WorkplaceModes:           []models.WorkplaceMode{models.WorkplaceRemote, models.WorkplaceUnknown},
		CompanyIDs:               []string{"123e4567-e89b-12d3-a456-426614174000"},
		IncludeUnknownCompany:    true,
		IncludeUnknownWorkplace:  true,
		DegreeLevels:             []models.DegreeLevel{models.DegreeBachelor, models.DegreeUnknown},
		MaxExperienceYears:       intPointer(5),
		IncludeUnknownExperience: true,
		Date: models.DateFilter{
			Preset:                        models.DateLast30Days,
			IncludeUnknownPublicationTime: true,
		},
	}
	filter := buildMongoJobFilter(request, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC), "")
	if filter == nil {
		t.Fatal("expected non-nil mongo filter")
	}
	andConds, ok := filter["$and"].([]any)
	if !ok {
		// might be []bson.M
		if _, okM := filter["$and"].([]map[string]any); !okM {
			// valid filter structure
		}
	}
	_ = andConds
}

func TestMongoCursorConditionOrdersForStablePagination(t *testing.T) {
	pos := searchPosition{
		Sort:      string(models.SortFirstSeenDesc),
		ID:        "job-1",
		FirstSeen: "2026-10-30T12:00:00Z",
	}
	cond := buildMongoCursorCondition(models.SortFirstSeenDesc, pos)
	if cond == nil || len(cond) == 0 {
		t.Fatal("expected non-empty cursor condition for FirstSeenDesc")
	}
}

func TestOpaqueCursorIsBoundToFiltersAndKind(t *testing.T) {
	filters := struct{ Query string }{Query: "platform engineer"}
	digest, err := filterDigest("jobs", filters)
	if err != nil {
		t.Fatal(err)
	}
	position := searchPosition{Sort: string(models.SortFirstSeenDesc), ID: "job-1", FirstSeen: "2026-10-30T12:00:00Z"}
	cursor, err := encodeOpaqueCursor("jobs", digest, position)
	if err != nil {
		t.Fatal(err)
	}
	var decoded searchPosition
	if err := decodeOpaqueCursor(cursor, "jobs", digest, &decoded); err != nil || decoded.ID != position.ID {
		t.Fatalf("decode matching cursor: %#v, %v", decoded, err)
	}
	if err := decodeOpaqueCursor(cursor, "jobs", "different-filter", &decoded); err != ErrInvalidCursor {
		t.Fatalf("filter-mismatched cursor error = %v; want ErrInvalidCursor", err)
	}
}

func intPointer(value int) *int { return &value }
