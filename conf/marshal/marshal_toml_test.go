package marshal

import (
	"fmt"
	"testing"

	"github.com/sipt/shuttle/conf/model"
	"github.com/stretchr/testify/assert"
)

type M map[string]string

var config = &model.Config{
	General: struct {
		LoggerLevel string `toml:"logger_level"`
	}{
		"debug",
	},

	Listener: []struct {
		Typ    string            `toml:"typ"`
		Addr   string            `toml:"addr"`
		Params map[string]string `toml:"params"`
	}{
		{"https", ":8081", map[string]string{"user": "root", "password": "123123"}},
		{"socks", ":8080", map[string]string{"user": "root", "password": "123123"}},
	},

	Server: map[string]struct {
		Typ    string            `toml:"typ"`
		Addr   string            `toml:"addr"`
		Port   string            `toml:"port"`
		Params map[string]string `toml:"params"`
	}{
		"JP1": {"ss", "jp.remote.com", "8080", M{"user": "root", "password": "123123"}},
		"JP2": {"ss", "jp.remote.com", "8080", M{"user": "root", "password": "123123"}},
		"US1": {"ss", "us.remote.com", "8080", M{"user": "root", "password": "123123"}},
		"US2": {"ss", "us.remote.com", "8080", M{"user": "root", "password": "123123"}},
	},

	ServerGroup: map[string]struct {
		Typ     string            `toml:"typ"`
		Servers []string          `toml:"servers"`
		Params  map[string]string `toml:"params"`
	}{
		"Proxy": {"select", []string{"AUTO", "JP", "US"}, nil},
		"AUTO":  {"rtt", []string{"JP1", "JP2", "US1", "US2"}, M{"url": "https://www.google.com"}},
		"JP":    {"select", []string{"JP1", "JP2"}, nil},
		"US":    {"select", []string{"US1", "US2"}, nil},
	},

	Rule: []struct {
		Typ    string            `toml:"typ"`
		Value  string            `toml:"value"`
		Params map[string]string `toml:"params"`
	}{
		{"DOMAIN", "google.com", M{"Proxy": "Proxy", "Comment": "search engine"}},
		{"DOMAIN", "github.com", M{"Proxy": "Proxy", "Comment": "source code"}},
	},
}

func TestTomlMarshal_Marshal(t *testing.T) {

	m, _ := newTomlMarshal(nil)
	data, err := m.Marshal(config)
	assert.NoError(t, err)

	fmt.Println(string(data))

	str := `
[general]
  logger_level = "debug"

[[listener]]
  typ = "https"
  addr = ":8081"
  [listener.params]
    password = "123123"
    user = "root"

[[listener]]
  typ = "socks"
  addr = ":8080"
  [listener.params]
    password = "123123"
    user = "root"

[server]
  JP1 = {typ = "ss", addr = "jp.remote.com", port = "8080", params = { password = "123123", user = "root" }}
  JP2 = {typ = "ss", addr = "jp.remote.com", port = "8080", params = { password = "123123", user = "root" }}
  US1 = {typ = "ss", addr = "us.remote.com", port = "8080", params = { password = "123123", user = "root" }}
  US2 = {typ = "ss", addr = "us.remote.com", port = "8080", params = { password = "123123", user = "root" }}

[server_group]
  [proxy_server_group.AUTO]
    typ = "rtt"
    servers = ["JP1", "JP2", "US1", "US2"]
    [proxy_server_group.AUTO.params]
      url = "https://www.google.com"
  [proxy_server_group.JP]
    typ = "select"
    servers = ["JP1", "JP2"]
  [proxy_server_group.Proxy]
    typ = "select"
    servers = ["AUTO", "JP", "US"]
  [proxy_server_group.US]
    typ = "select"
    servers = ["US1", "US2"]

[[rule]]
  typ = "DOMAIN"
  value = "google.com"
  [rule.params]
    Comment = "search engine"
    Proxy = "Proxy"

[[rule]]
  typ = "DOMAIN"
  value = "github.com"
  [rule.params]
    Comment = "source code"
    Proxy = "Proxy"

`

	config2, err := m.UnMarshal([]byte(str))
	assert.EqualValues(t, config, config2)
}