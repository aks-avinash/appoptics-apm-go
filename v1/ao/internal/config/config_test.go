// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aolog "github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestLoadConfig(t *testing.T) {
	key1 := "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go"
	key2 := "bbbb315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:Go"

	os.Setenv(envAppOpticsCollector, "example.com:12345")
	os.Setenv(envAppOpticsPrependDomain, "true")
	os.Setenv(envAppOpticsHistogramPrecision, "2")
	os.Setenv(envAppOpticsServiceKey, key1)
	os.Setenv(envAppOpticsDisabled, "true")

	c := NewConfig()
	assert.Equal(t, "example.com:12345", c.GetCollector())
	assert.Equal(t, true, c.PrependDomain)
	assert.Equal(t, 2, c.Precision)
	assert.Equal(t, true, c.Disabled)

	os.Setenv(envAppOpticsCollector, "test.abc:8080")
	os.Setenv(envAppOpticsDisabled, "false")
	os.Setenv(envAppOpticsTracingMode, "always")

	c.Load()
	assert.Equal(t, "test.abc:8080", c.GetCollector())
	assert.Equal(t, false, c.Disabled)
	assert.Equal(t, "enabled", string(c.GetTracingMode()))

	c = NewConfig(
		WithCollector("hello.world"),
		WithServiceKey(key2))
	assert.Equal(t, "hello.world", c.GetCollector())
	assert.Equal(t, ToServiceKey(key2), c.GetServiceKey())

	os.Setenv(envAppOpticsServiceKey, key1)
	os.Setenv(envAppOpticsHostnameAlias, "test")
	os.Setenv(envAppOpticsInsecureSkipVerify, "false")
	os.Setenv(envAppOpticsTrustedPath, "test.crt")
	os.Setenv(envAppOpticsCollectorUDP, "hello.udp")
	os.Setenv(envAppOpticsDisabled, "invalidValue")

	c.Load()
	assert.Equal(t, ToServiceKey(key1), c.GetServiceKey())
	assert.Equal(t, "test", c.GetHostAlias())
	assert.Equal(t, false, c.GetSkipVerify())
	assert.Equal(t, "test.crt", filepath.Base(c.GetTrustedPath()))
	assert.Equal(t, "hello.udp", c.GetCollectorUDP())
	assert.Equal(t, false, c.GetDisabled())
}

func TestConfig_HasLocalSamplingConfig(t *testing.T) {
	// Set tracing mode
	_ = os.Setenv(envAppOpticsTracingMode, "disabled")
	Load()
	assert.True(t, SamplingConfigured())
	assert.Equal(t, "disabled", string(GetTracingMode()))
	assert.Equal(t, ToInteger(getFieldDefaultValue(&SamplingConfig{}, "SampleRate")), GetSampleRate())

	// No local sampling config
	_ = os.Unsetenv(envAppOpticsTracingMode)
	Load()
	assert.False(t, SamplingConfigured())
	assert.Equal(t, getFieldDefaultValue(&SamplingConfig{}, "TracingMode"), string(GetTracingMode()))
	assert.Equal(t, ToInteger(getFieldDefaultValue(&SamplingConfig{}, "SampleRate")), GetSampleRate())

	// Set sample rate to 10000
	_ = os.Setenv(envAppOpticsSampleRate, "10000")
	Load()
	assert.True(t, SamplingConfigured())
	assert.Equal(t, getFieldDefaultValue(&SamplingConfig{}, "TracingMode"), string(GetTracingMode()))
	assert.Equal(t, 10000, GetSampleRate())
}

func TestPrintDelta(t *testing.T) {
	changed := newConfig().reset()
	changed.Collector = "test.com:443"
	changed.PrependDomain = true
	changed.ReporterProperties.EventFlushInterval = 100

	assert.Equal(t,
		` - Collector (APPOPTICS_COLLECTOR) = test.com:443 (default: collector.appoptics.com:443)
 - PrependDomain (APPOPTICS_PREPEND_DOMAIN) = true (default: false)
 - ReporterProperties.EventFlushInterval (APPOPTICS_EVENTS_FLUSH_INTERVAL) = 100 (default: 2)`,
		getDelta(newConfig().reset(), changed, "").sanitize().String())
}

func TestConfigInit(t *testing.T) {
	c := newConfig()

	// Set them to true, the call to `reset` in next step should reset them to false
	c.Sampling.sampleRateConfigured = true
	c.Sampling.tracingModeConfigured = true

	c.reset()

	defaultC := Config{
		Collector:    defaultSSLCollector,
		ServiceKey:   "",
		TrustedPath:  "",
		CollectorUDP: "",
		ReporterType: "ssl",
		Sampling: &SamplingConfig{
			TracingMode:           "enabled",
			tracingModeConfigured: false,
			SampleRate:            1000000,
			sampleRateConfigured:  false,
		},
		PrependDomain: false,
		HostAlias:     "",
		SkipVerify:    false,
		Precision:     2,
		ReporterProperties: &ReporterOptions{
			EventFlushInterval:      2,
			EventFlushBatchSize:     2000,
			MetricFlushInterval:     30,
			GetSettingsInterval:     30,
			SettingsTimeoutInterval: 10,
			PingInterval:            20,
			RetryDelayInitial:       500,
			RetryDelayMax:           60,
			RedirectMax:             20,
			RetryLogThreshold:       10,
			MaxRetries:              20,
		},
		Disabled:   false,
		DebugLevel: "warn",
	}
	assert.Equal(t, *c, defaultC)
}

func ClearEnvs() {
	for _, kv := range os.Environ() {
		kvSlice := strings.Split(kv, "=")
		k := kvSlice[0]
		os.Unsetenv(k)
	}
}

func SetEnvs(kvs []string) {
	for _, kv := range kvs {
		kvSlice := strings.Split(kv, "=")
		k, v := kvSlice[0], kvSlice[1]
		os.Setenv(k, v)
	}
}

func TestEnvsLoading(t *testing.T) {
	ClearEnvs()

	envs := []string{
		"APPOPTICS_COLLECTOR=collector.test.com",
		"APPOPTICS_SERVICE_KEY=ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go",
		"APPOPTICS_TRUSTEDPATH=/collector.crt",
		"APPOPTICS_COLLECTOR_UDP=udp.test.com",
		"APPOPTICS_REPORTER=udp",
		"APPOPTICS_TRACING_MODE=never",
		"APPOPTICS_SAMPLE_RATE=1000",
		"APPOPTICS_PREPEND_DOMAIN=true",
		"APPOPTICS_HOSTNAME_ALIAS=alias",
		"APPOPTICS_INSECURE_SKIP_VERIFY=true",
		"APPOPTICS_HISTOGRAM_PRECISION=4",
		"APPOPTICS_EVENTS_FLUSH_INTERVAL=4",
		"APPOPTICS_EVENTS_BATCHSIZE=4000",
		"APPOPTICS_DISABLED=true",
	}
	SetEnvs(envs)

	envConfig := Config{
		Collector:    "collector.test.com",
		ServiceKey:   "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go",
		TrustedPath:  "/collector.crt",
		CollectorUDP: "udp.test.com",
		ReporterType: "udp",
		Sampling: &SamplingConfig{
			TracingMode:           "disabled",
			tracingModeConfigured: true,
			SampleRate:            1000,
			sampleRateConfigured:  true,
		},
		PrependDomain: true,
		HostAlias:     "alias",
		SkipVerify:    true,
		Precision:     2 * 2,
		ReporterProperties: &ReporterOptions{
			EventFlushInterval:      2 * 2,
			EventFlushBatchSize:     2000 * 2,
			MetricFlushInterval:     30,
			GetSettingsInterval:     30,
			SettingsTimeoutInterval: 10,
			PingInterval:            20,
			RetryDelayInitial:       500,
			RetryDelayMax:           60,
			RedirectMax:             20,
			RetryLogThreshold:       10,
			MaxRetries:              20,
		},
		Disabled:   true,
		DebugLevel: "warn",
	}

	c := NewConfig()

	assert.Equal(t, *c, envConfig)
}

func TestYamlConfig(t *testing.T) {
	yamlConfig := Config{
		Collector:    "yaml.test.com",
		ServiceKey:   "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189218:go",
		TrustedPath:  "/yaml-collector.crt",
		CollectorUDP: "yamludp.test.com",
		ReporterType: "udp",
		Sampling: &SamplingConfig{
			TracingMode:           "disabled",
			tracingModeConfigured: true,
			SampleRate:            100,
			sampleRateConfigured:  true,
		},
		PrependDomain: true,
		HostAlias:     "yaml-alias",
		SkipVerify:    true,
		Precision:     2 * 3,
		ReporterProperties: &ReporterOptions{
			EventFlushInterval:      2 * 3,
			EventFlushBatchSize:     2000 * 3,
			MetricFlushInterval:     30,
			GetSettingsInterval:     30,
			SettingsTimeoutInterval: 10,
			PingInterval:            20,
			RetryDelayInitial:       500,
			RetryDelayMax:           60,
			RedirectMax:             20,
			RetryLogThreshold:       10,
			MaxRetries:              20,
		},
		TransactionSettings: []TransactionFilter{
			{"url", `\s+\d+\s+`, nil, "disabled"},
			{"url", "", []string{".jpg"}, "disabled"},
		},
		Disabled:   true,
		DebugLevel: "info",
	}

	out, err := yaml.Marshal(yamlConfig)
	assert.Nil(t, err)

	err = ioutil.WriteFile("/tmp/appoptics-config.yaml", out, 0644)
	assert.Nil(t, err)

	// Test with config file
	ClearEnvs()
	os.Setenv(EnvAppOpticsConfigFile, "/tmp/appoptics-config.yaml")

	c := NewConfig()
	assert.Equal(t, yamlConfig, *c)

	// Test with both config file and env variables
	envs := []string{
		"APPOPTICS_COLLECTOR=collector.test.com",
		"APPOPTICS_SERVICE_KEY=ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go",
		"APPOPTICS_TRUSTEDPATH=/collector.crt",
		"APPOPTICS_COLLECTOR_UDP=udp.test.com",
		"APPOPTICS_REPORTER=udp",
		"APPOPTICS_TRACING_MODE=never",
		"APPOPTICS_SAMPLE_RATE=1000",
		"APPOPTICS_PREPEND_DOMAIN=true",
		"APPOPTICS_HOSTNAME_ALIAS=alias",
		"APPOPTICS_INSECURE_SKIP_VERIFY=true",
		"APPOPTICS_HISTOGRAM_PRECISION=4",
		"APPOPTICS_EVENTS_FLUSH_INTERVAL=4",
		"APPOPTICS_EVENTS_BATCHSIZE=4000",
		"APPOPTICS_DISABLED=true",
	}
	ClearEnvs()
	SetEnvs(envs)
	os.Setenv("APPOPTICS_CONFIG_FILE", "/tmp/appoptics-config.yaml")

	envConfig := Config{
		Collector:    "collector.test.com",
		ServiceKey:   "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go",
		TrustedPath:  "/collector.crt",
		CollectorUDP: "udp.test.com",
		ReporterType: "udp",
		Sampling: &SamplingConfig{
			TracingMode:           "disabled",
			tracingModeConfigured: true,
			SampleRate:            1000,
			sampleRateConfigured:  true,
		},
		PrependDomain: true,
		HostAlias:     "alias",
		SkipVerify:    true,
		Precision:     2 * 2,
		ReporterProperties: &ReporterOptions{
			EventFlushInterval:      2 * 2,
			EventFlushBatchSize:     2000 * 2,
			MetricFlushInterval:     30,
			GetSettingsInterval:     30,
			SettingsTimeoutInterval: 10,
			PingInterval:            20,
			RetryDelayInitial:       500,
			RetryDelayMax:           60,
			RedirectMax:             20,
			RetryLogThreshold:       10,
			MaxRetries:              20,
		},
		TransactionSettings: []TransactionFilter{
			{"url", `\s+\d+\s+`, nil, "disabled"},
			{"url", "", []string{".jpg"}, "disabled"},
		},
		Disabled:   true,
		DebugLevel: "info",
	}

	c = NewConfig()
	assert.Equal(t, envConfig, *c)

	os.Unsetenv("APPOPTICS_CONFIG_FILE")
}

func TestSamplingConfigValidate(t *testing.T) {
	s := &SamplingConfig{
		TracingMode:           "invalid",
		tracingModeConfigured: true,
		SampleRate:            10000000,
		sampleRateConfigured:  true,
	}
	s.validate()
	assert.Equal(t, EnabledTracingMode, s.TracingMode)
	assert.Equal(t, false, s.tracingModeConfigured)
	assert.Equal(t, 1000000, s.SampleRate)
	assert.Equal(t, false, s.sampleRateConfigured)
}

func TestInvalidConfigFile(t *testing.T) {
	var buf utils.SafeBuffer
	var writers []io.Writer

	writers = append(writers, &buf)
	writers = append(writers, os.Stderr)

	log.SetOutput(io.MultiWriter(writers...))

	defer func() {
		log.SetOutput(os.Stderr)
	}()

	ClearEnvs()
	os.Setenv("APPOPTICS_SERVICE_KEY", "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go")
	os.Setenv("APPOPTICS_CONFIG_FILE", "/tmp/appoptics-config.json")
	_ = ioutil.WriteFile("/tmp/appoptics-config.json", []byte("hello"), 0644)

	_ = NewConfig()
	assert.Contains(t, buf.String(), ErrUnsupportedFormat.Error())
	_ = os.Remove("/tmp/file-not-exist.yaml")

	buf.Reset()
	ClearEnvs()
	os.Setenv("APPOPTICS_SERVICE_KEY", "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go")
	os.Setenv("APPOPTICS_CONFIG_FILE", "/tmp/file-not-exist.yaml")
	_ = NewConfig()
	assert.Contains(t, buf.String(), "no such file or directory")
}

func TestInvalidConfig(t *testing.T) {
	var buf utils.SafeBuffer
	var writers []io.Writer

	writers = append(writers, &buf)
	writers = append(writers, os.Stderr)

	log.SetOutput(io.MultiWriter(writers...))

	defer func() {
		log.SetOutput(os.Stderr)
	}()

	invalid := Config{
		Collector:    "",
		ServiceKey:   "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go",
		TrustedPath:  "",
		CollectorUDP: "",
		ReporterType: "invalid",
		Sampling: &SamplingConfig{
			TracingMode:           "disabled",
			tracingModeConfigured: true,
			SampleRate:            1000,
			sampleRateConfigured:  true,
		},
		PrependDomain: true,
		HostAlias:     "alias",
		SkipVerify:    true,
		Precision:     2 * 2,
		ReporterProperties: &ReporterOptions{
			EventFlushInterval:      2 * 2,
			EventFlushBatchSize:     2000 * 2,
			MetricFlushInterval:     30,
			GetSettingsInterval:     30,
			SettingsTimeoutInterval: 10,
			PingInterval:            20,
			RetryDelayInitial:       500,
			RetryDelayMax:           60,
			RedirectMax:             20,
			RetryLogThreshold:       10,
			MaxRetries:              20,
		},
		Disabled:   true,
		DebugLevel: "info",
	}

	assert.Nil(t, invalid.validate())

	assert.Equal(t, defaultSSLCollector, invalid.Collector)
	assert.Contains(t, buf.String(), "invalid env, discarded - Collector:", buf.String())

	assert.Equal(t, "ssl", invalid.ReporterType)
	assert.Contains(t, buf.String(), "invalid env, discarded - ReporterType:", buf.String())

	assert.Equal(t, "alias", invalid.HostAlias)
}

// TestConfigDefaultValues is to verify the default values defined in struct Config
// are all correct
func TestConfigDefaultValues(t *testing.T) {
	// A Config object initialized with default values
	c := newConfig().reset()

	// check default log level
	level, ok := aolog.ToLogLevel(c.DebugLevel)
	assert.Equal(t, level, aolog.DefaultLevel)
	assert.True(t, ok)

	// check default ssl collector url
	assert.Equal(t, defaultSSLCollector, c.Collector)

	// check the default sample rate
	assert.Equal(t, MaxSampleRate, c.Sampling.SampleRate)
}

func TestTransactionFilter_UnmarshalYAML(t *testing.T) {
	var testCases = []struct {
		filter TransactionFilter
		err    error
	}{
		{TransactionFilter{"invalid", `\s+\d+\s+`, nil, "disabled"}, ErrTFInvalidType},
		{TransactionFilter{"url", `\s+\d+\s+`, nil, "enabled"}, nil},
		{TransactionFilter{"url", `\s+\d+\s+`, nil, "disabled"}, nil},
		{TransactionFilter{"url", "", []string{".jpg"}, "disabled"}, nil},
		{TransactionFilter{"url", `\s+\d+\s+`, []string{".jpg"}, "disabled"}, ErrTFInvalidRegExExt},
		{TransactionFilter{"url", `\s+\d+\s+`, nil, "disabled"}, nil},
		{TransactionFilter{"url", `\s+\d+\s+`, nil, "invalid"}, ErrTFInvalidTracing},
	}

	for idx, testCase := range testCases {
		bytes, err := yaml.Marshal(testCase.filter)
		assert.Nil(t, err, fmt.Sprintf("Case #%d", idx))

		var filter TransactionFilter
		err = yaml.Unmarshal(bytes, &filter)
		assert.Equal(t, testCase.err, err, fmt.Sprintf("Case #%d", idx))
		if err == nil {
			assert.Equal(t, testCase.filter, filter, fmt.Sprintf("Case #%d", idx))
		}
	}
}
