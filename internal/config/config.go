package config

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

type ControllerConfiguration struct {
	MetricsAddress            string
	ProbeAddress              string
	EnableLeaderElection      bool
	ResyncPeriod              time.Duration
	UserRoles                 string
	UserRolePrefix            string
	GlobalExtensionsToInstall string
	AWS                       AwsConfig
	HostCredentials           map[string]postgres.Credentials
	ManagerRoleName           string
	SuperuserRoleName         string
	AllDatabasesReadEnabled   bool
	AllDatabasesWriteEnabled  bool
	ExtendedWriteEnabled      bool
	IAMPolicyPrefix           string
	SecureMetrics             bool
	EnableHTTP2               bool
}

type AwsConfig struct {
	PolicyName      string
	Region          string
	AccountID       string
	Profile         string
	AccessKeyID     string
	SecretAccessKey string
	LoginRoles      string
}

func (c *ControllerConfiguration) RegisterFlags(flagSet *flag.FlagSet) {
	flagSet.StringVar(&c.MetricsAddress, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flagSet.StringVar(&c.ProbeAddress, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flagSet.BoolVar(&c.EnableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flagSet.DurationVar(&c.ResyncPeriod, "resync-period", 10*time.Hour, "determines the minimum frequency at which watched resources are reconciled")

	flagSet.Var(&HostCredentials{value: &c.HostCredentials}, "host-credentials", "Host and credential pairs in the form hostname=user:password. Use comma separated pairs for multiple hosts")
	flagSet.StringVar(&c.ManagerRoleName, "manager-role-name", "postgres_role_manager", "Name of the role which will be managing other roles")
	flagSet.StringVar(&c.SuperuserRoleName, "superuser-role-name", "rds_superuser", "Name of the superuser role the connecting user must be a member of (defaults to RDS's rds_superuser; override for non-RDS deployments)")
	flagSet.StringVar(&c.UserRoles, "user-roles", "rds_iam", "List of roles granted to all users")
	flagSet.StringVar(&c.GlobalExtensionsToInstall, "global-extensions-to-install", "", "Comma-separated list of PostgreSQL extensions to install on all databases (existing extensions are never removed)")
	flagSet.BoolVar(&c.AllDatabasesReadEnabled, "all-databases-enabled-read", false, "Enable usage of allDatabases field in read access requests")
	flagSet.BoolVar(&c.AllDatabasesWriteEnabled, "all-databases-enabled-write", false, "Enable usage of allDatabases field in write access requests")
	flagSet.StringVar(&c.UserRolePrefix, "user-role-prefix", "iam_developer_", "Prefix of roles created in PostgreSQL for users")
	flagSet.StringVar(&c.AWS.PolicyName, "aws-policy-name", "postgres-controller-users", "AWS Policy name to update IAM statements on")
	flagSet.StringVar(&c.AWS.Region, "aws-region", "eu-west-1", "AWS Region where IAM policies are located")
	flagSet.StringVar(&c.AWS.AccountID, "aws-account-id", "660013655494", "AWS Account id where IAM policies are located")
	flagSet.StringVar(&c.AWS.Profile, "aws-profile", "", "AWS Profile to use for credentials")
	flagSet.StringVar(&c.AWS.AccessKeyID, "aws-access-key-id", "", "AWS access key id to use for credentials")
	flagSet.StringVar(&c.AWS.SecretAccessKey, "aws-secret-access-key", "", "AWS secret access key to use for credentials")
	flagSet.StringVar(&c.AWS.LoginRoles, "aws-login-role", "", "AWS IAM role to attach the policies to")
	flagSet.BoolVar(&c.ExtendedWriteEnabled, "extended-write-enabled", false, "Enable extended write access requests")
	flagSet.StringVar(&c.IAMPolicyPrefix, "iam-policy-prefix", "/", "Path prefix to use when creating IAM policies")
	flagSet.BoolVar(&c.SecureMetrics, "secure-metrics", false, "Whether to serve metrics with https")
	flagSet.BoolVar(&c.EnableHTTP2, "enable-http2", false, "Whether to serve traffic via. http2")
}

func (c *ControllerConfiguration) GetUserRoles() []string {
	return strings.Split(c.UserRoles, ",")
}

func (c *ControllerConfiguration) GetLoginRoles() []string {
	return strings.Split(c.AWS.LoginRoles, ",")
}

// GetGlobalExtensions returns the list of global extensions to install on all databases.
// Returns an empty slice if GlobalExtensionsToInstall is empty.
// Invalid extension names (containing spaces or invalid characters) are filtered out.
func (c *ControllerConfiguration) GetGlobalExtensions() []string {
	if c.GlobalExtensionsToInstall == "" {
		return []string{}
	}
	extensions := strings.Split(c.GlobalExtensionsToInstall, ",")
	result := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		trimmed := strings.TrimSpace(ext)
		if trimmed != "" && isValidExtensionName(trimmed) {
			result = append(result, trimmed)
		}
	}
	return result
}

// isValidExtensionName validates that an extension name follows PostgreSQL identifier rules.
// Extension names should contain only letters, digits, underscores, and hyphens.
// They cannot contain spaces or other special characters.
func isValidExtensionName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func (c *ControllerConfiguration) Log(log logr.Logger) {
	var hostNames []string
	for host := range c.HostCredentials {
		hostNames = append(hostNames, host)
	}
	log.Info("Controller configured",
		"hosts", hostNames,
		"roles", c.UserRoles,
		"prefix", c.UserRolePrefix,
		"globalExtensionsToInstall", c.GlobalExtensionsToInstall,
		"awsPolicyName", c.AWS.PolicyName,
		"awsRegion", c.AWS.Region,
		"awsAccountID", c.AWS.AccountID,
		"awsLoginRoles", c.AWS.LoginRoles,
		"allDatabasesReadEnabled", c.AllDatabasesReadEnabled,
		"allDatabasesWriteEnabled", c.AllDatabasesWriteEnabled,
		"iamPolicyPrefix", c.IAMPolicyPrefix,
	)
}

type HostCredentials struct {
	value *map[string]postgres.Credentials
}

func (h *HostCredentials) Set(val string) error {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	if h.value == nil {
		h.value = &map[string]postgres.Credentials{}
	}
	if *h.value == nil {
		*h.value = map[string]postgres.Credentials{}
	}
	hostCredentials := strings.Split(val, ",")
	for _, hostCredential := range hostCredentials {
		parts := strings.SplitN(hostCredential, "=", 3)
		if len(parts) != 2 && len(parts) != 3 {
			return fmt.Errorf("%s must be formatted as key=value", hostCredential)
		}
		host, userPass := parts[0], parts[1]
		if host == "" {
			return fmt.Errorf("%s must be formatted as key=value", hostCredential)
		}
		parsedCrendetials, err := postgres.ParseUsernamePassword(userPass)
		if err != nil {
			return fmt.Errorf("parse host '%s' failed: %w", hostCredential, err)
		}
		if len(parts) == 3 {
			parsedCrendetials.Params = parts[2]
		}
		(*h.value)[host] = parsedCrendetials
	}
	return nil
}

func (h *HostCredentials) Type() string {
	return "stringToCredentials"
}

func (h *HostCredentials) String() string {
	if h.value == nil {
		return "[]"
	}
	records := make([]string, 0, len(*h.value)>>1)
	for k, v := range *h.value {
		pair := k + "=" + v.User
		if v.Password != "" {
			pair += ":********"
		}
		records = append(records, pair)
	}

	// make sure the output is stable
	sort.Strings(records)

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(records); err != nil {
		panic(err)
	}
	w.Flush()
	return "[" + strings.TrimSpace(buf.String()) + "]"
}
