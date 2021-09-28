package config

import (
	pluginapi "github.com/mattermost/mattermost-plugin-api"
	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin/plugintest"
	"github.com/mattermost/mattermost-server/v6/services/configservice"

	"github.com/mattermost/mattermost-plugin-apps/server/telemetry"
	"github.com/mattermost/mattermost-plugin-apps/utils"
)

type TestService struct {
	config    Config
	mmconfig  model.Config
	mm        *pluginapi.Client
	log       utils.Logger
	telemetry *telemetry.Telemetry
}

var _ Service = (*TestService)(nil)

func NewTestConfigService(testConfig *Config) *TestService {
	conf, _ := NewTestService(testConfig)
	return conf
}

func NewTestService(testConfig *Config) (*TestService, *plugintest.API) {
	testAPI := &plugintest.API{}
	testDriver := &plugintest.Driver{}
	if testConfig == nil {
		testConfig = &Config{}
	}
	return &TestService{
		config:    *testConfig,
		log:       utils.NewTestLogger(),
		mm:        pluginapi.NewClient(testAPI, testDriver),
		telemetry: telemetry.NewTelemetry(nil),
	}, testAPI
}

func (s TestService) WithMattermostConfig(mmconfig model.Config) *TestService {
	s.mmconfig = mmconfig
	return &s
}

func (s TestService) WithMattermostAPI(mm *pluginapi.Client) *TestService {
	s.mm = mm
	return &s
}

func (s *TestService) Basic() (Config, *pluginapi.Client, utils.Logger) {
	return s.Get(),
		s.MattermostAPI(),
		s.Logger()
}

func (s *TestService) Get() Config {
	return s.config
}

func (s *TestService) Logger() utils.Logger {
	return s.log
}

func (s *TestService) MattermostAPI() *pluginapi.Client {
	return s.mm
}

func (s *TestService) Telemetry() *telemetry.Telemetry {
	return s.telemetry
}

func (s *TestService) MattermostConfig() configservice.ConfigService {
	return &mattermostConfigService{&s.mmconfig}
}

func (s *TestService) Reconfigure(StoredConfig, ...Configurable) error {
	return nil
}

func (s *TestService) StoreConfig(sc StoredConfig) error {
	s.config.StoredConfig = sc
	return nil
}
