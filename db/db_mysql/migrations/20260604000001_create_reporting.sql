-- +goose Up
-- Reporting module (DOCX mail-merge). Content-addressed blob storage + versioned
-- templates + immutable render snapshots. All binary content lives in report_blobs.

CREATE TABLE report_blobs (
    sha256     CHAR(64) NOT NULL PRIMARY KEY,
    content    LONGBLOB NOT NULL,
    size       BIGINT NOT NULL,
    created_at DATETIME
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE report_templates (
    id                BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id           BIGINT NOT NULL,
    name              VARCHAR(255) NOT NULL,
    description       TEXT,
    active_version_id BIGINT,
    is_default        BOOLEAN NOT NULL DEFAULT 0,
    created_at        DATETIME,
    updated_at        DATETIME,
    INDEX idx_report_templates_user (user_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE report_template_versions (
    id               BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    template_id      BIGINT NOT NULL,
    version          INT NOT NULL,
    content_sha256   CHAR(64) NOT NULL,
    tokens_json      TEXT,
    image_slots_json TEXT,
    validation_json  TEXT,
    uploaded_by      BIGINT,
    created_at       DATETIME,
    UNIQUE KEY uq_template_version (template_id, version),
    INDEX idx_report_template_versions_template (template_id),
    FOREIGN KEY (template_id) REFERENCES report_templates(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE reports (
    id             BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id        BIGINT NOT NULL,
    subject_kind   VARCHAR(20) NOT NULL,
    subject_id     BIGINT NOT NULL,
    template_id    BIGINT NOT NULL,
    company_name   VARCHAR(255),
    prepared_by    VARCHAR(255),
    report_date    DATETIME,
    executed_from  DATETIME,
    executed_to    DATETIME,
    users_with_2fa INT NOT NULL DEFAULT 0,
    intro_exec     TEXT,
    text_punto_1   TEXT,
    status         VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at     DATETIME,
    updated_at     DATETIME,
    INDEX idx_reports_user (user_id),
    INDEX idx_reports_template (template_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE report_assets (
    id             BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    report_id      BIGINT NOT NULL,
    slot           VARCHAR(40) NOT NULL,
    content_sha256 CHAR(64) NOT NULL,
    mime           VARCHAR(80),
    created_at     DATETIME,
    UNIQUE KEY uq_report_slot (report_id, slot),
    INDEX idx_report_assets_report (report_id),
    FOREIGN KEY (report_id) REFERENCES reports(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE report_renders (
    id                  BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    report_id           BIGINT NOT NULL,
    template_version_id BIGINT NOT NULL,
    metrics_json        TEXT NOT NULL,
    output_sha256       CHAR(64),
    generated_by        BIGINT,
    created_at          DATETIME,
    INDEX idx_report_renders_report (report_id),
    FOREIGN KEY (report_id) REFERENCES reports(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE report_render_assets (
    id             BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    render_id      BIGINT NOT NULL,
    slot           VARCHAR(40) NOT NULL,
    content_sha256 CHAR(64) NOT NULL,
    mime           VARCHAR(80),
    created_at     DATETIME,
    UNIQUE KEY uq_render_slot (render_id, slot),
    INDEX idx_report_render_assets_render (render_id),
    FOREIGN KEY (render_id) REFERENCES report_renders(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS report_render_assets;
DROP TABLE IF EXISTS report_renders;
DROP TABLE IF EXISTS report_assets;
DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS report_template_versions;
DROP TABLE IF EXISTS report_templates;
DROP TABLE IF EXISTS report_blobs;
