package config

import (
	"encoding/json"
	"io/ioutil"

	log "github.com/gophish/gophish/logger"
)

// AdminServer represents the Admin server configuration details
type AdminServer struct {
	ListenURL            string   `json:"listen_url"`
	UseTLS               bool     `json:"use_tls"`
	CertPath             string   `json:"cert_path"`
	KeyPath              string   `json:"key_path"`
	CSRFKey              string   `json:"csrf_key"`
	AllowedInternalHosts []string `json:"allowed_internal_hosts"`
	TrustedOrigins       []string `json:"trusted_origins"`
}

// PhishServer represents the Phish server configuration details
type PhishServer struct {
	ListenURL string `json:"listen_url"`
	UseTLS    bool   `json:"use_tls"`
	CertPath  string `json:"cert_path"`
	KeyPath   string `json:"key_path"`
}

// Config represents the configuration information.
type Config struct {
	AdminConf      AdminServer `json:"admin_server"`
	PhishConf      PhishServer `json:"phish_server"`
	DBName         string      `json:"db_name"`
	DBPath         string      `json:"db_path"`
	DBSSLCaPath    string      `json:"db_sslca_path"`
	MigrationsPath string      `json:"migrations_prefix"`
	TestFlag       bool        `json:"test_flag"`
	ContactAddress string      `json:"contact_address"`
	Logging        *log.Config `json:"logging"`
	// Trash/soft delete configuration
	TrashRetentionDays  int  `json:"trash_retention_days"`
	TrashPurgeEnabled   bool `json:"trash_purge_enabled"`
	TrashPurgeInterval  int  `json:"trash_purge_interval_minutes"` // In minutes
	TrashPurgeBatchSize int  `json:"trash_purge_batch_size"`
	// Reporting module feature flag. Disabled by default: when off, no reporting
	// routes, endpoints, pages or menus are exposed.
	Reporting ReportingConfig `json:"reporting"`
}

// ReportingConfig holds the configuration for the DOCX reporting module.
type ReportingConfig struct {
	Enabled bool `json:"enabled"`
	// EnableBlobDownload registers the render download endpoint that serves the
	// stored DOCX byte-for-byte. Disabled by default; when off, the route is not
	// registered at all (natural 404).
	EnableBlobDownload bool `json:"enable_blob_download"`
}

// Version contains the current gophish version
var Version = ""

// ServerName is the server type that is returned in the transparency response.
const ServerName = "gophish"

// LoadConfig loads the configuration from the specified filepath
func LoadConfig(filepath string) (*Config, error) {
	// Get the config file
	configFile, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	err = json.Unmarshal(configFile, config)
	if err != nil {
		return nil, err
	}
	if config.Logging == nil {
		config.Logging = &log.Config{}
	}
	// Set default trash retention if not specified
	if config.TrashRetentionDays == 0 {
		config.TrashRetentionDays = 90 // Default: 90 days
	}
	// Set default trash purge settings if not specified
	if config.TrashPurgeInterval == 0 {
		config.TrashPurgeInterval = 60 // Default: 60 minutes (1 hour)
	}
	if config.TrashPurgeBatchSize == 0 {
		config.TrashPurgeBatchSize = 100 // Default: 100 campaigns per batch
	}
	// Trash purge is opt-in. The zero value for TrashPurgeEnabled is false,
	// and interval/batch defaults must not enable automatic purging by themselves.
	// Choosing the migrations directory based on the database used.
	config.MigrationsPath = config.MigrationsPath + config.DBName
	// Explicitly set the TestFlag to false to prevent config.json overrides
	config.TestFlag = false
	return config, nil
}
