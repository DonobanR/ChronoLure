package report

import (
	"fmt"
	"time"

	"github.com/gophish/gophish/models"
)

// ReportSource decouples the generation engine from where the data comes from.
// The engine never knows whether it operates on a single Campaign or a Campaign
// Group; it only consumes this interface. Campaign Group is the primary use
// case (it integrates email, landing pages, calendar, Teams and credential
// capture), and an individual Campaign is fully supported through the same API.
type ReportSource interface {
	// Subject returns the kind ("campaign"|"campaign_group"), id and name.
	Subject() (kind string, id int64, name string)
	// Stats returns the normalized cumulative funnel input.
	Stats() (FunnelInput, error)
	// DateRange returns the execution window of the campaign(s).
	DateRange() (start, end time.Time, err error)
	// Recipients returns every recipient with the Spanish label of the furthest
	// funnel stage reached (same classification as the results table).
	Recipients() ([]Recipient, error)
}

// NewSource builds the appropriate ReportSource for the given subject kind,
// scoped to the owning user.
func NewSource(kind string, id, uid int64) (ReportSource, error) {
	switch kind {
	case "campaign":
		return &campaignSource{id: id, uid: uid}, nil
	case "campaign_group":
		return &campaignGroupSource{id: id, uid: uid}, nil
	}
	return nil, fmt.Errorf("unknown report subject kind: %q", kind)
}

// campaignSource adapts an individual campaign. The expensive loads are
// memoized on the struct so a single report generation queries each at most
// once, even though Subject/Stats/DateRange (summary) and Recipients
// (campaign) are separate interface methods. Results are identical to calling
// the models functions directly; only the redundant round-trips are removed.
type campaignSource struct {
	id  int64
	uid int64

	sumLoaded bool
	sum       models.CampaignSummary
	sumErr    error

	campLoaded bool
	camp       models.Campaign
	campErr    error
}

// summary loads (once) the campaign summary used by Subject/Stats/DateRange.
func (s *campaignSource) summary() (models.CampaignSummary, error) {
	if !s.sumLoaded {
		s.sum, s.sumErr = models.GetCampaignSummary(s.id, s.uid)
		s.sumLoaded = true
	}
	return s.sum, s.sumErr
}

// campaign loads (once) the campaign's results and events for Recipients. It
// uses the lightweight loader (Results + Events only) instead of GetCampaign,
// which also pulls Template/Page/SMTP/Attachments/Headers that the report never
// uses (I-3). The campaign-row scoping is identical (ScopeCampaignsActive), so
// Recipients sees the same rows.
func (s *campaignSource) campaign() (models.Campaign, error) {
	if !s.campLoaded {
		s.camp, s.campErr = models.GetCampaignResultsAndEvents(s.id, s.uid)
		s.campLoaded = true
	}
	return s.camp, s.campErr
}

func (s *campaignSource) Subject() (string, int64, string) {
	sum, _ := s.summary()
	return "campaign", s.id, sum.Name
}

func (s *campaignSource) Stats() (FunnelInput, error) {
	sum, err := s.summary()
	if err != nil {
		return FunnelInput{}, err
	}
	return fromCampaignStats(sum.Stats), nil
}

func (s *campaignSource) DateRange() (time.Time, time.Time, error) {
	sum, err := s.summary()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return sum.LaunchDate, sum.CompletedDate, nil
}

func (s *campaignSource) Recipients() ([]Recipient, error) {
	c, err := s.campaign()
	if err != nil {
		return nil, err
	}
	out := make([]Recipient, 0, len(c.Results))
	for _, r := range c.Results {
		// result.Status holds the furthest stage GoPhish recorded for the
		// recipient; classify it with the same rank() as the funnel.
		out = append(out, Recipient{
			Email:  r.Email,
			Status: recipientStatusLabel(rank(r.Status), r.Status == models.Error),
		})
	}
	return out, nil
}

// campaignGroupSource adapts a campaign group (the primary use case). The heavy
// loads are memoized: GetCampaignGroupStats (which scans ALL events+results of
// the group) is computed at most once per generation instead of once per
// Stats/Recipients call, and GetCampaignGroup at most once for Subject/
// DateRange. Results are identical; only duplicate scans are removed.
type campaignGroupSource struct {
	id  int64
	uid int64

	statsLoaded bool
	stats       models.CampaignGroupStats
	statsErr    error

	grpLoaded bool
	grp       models.CampaignGroup
	grpErr    error
}

// groupStats loads (once) the group's stats+journeys, the single most expensive
// query in a group report.
func (s *campaignGroupSource) groupStats() (models.CampaignGroupStats, error) {
	if !s.statsLoaded {
		s.stats, s.statsErr = models.GetCampaignGroupStats(s.id, s.uid)
		s.statsLoaded = true
	}
	return s.stats, s.statsErr
}

// group loads (once) the group structure used by Subject/DateRange.
func (s *campaignGroupSource) group() (models.CampaignGroup, error) {
	if !s.grpLoaded {
		s.grp, s.grpErr = models.GetCampaignGroup(s.id, s.uid)
		s.grpLoaded = true
	}
	return s.grp, s.grpErr
}

func (s *campaignGroupSource) Subject() (string, int64, string) {
	cg, _ := s.group()
	return "campaign_group", s.id, cg.Name
}

func (s *campaignGroupSource) Stats() (FunnelInput, error) {
	stats, err := s.groupStats()
	if err != nil {
		return FunnelInput{}, err
	}
	// Consume the group's canonical per-recipient counters DIRECTLY (computed by
	// GetCampaignGroupStats). This guarantees the report shows byte-identical
	// numbers to the Campaign Group dashboard and CSV — a single source of truth.
	return FunnelInput{
		TotalRecipients: stats.TotalRecipients,
		Sent:            stats.EmailsSent,
		Opened:          stats.OpenedEmail,
		Clicked:         stats.ClickedLink,
		Submitted:       stats.SubmittedData,
		Errored:         stats.Error,
		Reported:        stats.EmailReported,
	}, nil
}

func (s *campaignGroupSource) DateRange() (time.Time, time.Time, error) {
	cg, err := s.group()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	var start, end time.Time
	for _, cgc := range cg.Campaigns {
		c := cgc.Campaign
		if c.DeletedAt != nil {
			continue
		}
		if start.IsZero() || c.LaunchDate.Before(start) {
			start = c.LaunchDate
		}
		if c.CompletedDate.After(end) {
			end = c.CompletedDate
		}
	}
	return start, end, nil
}

func (s *campaignGroupSource) Recipients() ([]Recipient, error) {
	stats, err := s.groupStats()
	if err != nil {
		return nil, err
	}
	out := make([]Recipient, 0, len(stats.RecipientJourneys))
	for _, j := range stats.RecipientJourneys {
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
		out = append(out, Recipient{Email: j.Email, Status: recipientStatusLabel(best, hadError)})
	}
	return out, nil
}
