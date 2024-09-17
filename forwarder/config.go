package main

type Config struct {
	LogLevel   string `env:"LOG_LEVEL" default:"INFO"`
	ListenAddr string `env:"LISTEN_ADDR" default:"localhost:8080"`
	RootCA     string `env:"ROOT_CA" default:"rootca.pem"`
	ClientCert string `env:"CLIENT_CERT" default:"client.pem"`
	ClientKey  string `env:"CLIENT_KEY" default:"client.key"`
	RemoteUrl  string `env:"REMOTE_URL" default:"https://103.252.119.26:220"`
}
