// Copyright 2014 Unknwon
// Copyright 2014 Torkel Ödegaard

package setting

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/ini.v1"

	"github.com/go-macaron/session"

	"github.com/grafana/grafana/pkg/log"
	"github.com/grafana/grafana/pkg/util"
)

type Scheme string

const (
	HTTP              Scheme = "http"
	HTTPS             Scheme = "https"
	SOCKET            Scheme = "socket"
	DEFAULT_HTTP_ADDR string = "0.0.0.0"
)

const (
	DEV                                           string = "development"
	PROD                                          string = "production"
	TEST                                          string = "test"
	DEFAULT_ALERT_EVALTIME_LIMIT                  int64  = 21600 //time in seconds = 6 hrs
	DEFAULT_MISSING_ALERT_COUNT                   int    = 500
	DEFAULT_MISSING_ALERTS_DELAY                  int64  = 600     // 10 mins delay
	DEFAULT_MISSING_ALERTS_SCHEDULAR_TIME_MINUTES int    = 10      //Range from [1-60] to represent 60 minutes
	DEFAULT_CLUSTERING_CLEANUP_PERIOD             int    = 24      // Range from [1-24] to represent 24 hrs.
	DEFAULT_CLUSTERING_HB_RETENSION_PERIOD        int    = 86400   // 1 day
	DEFAULT_ANNOTATION_RETENSION_PERIOD           int    = 1209600 // 14 days
)

var (
	// App settings.
	Env          string = DEV
	AppUrl       string
	AppSubUrl    string
	InstanceName string

	// build
	BuildVersion string
	BuildCommit  string
	BuildStamp   int64

	// Paths
	LogsPath       string
	HomePath       string
	DataPath       string
	PluginsPath    string
	CustomInitPath = "conf/custom.ini"

	// Log settings.
	LogModes   []string
	LogConfigs []util.DynMap

	// Http server options
	Protocol           Scheme
	Domain             string
	HttpAddr, HttpPort string
	SshPort            int
	CertFile, KeyFile  string
	SocketPath         string
	RouterLogging      bool
	DataProxyLogging   bool
	StaticRootPath     string
	EnableGzip         bool
	EnforceDomain      bool

	// Security settings.
	SecretKey             string
	LogInRememberDays     int
	CookieUserName        string
	CookieRememberName    string
	DisableGravatar       bool
	EmailCodeValidMinutes int
	DataProxyWhiteList    map[string]bool

	// Snapshots
	ExternalSnapshotUrl   string
	ExternalSnapshotName  string
	ExternalEnabled       bool
	SnapShotTTLDays       int
	SnapShotRemoveExpired bool

	// User settings
	AllowUserSignUp         bool
	AllowUserOrgCreate      bool
	AutoAssignOrg           bool
	AutoAssignOrgRole       string
	VerifyEmailEnabled      bool
	LoginHint               string
	DefaultTheme            string
	DisableLoginForm        bool
	DisableSignoutMenu      bool
	ExternalUserMngLinkUrl  string
	ExternalUserMngLinkName string
	ExternalUserMngInfo     string

	// Http auth
	AdminUser     string
	AdminPassword string

	AnonymousEnabled bool
	AnonymousOrgName string
	AnonymousOrgRole string

	// Auth proxy settings
	AuthProxyEnabled        bool
	AuthProxyHeaderName     string
	AuthProxyHeaderProperty string
	AuthProxyAutoSignUp     bool
	AuthProxyLdapSyncTtl    int
	AuthProxyWhitelist      string

	// Basic Auth
	BasicAuthEnabled bool

	// Session settings.
	SessionOptions session.Options

	// Global setting objects.
	Cfg          *ini.File
	ConfRootPath string
	IsWindows    bool

	// PhantomJs Rendering
	ImagesDir  string
	PhantomDir string

	// for logging purposes
	configFiles                  []string
	appliedCommandLineProperties []string
	appliedEnvOverrides          []string

	ReportingEnabled   bool
	CheckForUpdates    bool
	GoogleAnalyticsId  string
	GoogleTagManagerId string

	// LDAP
	LdapEnabled     bool
	LdapConfigFile  string
	LdapAllowSignup bool = true

	// SMTP email settings
	Smtp SmtpSettings

	// QUOTA
	Quota QuotaSettings

	// Alerting
	AlertingEnabled bool
	ExecuteAlerts   bool

	// logger
	logger log.Logger

	// Grafana.NET URL
	GrafanaComUrl string

	// S3 temp image store
	S3TempImageStoreBucketUrl string
	S3TempImageStoreAccessKey string
	S3TempImageStoreSecretKey string

	ImageUploadProvider string

	// Clustering
	ClusteringEnabled                        bool
	MaxAlertEvalTimeLimitInSeconds           int64 = DEFAULT_ALERT_EVALTIME_LIMIT
	MaxMissingAlertCount                     int   = DEFAULT_MISSING_ALERT_COUNT
	DefaultMissingAlertsDelay                int64 = DEFAULT_MISSING_ALERTS_DELAY
	DefaultMissingAlertsSchedularTimeMinutes int   = DEFAULT_MISSING_ALERTS_SCHEDULAR_TIME_MINUTES
	ClusteringCleanupPeriod                  int
	ClusteringHBRetention                    int
	AnnotationRetention                      int
)

type CommandLineArgs struct {
	Config   string
	HomePath string
	Args     []string
}

func init() {
	IsWindows = runtime.GOOS == "windows"
	logger = log.New("settings")
}

func parseAppUrlAndSubUrl(section *ini.Section) (string, string) {
	appUrl := section.Key("root_url").MustString("http://localhost:3000/")
	if appUrl[len(appUrl)-1] != '/' {
		appUrl += "/"
	}

	// Check if has app suburl.
	url, err := url.Parse(appUrl)
	if err != nil {
		log.Fatal(4, "Invalid root_url(%s): %s", appUrl, err)
	}
	appSubUrl := strings.TrimSuffix(url.Path, "/")

	return appUrl, appSubUrl
}

func ToAbsUrl(relativeUrl string) string {
	return AppUrl + relativeUrl
}

func shouldRedactKey(s string) bool {
	uppercased := strings.ToUpper(s)
	return strings.Contains(uppercased, "PASSWORD") || strings.Contains(uppercased, "SECRET") || strings.Contains(uppercased, "PROVIDER_CONFIG")
}

func shouldRedactURLKey(s string) bool {
	uppercased := strings.ToUpper(s)
	return strings.Contains(uppercased, "DATABASE_URL")
}

func applyEnvVariableOverrides() {
	appliedEnvOverrides = make([]string, 0)
	for _, section := range Cfg.Sections() {
		for _, key := range section.Keys() {
			sectionName := strings.ToUpper(strings.Replace(section.Name(), ".", "_", -1))
			keyName := strings.ToUpper(strings.Replace(key.Name(), ".", "_", -1))
			envKey := fmt.Sprintf("GF_%s_%s", sectionName, keyName)
			envValue := os.Getenv(envKey)

			if len(envValue) > 0 {
				key.SetValue(envValue)
				if shouldRedactKey(envKey) {
					envValue = "*********"
				}
				if shouldRedactURLKey(envKey) {
					u, _ := url.Parse(envValue)
					ui := u.User
					if ui != nil {
						_, exists := ui.Password()
						if exists {
							u.User = url.UserPassword(ui.Username(), "-redacted-")
							envValue = u.String()
						}
					}
				}
				appliedEnvOverrides = append(appliedEnvOverrides, fmt.Sprintf("%s=%s", envKey, envValue))
			}
		}
	}
}

func applyCommandLineDefaultProperties(props map[string]string) {
	appliedCommandLineProperties = make([]string, 0)
	for _, section := range Cfg.Sections() {
		for _, key := range section.Keys() {
			keyString := fmt.Sprintf("default.%s.%s", section.Name(), key.Name())
			value, exists := props[keyString]
			if exists {
				key.SetValue(value)
				if shouldRedactKey(keyString) {
					value = "*********"
				}
				appliedCommandLineProperties = append(appliedCommandLineProperties, fmt.Sprintf("%s=%s", keyString, value))
			}
		}
	}
}

func applyCommandLineProperties(props map[string]string) {
	for _, section := range Cfg.Sections() {
		for _, key := range section.Keys() {
			keyString := fmt.Sprintf("%s.%s", section.Name(), key.Name())
			value, exists := props[keyString]
			if exists {
				key.SetValue(value)
				appliedCommandLineProperties = append(appliedCommandLineProperties, fmt.Sprintf("%s=%s", keyString, value))
			}
		}
	}
}

func getCommandLineProperties(args []string) map[string]string {
	props := make(map[string]string)

	for _, arg := range args {
		if !strings.HasPrefix(arg, "cfg:") {
			continue
		}

		trimmed := strings.TrimPrefix(arg, "cfg:")
		parts := strings.Split(trimmed, "=")
		if len(parts) != 2 {
			log.Fatal(3, "Invalid command line argument", arg)
			return nil
		}

		props[parts[0]] = parts[1]
	}
	return props
}

func makeAbsolute(path string, root string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func evalEnvVarExpression(value string) string {
	regex := regexp.MustCompile(`\${(\w+)}`)
	return regex.ReplaceAllStringFunc(value, func(envVar string) string {
		envVar = strings.TrimPrefix(envVar, "${")
		envVar = strings.TrimSuffix(envVar, "}")
		envValue := os.Getenv(envVar)

		// if env variable is hostname and it is empty use os.Hostname as default
		if envVar == "HOSTNAME" && envValue == "" {
			envValue, _ = os.Hostname()
		}

		return envValue
	})
}

func evalConfigValues() {
	for _, section := range Cfg.Sections() {
		for _, key := range section.Keys() {
			key.SetValue(evalEnvVarExpression(key.Value()))
		}
	}
}

func loadSpecifedConfigFile(configFile string) error {
	if configFile == "" {
		configFile = filepath.Join(HomePath, CustomInitPath)
		// return without error if custom file does not exist
		if !pathExists(configFile) {
			return nil
		}
	}

	userConfig, err := ini.Load(configFile)
	if err != nil {
		return fmt.Errorf("Failed to parse %v, %v", configFile, err)
	}

	userConfig.BlockMode = false

	for _, section := range userConfig.Sections() {
		for _, key := range section.Keys() {
			if key.Value() == "" {
				continue
			}

			defaultSec, err := Cfg.GetSection(section.Name())
			if err != nil {
				defaultSec, _ = Cfg.NewSection(section.Name())
			}
			defaultKey, err := defaultSec.GetKey(key.Name())
			if err != nil {
				defaultKey, _ = defaultSec.NewKey(key.Name(), key.Value())
			}
			defaultKey.SetValue(key.Value())
		}
	}

	configFiles = append(configFiles, configFile)
	return nil
}

func loadConfiguration(args *CommandLineArgs) {
	var err error

	// load config defaults
	defaultConfigFile := path.Join(HomePath, "conf/defaults.ini")
	configFiles = append(configFiles, defaultConfigFile)

	// check if config file exists
	if _, err := os.Stat(defaultConfigFile); os.IsNotExist(err) {
		fmt.Println("Grafana-server Init Failed: Could not find config defaults, make sure homepath command line parameter is set or working directory is homepath")
		os.Exit(1)
	}

	// load defaults
	Cfg, err = ini.Load(defaultConfigFile)
	if err != nil {
		fmt.Println(fmt.Sprintf("Failed to parse defaults.ini, %v", err))
		os.Exit(1)
		return
	}

	Cfg.BlockMode = false

	// command line props
	commandLineProps := getCommandLineProperties(args.Args)
	// load default overrides
	applyCommandLineDefaultProperties(commandLineProps)

	// load specified config file
	err = loadSpecifedConfigFile(args.Config)
	if err != nil {
		initLogging()
		log.Fatal(3, err.Error())
	}

	// apply environment overrides
	applyEnvVariableOverrides()

	// apply command line overrides
	applyCommandLineProperties(commandLineProps)

	// evaluate config values containing environment variables
	evalConfigValues()

	// update data path and logging config
	DataPath = makeAbsolute(Cfg.Section("paths").Key("data").String(), HomePath)
	initLogging()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func setHomePath(args *CommandLineArgs) {
	if args.HomePath != "" {
		HomePath = args.HomePath
		return
	}

	HomePath, _ = filepath.Abs(".")
	// check if homepath is correct
	if pathExists(filepath.Join(HomePath, "conf/defaults.ini")) {
		return
	}

	// try down one path
	if pathExists(filepath.Join(HomePath, "../conf/defaults.ini")) {
		HomePath = filepath.Join(HomePath, "../")
	}
}

var skipStaticRootValidation bool = false

func validateStaticRootPath() error {
	if skipStaticRootValidation {
		return nil
	}

	if _, err := os.Stat(path.Join(StaticRootPath, "css")); err == nil {
		return nil
	}

	if _, err := os.Stat(StaticRootPath + "_gen/css"); err == nil {
		StaticRootPath = StaticRootPath + "_gen"
		return nil
	}

	return fmt.Errorf("Failed to detect generated css or javascript files in static root (%s), have you executed default grunt task?", StaticRootPath)
}

func NewConfigContext(args *CommandLineArgs) error {
	setHomePath(args)
	loadConfiguration(args)

	Env = Cfg.Section("").Key("app_mode").MustString("development")
	InstanceName = Cfg.Section("").Key("instance_name").MustString("unknown_instance_name")
	PluginsPath = makeAbsolute(Cfg.Section("paths").Key("plugins").String(), HomePath)

	server := Cfg.Section("server")
	AppUrl, AppSubUrl = parseAppUrlAndSubUrl(server)

	Protocol = HTTP
	if server.Key("protocol").MustString("http") == "https" {
		Protocol = HTTPS
		CertFile = server.Key("cert_file").String()
		KeyFile = server.Key("cert_key").String()
	}
	if server.Key("protocol").MustString("http") == "socket" {
		Protocol = SOCKET
		SocketPath = server.Key("socket").String()
	}

	Domain = server.Key("domain").MustString("localhost")
	HttpAddr = server.Key("http_addr").MustString(DEFAULT_HTTP_ADDR)
	HttpPort = server.Key("http_port").MustString("3000")
	RouterLogging = server.Key("router_logging").MustBool(false)

	EnableGzip = server.Key("enable_gzip").MustBool(false)
	EnforceDomain = server.Key("enforce_domain").MustBool(false)
	StaticRootPath = makeAbsolute(server.Key("static_root_path").String(), HomePath)

	if err := validateStaticRootPath(); err != nil {
		return err
	}

	// read data proxy settings
	dataproxy := Cfg.Section("dataproxy")
	DataProxyLogging = dataproxy.Key("logging").MustBool(false)

	// read security settings
	security := Cfg.Section("security")
	SecretKey = security.Key("secret_key").String()
	LogInRememberDays = security.Key("login_remember_days").MustInt()
	CookieUserName = security.Key("cookie_username").String()
	CookieRememberName = security.Key("cookie_remember_name").String()
	DisableGravatar = security.Key("disable_gravatar").MustBool(true)

	// read snapshots settings
	snapshots := Cfg.Section("snapshots")
	ExternalSnapshotUrl = snapshots.Key("external_snapshot_url").String()
	ExternalSnapshotName = snapshots.Key("external_snapshot_name").String()
	ExternalEnabled = snapshots.Key("external_enabled").MustBool(true)
	SnapShotRemoveExpired = snapshots.Key("snapshot_remove_expired").MustBool(true)
	SnapShotTTLDays = snapshots.Key("snapshot_TTL_days").MustInt(90)

	//  read data source proxy white list
	DataProxyWhiteList = make(map[string]bool)
	for _, hostAndIp := range util.SplitString(security.Key("data_source_proxy_whitelist").String()) {
		DataProxyWhiteList[hostAndIp] = true
	}

	// admin
	AdminUser = security.Key("admin_user").String()
	AdminPassword = security.Key("admin_password").String()

	users := Cfg.Section("users")
	AllowUserSignUp = users.Key("allow_sign_up").MustBool(true)
	AllowUserOrgCreate = users.Key("allow_org_create").MustBool(true)
	AutoAssignOrg = users.Key("auto_assign_org").MustBool(true)
	AutoAssignOrgRole = users.Key("auto_assign_org_role").In("Editor", []string{"Editor", "Admin", "Read Only Editor", "Viewer"})
	VerifyEmailEnabled = users.Key("verify_email_enabled").MustBool(false)
	LoginHint = users.Key("login_hint").String()
	DefaultTheme = users.Key("default_theme").String()
	ExternalUserMngLinkUrl = users.Key("external_manage_link_url").String()
	ExternalUserMngLinkName = users.Key("external_manage_link_name").String()
	ExternalUserMngInfo = users.Key("external_manage_info").String()

	// auth
	auth := Cfg.Section("auth")
	DisableLoginForm = auth.Key("disable_login_form").MustBool(false)
	DisableSignoutMenu = auth.Key("disable_signout_menu").MustBool(false)

	// anonymous access
	AnonymousEnabled = Cfg.Section("auth.anonymous").Key("enabled").MustBool(false)
	AnonymousOrgName = Cfg.Section("auth.anonymous").Key("org_name").String()
	AnonymousOrgRole = Cfg.Section("auth.anonymous").Key("org_role").String()

	// auth proxy
	authProxy := Cfg.Section("auth.proxy")
	AuthProxyEnabled = authProxy.Key("enabled").MustBool(false)
	AuthProxyHeaderName = authProxy.Key("header_name").String()
	AuthProxyHeaderProperty = authProxy.Key("header_property").String()
	AuthProxyAutoSignUp = authProxy.Key("auto_sign_up").MustBool(true)
	AuthProxyLdapSyncTtl = authProxy.Key("ldap_sync_ttl").MustInt()
	AuthProxyWhitelist = authProxy.Key("whitelist").String()

	// basic auth
	authBasic := Cfg.Section("auth.basic")
	BasicAuthEnabled = authBasic.Key("enabled").MustBool(true)

	// PhantomJS rendering
	ImagesDir = filepath.Join(DataPath, "png")
	PhantomDir = filepath.Join(HomePath, "vendor/phantomjs")

	analytics := Cfg.Section("analytics")
	ReportingEnabled = analytics.Key("reporting_enabled").MustBool(true)
	CheckForUpdates = analytics.Key("check_for_updates").MustBool(true)
	GoogleAnalyticsId = analytics.Key("google_analytics_ua_id").String()
	GoogleTagManagerId = analytics.Key("google_tag_manager_id").String()

	ldapSec := Cfg.Section("auth.ldap")
	LdapEnabled = ldapSec.Key("enabled").MustBool(false)
	LdapConfigFile = ldapSec.Key("config_file").String()
	LdapAllowSignup = ldapSec.Key("allow_sign_up").MustBool(true)

	alerting := Cfg.Section("alerting")
	AlertingEnabled = alerting.Key("enabled").MustBool(true)
	ExecuteAlerts = alerting.Key("execute_alerts").MustBool(true)

	clustering := Cfg.Section("clustering")
	ClusteringEnabled = clustering.Key("enabled").MustBool(true)
	MaxAlertEvalTimeLimitInSeconds = clustering.Key("max_alert_evaltime_limit_seconds").MustInt64(DEFAULT_ALERT_EVALTIME_LIMIT)
	MaxMissingAlertCount = clustering.Key("max_missing_alert_count").MustInt(DEFAULT_MISSING_ALERT_COUNT)
	DefaultMissingAlertsDelay = clustering.Key("default_missing_alerts_delay").MustInt64(DEFAULT_MISSING_ALERTS_DELAY)
	DefaultMissingAlertsSchedularTimeMinutes = clustering.Key("default_missing_alerts_schedular_time_minutes").MustInt(DEFAULT_MISSING_ALERTS_SCHEDULAR_TIME_MINUTES)
	ClusteringCleanupPeriod = clustering.Key("cleanup_period").MustInt(DEFAULT_CLUSTERING_CLEANUP_PERIOD)
	ClusteringHBRetention = clustering.Key("hb_retention_period").MustInt(DEFAULT_CLUSTERING_HB_RETENSION_PERIOD)
	AnnotationRetention = clustering.Key("annotation_retention_period").MustInt(DEFAULT_ANNOTATION_RETENSION_PERIOD)

	readSessionConfig()
	readSmtpSettings()
	readQuotaSettings()

	if VerifyEmailEnabled && !Smtp.Enabled {
		log.Warn("require_email_validation is enabled but smpt is disabled")
	}

	// check old key  name
	GrafanaComUrl = Cfg.Section("grafana_net").Key("url").MustString("")
	if GrafanaComUrl == "" {
		GrafanaComUrl = Cfg.Section("grafana_com").Key("url").MustString("https://grafana.com")
	}

	imageUploadingSection := Cfg.Section("external_image_storage")
	ImageUploadProvider = imageUploadingSection.Key("provider").MustString("internal")
	return nil
}

func readSessionConfig() {
	sec := Cfg.Section("session")
	SessionOptions = session.Options{}
	SessionOptions.Provider = sec.Key("provider").In("memory", []string{"memory", "file", "redis", "mysql", "postgres", "memcache"})
	SessionOptions.ProviderConfig = strings.Trim(sec.Key("provider_config").String(), "\" ")
	SessionOptions.CookieName = sec.Key("cookie_name").MustString("grafana_sess")
	SessionOptions.CookiePath = AppSubUrl
	SessionOptions.Secure = sec.Key("cookie_secure").MustBool()
	SessionOptions.Gclifetime = Cfg.Section("session").Key("gc_interval_time").MustInt64(86400)
	SessionOptions.Maxlifetime = Cfg.Section("session").Key("session_life_time").MustInt64(86400)
	SessionOptions.IDLength = 16

	if SessionOptions.Provider == "file" {
		SessionOptions.ProviderConfig = makeAbsolute(SessionOptions.ProviderConfig, DataPath)
		os.MkdirAll(path.Dir(SessionOptions.ProviderConfig), os.ModePerm)
	}

	if SessionOptions.CookiePath == "" {
		SessionOptions.CookiePath = "/"
	}
}

func initLogging() {
	// split on comma
	LogModes = strings.Split(Cfg.Section("log").Key("mode").MustString("console"), ",")
	// also try space
	if len(LogModes) == 1 {
		LogModes = strings.Split(Cfg.Section("log").Key("mode").MustString("console"), " ")
	}
	LogsPath = makeAbsolute(Cfg.Section("paths").Key("logs").String(), HomePath)
	log.ReadLoggingConfig(LogModes, LogsPath, Cfg)
}

func LogConfigurationInfo() {
	var text bytes.Buffer

	for _, file := range configFiles {
		logger.Info("Config loaded from", "file", file)
	}

	if len(appliedCommandLineProperties) > 0 {
		for _, prop := range appliedCommandLineProperties {
			logger.Info("Config overridden from command line", "arg", prop)
		}
	}

	if len(appliedEnvOverrides) > 0 {
		text.WriteString("\tEnvironment variables used:\n")
		for _, prop := range appliedEnvOverrides {
			logger.Info("Config overridden from Environment variable", "var", prop)
		}
	}

	logger.Info("Path Home", "path", HomePath)
	logger.Info("Path Data", "path", DataPath)
	logger.Info("Path Logs", "path", LogsPath)
	logger.Info("Path Plugins", "path", PluginsPath)
}
