-- +goose Up
-- Reporting module (DOCX mail-merge). Content-addressed blob storage + versioned
-- templates + immutable render snapshots. All binary content lives in report_blobs.

CREATE TABLE report_blobs (
    sha256     CHAR(64) PRIMARY KEY,
    content    BLOB NOT NULL,
    size       INTEGER NOT NULL,
    created_at DATETIME
);

CREATE TABLE report_templates (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           BIGINT NOT NULL,
    name              VARCHAR(255) NOT NULL,
    description       TEXT,
    active_version_id BIGINT,
    is_default        BOOLEAN NOT NULL DEFAULT 0,
    created_at        DATETIME,
    updated_at        DATETIME
);
CREATE INDEX idx_report_templates_user ON report_templates(user_id);

CREATE TABLE report_template_versions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id      BIGINT NOT NULL,
    version          INTEGER NOT NULL,
    content_sha256   CHAR(64) NOT NULL,
    tokens_json      TEXT,
    image_slots_json TEXT,
    validation_json  TEXT,
    uploaded_by      BIGINT,
    created_at       DATETIME,
    UNIQUE (template_id, version)
);
CREATE INDEX idx_report_template_versions_template ON report_template_versions(template_id);

CREATE TABLE reports (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id        BIGINT NOT NULL,
    subject_kind   VARCHAR(20) NOT NULL,   -- campaign | campaign_group
    subject_id     BIGINT NOT NULL,
    template_id    BIGINT NOT NULL,        -- draft tracks template, not version
    company_name   VARCHAR(255),
    prepared_by    VARCHAR(255),
    report_date    DATETIME,
    executed_from  DATETIME,
    executed_to    DATETIME,
    users_with_2fa INTEGER NOT NULL DEFAULT 0,
    intro_exec     TEXT,
    text_punto_1   TEXT,
    status         VARCHAR(20) NOT NULL DEFAULT 'draft', -- draft | generated
    created_at     DATETIME,
    updated_at     DATETIME
);
CREATE INDEX idx_reports_user ON reports(user_id);
CREATE INDEX idx_reports_template ON reports(template_id);

CREATE TABLE report_assets (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id      BIGINT NOT NULL,
    slot           VARCHAR(40) NOT NULL,
    content_sha256 CHAR(64) NOT NULL,
    mime           VARCHAR(80),
    created_at     DATETIME,
    UNIQUE (report_id, slot)
);
CREATE INDEX idx_report_assets_report ON report_assets(report_id);

CREATE TABLE report_renders (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    report_id           BIGINT NOT NULL,
    template_version_id BIGINT NOT NULL,    -- exact version frozen at generation
    metrics_json        TEXT NOT NULL,      -- funnel + submitted + 2fa snapshot
    output_sha256       CHAR(64),           -- optional frozen output docx
    generated_by        BIGINT,
    created_at          DATETIME
);
CREATE INDEX idx_report_renders_report ON report_renders(report_id);

CREATE TABLE report_render_assets (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    render_id      BIGINT NOT NULL,
    slot           VARCHAR(40) NOT NULL,
    content_sha256 CHAR(64) NOT NULL,
    mime           VARCHAR(80),
    created_at     DATETIME,
    UNIQUE (render_id, slot)
);
CREATE INDEX idx_report_render_assets_render ON report_render_assets(render_id);

-- +goose Down
DROP TABLE IF EXISTS report_render_assets;
DROP TABLE IF EXISTS report_renders;
DROP TABLE IF EXISTS report_assets;
DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS report_template_versions;
DROP TABLE IF EXISTS report_templates;
DROP TABLE IF EXISTS report_blobs;
