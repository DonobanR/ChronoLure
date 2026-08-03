# Fixes aplicados tras auditoria ChronoLure v2

Resumen tecnico de los fixes aplicados con base en la auditoria del vault de Obsidian ubicado en `/home/dono/Proyects/gophish/docs/obsidian-gophish-vault`.

## Commits cubiertos

- `c572797` - hardening de trash, purge, restore, campaign group links y TTL cleanup.
- `6cbcbe2` - hardening de trash lifecycle, audit, calendar y campaign group consistency.
- `6a37ba5` - ownership en purge y operaciones de campaign groups.
- `686a615` - TTL purge desactivado por defecto.

## Cambios principales

- Soft delete borra `mail_logs` pendientes de la campana.
- Worker filtra `mail_logs` con join contra `campaigns` y exige `campaigns.deleted_at IS NULL`.
- Purge/delete forever elimina `mail_logs` y links `campaign_group_campaigns`.
- Purge global valida tipo de item, estado en trash y confirmacion del nombre de campana en backend.
- TTL purge queda opt-in: `trash_purge_enabled=false` por defecto y el intervalo no habilita el job.
- `/calendar/track` persiste `calendar_events`.
- Submit calendar evita duplicar `Submitted Data`.
- Eventos calendar guardan IP, user-agent y detalles.
- `/calendar/download` solo aplica a campanas calendar y registra `ics_downloaded`.
- Campaign group stats excluye campanas soft-deleted.
- Purge valida ownership multiusuario.
- Audit log resuelve `actor_name` para acciones genericas de trash.
- Migracion MySQL calendar agrega `platform_type` y `event_meeting_url`.
- `Campaign` incluye `EventTitle` y `EventDescription`.

## Pruebas relacionadas

Pruebas agregadas o reforzadas en:

- `controllers/api/trash_test.go`
- `controllers/calendar_test.go`
- `controllers/phish_test.go`
- `models/audit_log_test.go`
- `models/calendar_event_test.go`
- `models/campaign_group_metrics_test.go`
- `models/campaign_test.go`
- `config/config_test.go`
- `worker/trash_ttl_job_test.go`

## Pendientes relevantes

- Definir politica explicita de captura de passwords en calendar.
- Revisar migracion MySQL soft delete que referencia `created_at` si la tabla usa `created_date`.
- Revisar tipos `INT`/`BIGINT` en columnas de usuario para campaign group trash MySQL.
- Confirmar contrato de endpoints de detalle/results para campanas en trash.
- Revisar limpieza de tablas nuevas en suite completa de tests.
