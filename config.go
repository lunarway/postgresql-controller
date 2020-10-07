package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"go.lunarway.com/postgresql-controller/pkg/postgres"
)

type controllerConfiguration struct {
	MetricsAddress           string
	EnableLeaderElection     bool
	ResyncPeriod             time.Duration
	UserRoles                []string
	UserRolePrefix           string
	AWS                      awsConfig
	HostCredentials          map[string]postgres.Credentials
	AllDatabasesReadEnabled  bool
	AllDatabasesWriteEnabled bool
	ExtendedWriteEnabled     bool
}

type awsConfig struct {
	PolicyName      string
	Region          string
	AccountID       string
	Profile         string
	AccessKeyID     string
	SecretAccessKey string
}

func (c *controllerConfiguration) RegisterFlags(flagSet *pflag.FlagSet) {
	flagSet.StringVar(&c.MetricsAddress, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flagSet.BoolVar(&c.EnableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flagSet.DurationVar(&c.ResyncPeriod, "resync-period", 10*time.Hour, "determines the minimum frequency at which watched resources are reconciled")

	flagSet.Var(&hostCredentials{value: &c.HostCredentials}, "host-credentials", "Host and credential pairs in the form hostname=user:password. Use comma separated pairs for multiple hosts")
	flagSet.StringSliceVar(&c.UserRoles, "user-roles", []string{"rds_iam"}, "List of roles granted to all users")
	flagSet.BoolVar(&c.AllDatabasesReadEnabled, "all-databases-enabled-read", false, "Enable usage of allDatabases field in read access requests")
	flagSet.BoolVar(&c.AllDatabasesWriteEnabled, "all-databases-enabled-write", false, "Enable usage of allDatabases field in write access requests")
	flagSet.StringVar(&c.UserRolePrefix, "user-role-prefix", "iam_developer_", "Prefix of roles created in PostgreSQL for users")
	flagSet.StringVar(&c.AWS.PolicyName, "aws-policy-name", "postgres-controller-users", "AWS Policy name to update IAM statements on")
	flagSet.StringVar(&c.AWS.Region, "aws-region", "eu-west-1", "AWS Region where IAM policies are located")
	flagSet.StringVar(&c.AWS.AccountID, "aws-account-id", "660013655494", "AWS Account id where IAM policies are located")
	flagSet.StringVar(&c.AWS.Profile, "aws-profile", "", "AWS Profile to use for credentials")
	flagSet.StringVar(&c.AWS.AccessKeyID, "aws-access-key-id", "", "AWS access key id to use for credentials")
	flagSet.StringVar(&c.AWS.SecretAccessKey, "aws-secret-access-key", "", "AWS secret access key to use for credentials")
	flagSet.BoolVar(&c.ExtendedWriteEnabled, "extended-write-enabled", false, "Enable extended write access requests")
}

func (c *controllerConfiguration) Log(log logr.Logger) {
	var hostNames []string
	for host := range c.HostCredentials {
		hostNames = append(hostNames, host)
	}
	log.Info("Controller configured",
		"hosts", hostNames,
		"roles", c.UserRoles,
		"prefix", c.UserRolePrefix,
		"awsPolicyName", c.AWS.PolicyName,
		"awsRegion", c.AWS.Region,
		"awsAccountID", c.AWS.AccountID,
		"allDatabasesReadEnabled", c.AllDatabasesReadEnabled,
		"allDatabasesWriteEnabled", c.AllDatabasesWriteEnabled,
	)
}

type hostCredentials struct {
	value *map[string]postgres.Credentials
}

func (h *hostCredentials) Set(val string) error {
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
		parts := strings.Split(hostCredential, "=")
		if len(parts) != 2 {
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
		(*h.value)[host] = parsedCrendetials
	}
	return nil
}

func (h *hostCredentials) Type() string {
	return "stringToCredentials"
}

func (h *hostCredentials) String() string {
	if h.value == nil {
		return "[]"
	}
	records := make([]string, 0, len(*h.value)>>1)
	for k, v := range *h.value {
		pair := k + "=" + v.Name
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
