# ChronoLure

**Advanced Calendar-Based Social Engineering Framework**

ChronoLure is a specialized security testing toolkit that leverages calendar invitations as a phishing vector. This approach exploits the trust users place in calendar notifications and reminders, creating persistent engagement opportunities that traditional email phishing cannot achieve.

## 🎯 Core Concept

Calendar-based phishing offers unique advantages over traditional methods:

- **Persistence**: Events remain visible in users' calendars for extended periods
- **Automatic Reminders**: Calendar clients send reminders without additional action
- **Enhanced Trust**: Calendar invitations carry implicit legitimacy
- **Reduced Detection**: .ICS files face less scrutiny than HTML emails

## ✨ Key Features

### Calendar Campaign Management
- Generate RFC 5545-compliant .ICS calendar files
- Support for multiple platforms (Teams, Zoom, Google Meet)
- Customizable event details (title, description, location, time)
- Automatic reminder configuration

### Advanced Targeting
- Individual tracking via unique RID parameters
- Dynamic template variables for personalization
- Group-based campaign deployment
- Multi-platform calendar event simulation

### Comprehensive Analytics
- Track .ICS file delivery
- Monitor landing page access
- Capture credential submissions
- Real-time campaign metrics

### Email Integration
- MIME multipart messages with .ICS attachments
- Calendar invite format (`text/calendar; method=REQUEST`)
- SMTP profile management
- Template-based email generation

## 🚀 Quick Start

### Building from Source
Requires Go v1.10 or above:

```bash
git clone https://github.com/DonobanR/ChronoLure.git
cd ChronoLure
go build
```

### Running ChronoLure
```bash
./chronolure
```

Access the web interface at `https://localhost:3333`. Default credentials will be displayed in the console output.

### Creating a Calendar Campaign

1. Navigate to the Campaigns section
2. Select "Calendar Campaign" as the type
3. Configure event details:
   - Event title and description
   - Start time and duration
   - Platform type (Teams/Zoom/Google Meet)
   - Organizer information
4. Select target groups and landing page
5. Launch campaign

## 📋 Campaign Types

- **Traditional Email**: Standard phishing campaigns
- **Calendar Events**: .ICS-based calendar invitations with embedded tracking URLs
- **Hybrid**: Combined email and calendar approaches

## 🔧 Configuration

Edit `config.json` to customize:
- Server ports and SSL settings
- Database connection
- SMTP server configuration
- Admin credentials

## 📊 Tracking Capabilities

ChronoLure tracks:
- ✅ .ICS file sent successfully
- ✅ Landing page accessed (link clicked)
- ✅ Data submitted on landing page
- ✅ User reported the attempt

*Note: Client-side calendar actions (adding events, viewing reminders) occur locally and cannot be tracked by the server.*

## 🐳 Docker Support

Run ChronoLure in a container:

```bash
docker build -t chronolure .
docker run -p 3333:3333 chronolure
```

## 📖 Documentation

For detailed usage instructions, API documentation, and best practices, refer to the `/doc` directory.

## ⚠️ Legal Disclaimer

ChronoLure is designed for authorized security testing only. Users must:
- Obtain explicit written permission before conducting tests
- Comply with all applicable laws and regulations
- Use the tool ethically and responsibly

Unauthorized use of this tool may violate laws. The developers assume no liability for misuse.

## �️ Global Trash System

ChronoLure includes a unified **Windows Recycle Bin–style trash** at `/trash` that aggregates soft-deleted items from all supported models in one place, filterable by type.

### Adding a New Model to the Global Trash

The aggregator lives in `models/global_trash.go` and is designed to be extended with minimal effort. To add a new entity type (e.g. `Template`, `LandingPage`):

1. **Add soft-delete fields** — Embed `TrashableModel` in the model struct, or manually add columns:
   `deleted_at`, `deleted_by`, `delete_reason`, `restored_at`, `restored_by`.
   Add a DB migration for both `db/db_sqlite3/migrations/` and `db/db_mysql/migrations/`.

2. **Implement Trashable** — Create `models/yourmodel_trash.go`:
   - Implement: `GetID()`, `GetUserID()`, `GetName()`, `GetEntityType()`, `GetTableName()`, `PurgeDependencies(tx)`
   - Add wrapper functions: `SoftDeleteYourModel`, `RestoreYourModel`, `PurgeYourModel`, `GetTrashedYourModels`

3. **Register in the aggregator** — In `models/global_trash.go`:
   - Add a constant: `TrashTypeYourModel = "your_model"`
   - In `GetTrashItems`: add a collector branch for `TrashTypeYourModel`
   - In `RestoreTrashItem`: add a `case TrashTypeYourModel:` that calls `RestoreYourModel`
   - In `PurgeTrashItem`: add a `case TrashTypeYourModel:` that calls `PurgeYourModel`

4. **Done.** No template, JS, or route changes required — the `/trash` page renders type badges automatically and the filter tab counts update on their own.

## �📄 License

MIT License - See LICENSE file for details
