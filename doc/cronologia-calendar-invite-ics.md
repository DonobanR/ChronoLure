# Cronología técnica de Calendar Invite / ICS (basada en commits y diffs)

Este documento se basa **exclusivamente** en el historial disponible del repositorio. No se especula sobre diseño futuro.

---

## 1) Primer commit donde aparece Calendar Invite / ICS

### Commit inicial: `5ce8db9` (Wed Dec 31 2025)
**Mensaje:** “Initial commit: ChronoLure - Advanced Calendar-Based Social Engineering Framework”

**Por qué es el primer commit con ICS:**
- En el `git log --reverse` el commit inicial ya incluye el módulo `ics/` y el controlador `controllers/calendar.go`.
- No hay commits anteriores en este repositorio.

**Archivos y funciones clave introducidos en este commit:**
- **Endpoints calendar**:
  - `controllers/phish.go`: rutas `/calendar`, `/calendar/track`, `/calendar/download` registradas en `registerRoutes()`.
- **Controlador calendar**:
  - `controllers/calendar.go`: `CalendarPhish`, `CalendarTrack`, `CalendarDownloadICS`.
- **Generación de ICS**:
  - `ics/ics.go`: `CalendarEvent.Generate()`.
  - `models/maillog.go`: `generateCalendarEmail(...)`, `GenerateICSForResult(...)`.
- **Persistencia de eventos calendar**:
  - `models/calendar_event.go`: `CalendarEvent`, `SaveCalendarEvent`, `GetCalendarEventsByResult`, `GetCalendarEventsByCampaign`.
- **Migraciones de base de datos**:
  - `db/db_sqlite3/migrations/20251227000000_calendar_phishing.sql`.
  - `db/db_mysql/migrations/20251227000000_calendar_phishing.sql`.
- **UI/Frontend para campañas calendar**:
  - `templates/campaigns.html` + `static/js/src/app/campaigns.js`.
  - `templates/calendar_landing.html`, `templates/teams_login.html`.

**Comportamiento nuevo habilitado por este commit:**
- Envío de campañas con invitación calendar adjunta (`text/calendar; method=REQUEST`) en `generateCalendarEmail`. `models/maillog.go`.
- Nuevo flujo de phishing vía `/calendar` con tracking propio (`calendar_events`). `controllers/calendar.go`.
- Posibilidad de descargar un `.ics` para testing con `/calendar/download`. `controllers/calendar.go`.

---

## 2) Commits relevantes y qué se añadió/modificó

### Commit `2b47046` (Tue Jan 6 2026)
**Mensaje:** “feat: Use EventMeetingURL from campaign for redirect after credential capture”

**Archivo modificado:**
- `controllers/calendar.go`.

**Cambio concreto:**
- En `handleCalendarPhishPOST` se prioriza `c.EventMeetingURL` sobre `c.Page.RedirectURL` al construir la redirección.

**Comportamiento nuevo habilitado:**
- La redirección post‑submit puede apuntar a un **meeting URL** configurado en la campaña, en lugar de la URL de la landing. Esto cambia el destino final tras captura de credenciales en campañas calendar.

---

### Commit `775ce59` (Mon Jan 19 2026)
**Mensaje:** “feat: Add campaign trash system with soft delete and TTL auto-purge”

**Archivos relevantes para calendar/ICS en este commit:**
- `models/campaign_trash.go`: añade limpieza de `calendar_events` cuando se purga una campaña.
- `db/db_sqlite3/migrations/20260118000001_add_soft_delete_campaigns.sql` y `db/db_mysql/migrations/20260118000001_add_soft_delete_campaigns.sql`: cambios generales de soft delete (no específicos de ICS).

**Comportamiento nuevo habilitado (relacionado a ICS):**
- Al purgar campañas, se eliminan eventos de `calendar_events` relacionados con los resultados, evitando residuos de tracking calendar. `models/campaign_trash.go`.

---

### Commit `59102d5` (Mon Jan 19 2026)
**Mensaje:** “fix: Add fallback for undefined campaign statuses in dashboard”

**Archivo modificado:**
- `static/js/src/app/dashboard.js`.

**Relación con ICS:**
- No introduce comportamiento ICS directo; es un fix de UI de dashboard.

---

### Commit `a4c0065` (Mon Jan 19 2026)
**Mensaje:** “fix: Restore campaign to original status instead of forcing Created”

**Archivo modificado:**
- `models/campaign_trash.go`.

**Relación con ICS:**
- Ajusta estados de campaña en restauración (general). No altera el flujo ICS.

---

## 3) Dependencias o librerías nuevas introducidas

### En el commit inicial `5ce8db9`
- Todas las dependencias ya aparecen en `go.mod` desde el primer commit. No hay un commit previo para comparar.

### En el commit `775ce59`
`go.mod` y `go.sum` cambian.

**Cambios observables en `go.mod`:**
- Se agrega `toolchain go1.22.2`.
- Se introduce `github.com/stretchr/testify v1.4.0` como dependencia **directa** (antes estaba como indirecta en el commit inicial).
- Se elimina una lista amplia de dependencias indirectas que estaban presentes en el commit inicial.

**Conclusión objetiva:**
- El único commit que altera dependencias es `775ce59`. No hay cambios de dependencias en `2b47046`, `59102d5`, `a4c0065`.

---

## 4) Qué comportamiento nuevo habilitó cada cambio

### `5ce8db9` (introducción de ICS)
- **Generación de archivos `.ics`**: `ics/ics.go` + `models/maillog.go`.
- **Envío de invitaciones calendar por email** con MIME `text/calendar; method=REQUEST`. `models/maillog.go`.
- **Flujo de phishing calendar** vía `/calendar` con tracking específico (`calendar_events`). `controllers/calendar.go`.
- **Tracking separado** de eventos ICS: `ics_sent`, `link_opened`, `credentials_submitted`. `models/calendar_event.go` + `controllers/calendar.go` + `models/maillog.go`.
- **UI de campañas calendar** con campos adicionales. `templates/campaigns.html`, `static/js/src/app/campaigns.js`.

### `2b47046` (redirección post‑submit)
- Permite redirigir a `EventMeetingURL` si está configurado, evitando depender de la redirección de la landing. `controllers/calendar.go`.

### `775ce59` (trash + TTL)
- Purga de `calendar_events` cuando se eliminan campañas, evitando dejar tracking calendar huérfano. `models/campaign_trash.go`.

### `59102d5`, `a4c0065`
- Cambios de dashboard y restore de campañas; no cambian la lógica ICS.

---

## Referencias rápidas (commits, archivos, funciones)

- **Commit inicial con ICS**: `5ce8db9`.
- **Redirección EventMeetingURL**: `2b47046` → `controllers/calendar.go:handleCalendarPhishPOST`.
- **Limpieza de calendar_events**: `775ce59` → `models/campaign_trash.go`.
- **Endpoints calendar**: `controllers/phish.go:registerRoutes`.
- **ICS generator**: `ics/ics.go:CalendarEvent.Generate`.
- **Email con .ics**: `models/maillog.go:generateCalendarEmail`.
- **Download ICS**: `controllers/calendar.go:CalendarDownloadICS`.
