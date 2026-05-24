package models

import (
	"fmt"
	"testing"
	"time"

	"github.com/gophish/gophish/config"
	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

type ModelsSuite struct {
	config *config.Config
}

var _ = check.Suite(&ModelsSuite{})

func (s *ModelsSuite) SetUpSuite(c *check.C) {
	conf := &config.Config{
		DBName:         "sqlite3",
		DBPath:         ":memory:",
		MigrationsPath: "../db/db_sqlite3/migrations/",
	}
	s.config = conf
	err := Setup(conf)
	if err != nil {
		c.Fatalf("Failed creating database: %v", err)
	}
}

func (s *ModelsSuite) TearDownTest(c *check.C) {
	// Clear database tables between each test. If new tables are
	// used in this test suite they will need to be cleaned up here.
	db.Exec("DELETE FROM calendar_events")
	db.Exec("DELETE FROM audit_log")
	db.Exec("DELETE FROM campaign_group_campaigns")
	db.Exec("DELETE FROM campaign_groups")
	db.Delete(Group{})
	db.Delete(Target{})
	db.Delete(GroupTarget{})
	db.Delete(SMTP{})
	db.Delete(Page{})
	db.Delete(Result{})
	db.Delete(MailLog{})
	db.Unscoped().Delete(Campaign{})

	// Reset users table to default state.
	db.Not("id", 1).Delete(User{})
	db.Model(User{}).Update("username", "admin")
}

func (s *ModelsSuite) TestTearDownTestCleansChronoLureTables(c *check.C) {
	c.Assert(db.Exec("INSERT INTO audit_log (action, entity_type, entity_id) VALUES (?, ?, ?)", "TEST", "campaign", 1).Error, check.Equals, nil)
	c.Assert(db.Exec("INSERT INTO calendar_events (result_id, event_type, timestamp) VALUES (?, ?, ?)", 1, "test", time.Now().UTC()).Error, check.Equals, nil)
	c.Assert(db.Exec("INSERT INTO campaign_groups (id, name, user_id) VALUES (?, ?, ?)", 999, "cleanup group", 1).Error, check.Equals, nil)
	c.Assert(db.Exec("INSERT INTO campaign_group_campaigns (group_id, campaign_id) VALUES (?, ?)", 999, 999).Error, check.Equals, nil)

	s.TearDownTest(c)

	for _, table := range []string{"calendar_events", "audit_log", "campaign_group_campaigns", "campaign_groups"} {
		var count int
		c.Assert(db.Table(table).Count(&count).Error, check.Equals, nil)
		c.Assert(count, check.Equals, 0)
	}
}

func (s *ModelsSuite) createCampaignDependencies(ch *check.C, optional ...string) Campaign {
	// we use the optional parameter to pass an alternative subject
	group := Group{Name: "Test Group"}
	group.Targets = []Target{
		Target{BaseRecipient: BaseRecipient{Email: "test1@example.com", FirstName: "First", LastName: "Example"}},
		Target{BaseRecipient: BaseRecipient{Email: "test2@example.com", FirstName: "Second", LastName: "Example"}},
		Target{BaseRecipient: BaseRecipient{Email: "test3@example.com", FirstName: "Second", LastName: "Example"}},
		Target{BaseRecipient: BaseRecipient{Email: "test4@example.com", FirstName: "Second", LastName: "Example"}},
	}
	group.UserId = 1
	ch.Assert(PostGroup(&group), check.Equals, nil)

	// Add a template
	t := Template{Name: "Test Template"}
	if len(optional) > 0 {
		t.Subject = optional[0]
	} else {
		t.Subject = "{{.RId}} - Subject"
	}
	t.Text = "{{.RId}} - Text"
	t.HTML = "{{.RId}} - HTML"
	t.UserId = 1
	ch.Assert(PostTemplate(&t), check.Equals, nil)

	// Add a landing page
	p := Page{Name: "Test Page"}
	p.HTML = "<html>Test</html>"
	p.UserId = 1
	ch.Assert(PostPage(&p), check.Equals, nil)

	// Add a sending profile
	smtp := SMTP{Name: "Test Page"}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "test@test.com"
	ch.Assert(PostSMTP(&smtp), check.Equals, nil)

	c := Campaign{Name: fmt.Sprintf("Test campaign %d", time.Now().UnixNano())}
	c.UserId = 1
	c.Template = t
	c.Page = p
	c.SMTP = smtp
	c.Groups = []Group{group}
	return c
}

func (s *ModelsSuite) createCampaign(ch *check.C) Campaign {
	c := s.createCampaignDependencies(ch)
	// Setup and "launch" our campaign
	ch.Assert(PostCampaign(&c, c.UserId), check.Equals, nil)

	// For comparing the dates, we need to fetch the campaign again. This is
	// to solve an issue where the campaign object right now has time down to
	// the microsecond, while in MySQL it's rounded down to the second.
	c, _ = GetCampaign(c.Id, c.UserId)
	return c
}

func setupBenchmark(b *testing.B) {
	conf := &config.Config{
		DBName:         "sqlite3",
		DBPath:         ":memory:",
		MigrationsPath: "../db/db_sqlite3/migrations/",
	}
	err := Setup(conf)
	if err != nil {
		b.Fatalf("Failed creating database: %v", err)
	}
}

func tearDownBenchmark(b *testing.B) {
	err := db.Close()
	if err != nil {
		b.Fatalf("error closing database: %v", err)
	}
}

func resetBenchmark(b *testing.B) {
	db.Delete(Group{})
	db.Delete(Target{})
	db.Delete(GroupTarget{})
	db.Delete(SMTP{})
	db.Delete(Page{})
	db.Delete(Result{})
	db.Delete(MailLog{})
	db.Delete(Campaign{})

	// Reset users table to default state.
	db.Not("id", 1).Delete(User{})
	db.Model(User{}).Update("username", "admin")
}
