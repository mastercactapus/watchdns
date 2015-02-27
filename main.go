package main

import (
	"github.com/coreos/fleet/registry"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"regexp"
	"strings"
	"time"
)

var mainCmd = &cobra.Command{
	Use:   "watchdns",
	Short: "A dynamic dns server configured by fleet service files",
	Run:   execute,
}

var domainRx = regexp.MustCompile(`^[a-z][a-z0-9-.]*\.$`)

func mustParseDurationKey(name string) time.Duration {
	d, err := time.ParseDuration(viper.GetString(name))
	if err != nil {
		log.Fatalf("invalid duration '%s' for key '%s': %s\n", viper.GetString(name), name, err.Error())
	}
	return d
}

func registryOptions() *RegistryOptions {
	opts := new(RegistryOptions)
	opts.CheckConcurrent = viper.GetInt("CheckConcurrent")
	opts.Domain = viper.GetString("Domain")
	opts.CheckInterval = mustParseDurationKey("CheckInterval")
	opts.CheckTimeout = mustParseDurationKey("CheckTimeout")
	opts.CheckResolution = mustParseDurationKey("CheckResolution")
	opts.FleetInterval = mustParseDurationKey("FleetInterval")
	opts.RecordSort = viper.GetString("RecordSort")
	if opts.RecordSort != "default" && opts.RecordSort != "random" && opts.RecordSort != "roundrobin" {
		log.Fatalln("Unknown RecordSort value: ", opts.RecordSort)
	}
	return opts
}

func execute(cmd *cobra.Command, args []string) {
	setupLogrus()
	if !domainRx.MatchString(strings.ToLower(viper.GetString("Domain"))) {
		log.Fatalln("invalid domain specified for Domain:", viper.GetString("Domain"))
	}
	peers := strings.Split(viper.GetString("EtcdPeers"), ",")
	r, err := NewServiceRegistry(peers, viper.GetString("FleetPrefix"), mustParseDurationKey("EtcdTimeout"), registryOptions())
	if err != nil {
		log.Fatalln("Failed to initialize fleet registry:", err)
	}
	r.Start()
	serveDns(r, viper.GetString("BindAddress"))
}

func init() {
	mainCmd.PersistentFlags().StringP("watch-domain", "d", "watchdns.", "tld to serve queries from, must end with a '.'")
	mainCmd.PersistentFlags().Duration("check-interval", time.Second*5, "Interval to set for CheckInterval when unspecified in a unit file.")
	mainCmd.PersistentFlags().Duration("check-timeout", time.Second*3, "Timeout for TCP and HTTP checks when unspecified in a unit file.")
	mainCmd.PersistentFlags().UintP("check-concurrent", "c", 20, "Number of concurrent health checks to run.")
	mainCmd.PersistentFlags().Duration("check-resolution", time.Second, "Maximum tick resolution for health check intervals.")
	mainCmd.PersistentFlags().DurationP("fleet-interval", "i", time.Second*3, "Time to wait between polling fleet for service changes.")
	mainCmd.PersistentFlags().StringP("etcd-peers", "e", "http://localhost:4001", "Comma-delimited list of etcd peers to connect to.")
	mainCmd.PersistentFlags().Duration("etcd-timeout", time.Second*5, "Timeout for etcd operations to complete.")
	mainCmd.PersistentFlags().String("fleet-prefix", registry.DefaultKeyPrefix, "Prefix for fleet registry in etcd.")
	mainCmd.PersistentFlags().StringP("bind-address", "b", ":8053", "Bind address for the DNS responder.")
	mainCmd.PersistentFlags().StringP("log-level", "l", "warn", "Log verbosity level, can be: 'debug', 'info', 'warn', 'error', or 'fatal'.")
	mainCmd.PersistentFlags().StringP("log-format", "o", "ascii", "Log format, can be: 'ascii' or 'json'.")
	mainCmd.PersistentFlags().StringP("record-sort", "s", "default", "Sort-order for DNS responses. Can be 'default', 'random', or 'roundrobin'")
	viper.BindPFlag("Domain", mainCmd.PersistentFlags().Lookup("watch-domain"))
	viper.BindPFlag("CheckInterval", mainCmd.PersistentFlags().Lookup("check-interval"))
	viper.BindPFlag("CheckTimeout", mainCmd.PersistentFlags().Lookup("check-timeout"))
	viper.BindPFlag("CheckConcurrent", mainCmd.PersistentFlags().Lookup("check-concurrent"))
	viper.BindPFlag("CheckResolution", mainCmd.PersistentFlags().Lookup("check-resolution"))
	viper.BindPFlag("FleetInterval", mainCmd.PersistentFlags().Lookup("fleet-interval"))
	viper.BindPFlag("EtcdTimeout", mainCmd.PersistentFlags().Lookup("etcd-timeout"))
	viper.BindPFlag("EtcdPeers", mainCmd.PersistentFlags().Lookup("etcd-peers"))
	viper.BindPFlag("FleetPrefix", mainCmd.PersistentFlags().Lookup("fleet-prefix"))
	viper.BindPFlag("BindAddress", mainCmd.PersistentFlags().Lookup("bind-address"))
	viper.BindPFlag("LogLevel", mainCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("LogFormat", mainCmd.PersistentFlags().Lookup("log-format"))
	viper.BindPFlag("RecordSort", mainCmd.PersistentFlags().Lookup("record-sort"))
	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/watchdns/")
	viper.ReadInConfig()
	viper.SetEnvPrefix("wdns")
	viper.AutomaticEnv()
}

func setupLogrus() {
	switch viper.GetString("LogFormat") {
	case "ascii":
		log.SetFormatter(&log.TextFormatter{})
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	default:
		log.Fatalln("Unknown log format:", viper.GetString("LogFormat"))
	}
	switch viper.GetString("LogLevel") {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	default:
		log.Fatalln("Unknown log level:", viper.GetString("LogLevel"))
	}
}

func main() {
	mainCmd.Execute()
}
